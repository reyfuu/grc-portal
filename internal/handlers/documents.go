package handlers

import (
	"context"
	"net/http"
	"strings"
	"time"
	"unicode"

	"grcportal/internal/ai"
	"grcportal/internal/auth"
)

type Document struct {
	ID        int64     `json:"id"`
	Title     string    `json:"title"`
	Category  string    `json:"category"`
	Source    string    `json:"source"`
	Body      string    `json:"body,omitempty"`
	Snippet   string    `json:"snippet,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

var validDocCategory = map[string]bool{"sop": true, "kebijakan": true, "evidence": true}

// headlineOpts keeps ts_headline output plain-ish: matched terms wrapped in [ ]
// (no HTML), so the frontend can render the snippet as text safely.
const headlineOpts = "MaxWords=45,MinWords=20,MaxFragments=1,StartSel=[,StopSel=]"

func (h *Handler) ListDocuments(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Query(`
		SELECT id, title, category, COALESCE(source,''), created_at
		FROM documents ORDER BY created_at DESC`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := []Document{}
	for rows.Next() {
		var d Document
		if err := rows.Scan(&d.ID, &d.Title, &d.Category, &d.Source, &d.CreatedAt); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, d)
	}
	writeJSON(w, http.StatusOK, out)
}

type docReq struct {
	Title    string `json:"title"`
	Category string `json:"category"`
	Source   string `json:"source"`
	Body     string `json:"body"`
}

func (h *Handler) CreateDocument(w http.ResponseWriter, r *http.Request) {
	var req docReq
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	req.Body = strings.TrimSpace(req.Body)
	req.Category = strings.ToLower(strings.TrimSpace(req.Category))
	if req.Category == "" {
		req.Category = "sop"
	}
	if req.Title == "" || req.Body == "" {
		writeErr(w, http.StatusBadRequest, "title and body are required")
		return
	}
	if !validDocCategory[req.Category] {
		writeErr(w, http.StatusBadRequest, "category must be one of: sop, kebijakan, evidence")
		return
	}
	var d Document
	err := h.DB.QueryRow(`
		INSERT INTO documents(title, category, source, body, created_by)
		VALUES($1,$2,NULLIF($3,''),$4,$5)
		RETURNING id, title, category, COALESCE(source,''), created_at`,
		req.Title, req.Category, req.Source, req.Body, auth.UserFrom(r.Context()).ID).
		Scan(&d.ID, &d.Title, &d.Category, &d.Source, &d.CreatedAt)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(auth.UserFrom(r.Context()).ID, "document.create", "document", d.ID, d.Title)
	writeJSON(w, http.StatusCreated, d)
}

func (h *Handler) DeleteDocument(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	res, err := h.DB.Exec(`DELETE FROM documents WHERE id = $1`, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeErr(w, http.StatusNotFound, "document not found")
		return
	}
	h.audit(auth.UserFrom(r.Context()).ID, "document.delete", "document", id, "")
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// orTSQuery turns free text into an OR tsquery (e.g. "cuti | tahunan"), keeping
// only alphanumeric tokens ≥2 chars. OR (not AND) so a natural-language question
// still retrieves docs matching any significant term; ts_rank then favors the
// docs matching the most terms. Building it ourselves keeps it injection-safe.
func orTSQuery(q string) string {
	toks := strings.FieldsFunc(strings.ToLower(q), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	kept := toks[:0]
	for _, t := range toks {
		if len([]rune(t)) >= 2 {
			kept = append(kept, t)
		}
	}
	return strings.Join(kept, " | ")
}

// retrieve runs the FTS query and returns the top-k matching documents.
func (h *Handler) retrieve(q, category string, k int, withBody bool) ([]Document, error) {
	tsq := orTSQuery(q)
	if tsq == "" {
		return []Document{}, nil
	}
	bodyCol := "''"
	if withBody {
		bodyCol = "body"
	}
	sql := `
		SELECT id, title, category, ` + bodyCol + `,
		       ts_headline('simple', body, to_tsquery('simple',$1), '` + headlineOpts + `') AS snippet
		FROM documents
		WHERE search @@ to_tsquery('simple',$1)`
	args := []any{tsq}
	if category != "" {
		sql += ` AND category = $2`
		args = append(args, category)
	}
	sql += ` ORDER BY ts_rank(search, to_tsquery('simple',$1)) DESC LIMIT ` + itoa(k)

	rows, err := h.DB.Query(sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Document{}
	for rows.Next() {
		var d Document
		if err := rows.Scan(&d.ID, &d.Title, &d.Category, &d.Body, &d.Snippet); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

func (h *Handler) SearchDocuments(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, http.StatusOK, []Document{})
		return
	}
	docs, err := h.retrieve(q, r.URL.Query().Get("category"), 20, false)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, docs)
}

type askReq struct {
	Question string `json:"question"`
	Category string `json:"category"`
}

type askResp struct {
	Answer    string     `json:"answer"`
	Sources   []Document `json:"sources"`
	AIEnabled bool       `json:"ai_enabled"`
	Model     string     `json:"model,omitempty"`
	Error     string     `json:"error,omitempty"`
}

// AskDocuments is the RAG endpoint: FTS-retrieve top docs, then have the chat
// model answer strictly from that context with references.
func (h *Handler) AskDocuments(w http.ResponseWriter, r *http.Request) {
	var req askReq
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	req.Question = strings.TrimSpace(req.Question)
	if req.Question == "" {
		writeErr(w, http.StatusBadRequest, "question is required")
		return
	}
	docs, err := h.retrieve(req.Question, req.Category, 4, true)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Sources exposed to the client carry only the snippet, not the full body.
	sources := make([]Document, len(docs))
	for i, d := range docs {
		sources[i] = Document{ID: d.ID, Title: d.Title, Category: d.Category, Snippet: d.Snippet}
	}

	if len(docs) == 0 {
		writeJSON(w, http.StatusOK, askResp{
			Answer:    "Tidak ada SOP/kebijakan/evidence yang cocok dengan pertanyaan itu.",
			Sources:   sources,
			AIEnabled: ai.Configured(),
		})
		return
	}
	if !ai.Configured() {
		// Degrade gracefully to search-only when the AI router isn't wired.
		writeJSON(w, http.StatusOK, askResp{Answer: "", Sources: sources, AIEnabled: false})
		return
	}

	var ctxText strings.Builder
	for _, d := range docs {
		body := d.Body
		if len(body) > 1800 {
			body = body[:1800]
		}
		ctxText.WriteString("### " + d.Title + " (" + d.Category + ")\n" + body + "\n\n")
	}
	system := "Kamu asisten GRC/compliance. Jawab pertanyaan HANYA berdasarkan kutipan dokumen di bawah. " +
		"Rangkum aturan yang berlaku beserta kondisi dan pengecualiannya bila ada. " +
		"Sebutkan judul dokumen sebagai referensi. Jika jawabannya tidak ada di dokumen, katakan tidak ditemukan. " +
		"Jawab ringkas dalam Bahasa Indonesia."
	user := "Pertanyaan: " + req.Question + "\n\nDokumen:\n" + ctxText.String()

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	answer, err := ai.Chat(ctx, system, user)
	if err != nil {
		// Retrieval still succeeded — return sources with the error note.
		writeJSON(w, http.StatusOK, askResp{Answer: "", Sources: sources, AIEnabled: true, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, askResp{Answer: answer, Sources: sources, AIEnabled: true, Model: ai.Model()})
}
