package handlers

import (
	"database/sql"
	"net/http"
	"strings"
	"time"

	"grcportal/internal/auth"
)

// EvidenceRequest is an incoming request for audit/assessment evidence (UC-01).
// It is resolved either by reusing an existing evidence document (jejak sumber =
// document_id) or by flagging a gap and routing it to an owner.
type EvidenceRequest struct {
	ID          int64      `json:"id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Requester   string     `json:"requester"`
	Status      string     `json:"status"`
	DocumentID  *int64     `json:"document_id"`
	DocTitle    string     `json:"document_title,omitempty"`
	Owner       string     `json:"owner"`
	Note        string     `json:"note"`
	CreatedAt   time.Time  `json:"created_at"`
	ResolvedAt  *time.Time `json:"resolved_at"`
}

var validRequester = map[string]bool{
	"auditor": true, "pihak_ketiga": true, "internal_audit": true, "assessor": true,
}
var validEvidenceStatus = map[string]bool{"open": true, "fulfilled": true, "gap": true}

const evidenceSelect = `
	SELECT er.id, er.title, er.description, er.requester, er.status,
	       er.document_id, COALESCE(d.title,''), COALESCE(er.owner,''), COALESCE(er.note,''),
	       er.created_at, er.resolved_at
	FROM evidence_requests er
	LEFT JOIN documents d ON d.id = er.document_id`

type scanner interface{ Scan(dest ...any) error }

func scanEvidence(sc scanner) (EvidenceRequest, error) {
	var e EvidenceRequest
	var docID sql.NullInt64
	var resolved sql.NullTime
	err := sc.Scan(&e.ID, &e.Title, &e.Description, &e.Requester, &e.Status,
		&docID, &e.DocTitle, &e.Owner, &e.Note, &e.CreatedAt, &resolved)
	if err != nil {
		return e, err
	}
	if docID.Valid {
		e.DocumentID = &docID.Int64
	}
	if resolved.Valid {
		e.ResolvedAt = &resolved.Time
	}
	return e, nil
}

// resolveEvidence validates a resolve action. Pure (no DB) so it is unit-tested.
func resolveEvidence(currentStatus, action, role string, hasDoc bool, owner string) error {
	if role != "admin" && role != "reviewer" {
		return errEvidencePermission
	}
	if currentStatus != "open" {
		return errEvidenceResolved
	}
	switch action {
	case "fulfilled":
		if !hasDoc {
			return errEvidenceDocRequired
		}
		return nil
	case "gap":
		if strings.TrimSpace(owner) == "" {
			return errEvidenceOwnerRequired
		}
		return nil
	default:
		return errBadEvidenceAction
	}
}

func writeEvidenceErr(w http.ResponseWriter, err error) {
	switch err {
	case errEvidencePermission:
		writeErr(w, http.StatusForbidden, err.Error())
	case errEvidenceResolved:
		writeErr(w, http.StatusConflict, err.Error())
	default:
		writeErr(w, http.StatusBadRequest, err.Error())
	}
}

func (h *Handler) ListEvidenceRequests(w http.ResponseWriter, r *http.Request) {
	q := evidenceSelect
	args := []any{}
	if s := r.URL.Query().Get("status"); validEvidenceStatus[s] {
		args = append(args, s)
		q += ` WHERE er.status = $1`
	}
	q += ` ORDER BY er.created_at DESC`
	rows, err := h.DB.Query(q, args...)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := []EvidenceRequest{}
	for rows.Next() {
		e, err := scanEvidence(rows)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, e)
	}
	writeJSON(w, http.StatusOK, out)
}

type evidenceReq struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Requester   string `json:"requester"`
}

func (h *Handler) CreateEvidenceRequest(w http.ResponseWriter, r *http.Request) {
	var req evidenceReq
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	req.Description = strings.TrimSpace(req.Description)
	req.Requester = strings.ToLower(strings.TrimSpace(req.Requester))
	if req.Requester == "" {
		req.Requester = "auditor"
	}
	if req.Title == "" {
		writeErr(w, http.StatusBadRequest, "title is required")
		return
	}
	if !validRequester[req.Requester] {
		writeErr(w, http.StatusBadRequest, "requester must be one of: auditor, pihak_ketiga, internal_audit, assessor")
		return
	}
	var id int64
	if err := h.DB.QueryRow(`
		INSERT INTO evidence_requests(title, description, requester, created_by)
		VALUES($1,$2,$3,$4) RETURNING id`,
		req.Title, req.Description, req.Requester, auth.UserFrom(r.Context()).ID).Scan(&id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(auth.UserFrom(r.Context()).ID, "evidence.create", "evidence_request", id, req.Title)
	writeJSON(w, http.StatusCreated, map[string]int64{"id": id})
}

// GetEvidenceRequest returns the request plus (a) suggested evidence documents to
// reuse — FTS over documents(category=evidence) — and (b) similar past requests to
// catch duplicate asks. The suggestions are advisory; a human resolves the request.
func (h *Handler) GetEvidenceRequest(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	er, err := scanEvidence(h.DB.QueryRow(evidenceSelect+` WHERE er.id = $1`, id))
	if err != nil {
		writeErr(w, http.StatusNotFound, "evidence request not found")
		return
	}

	query := er.Title + " " + er.Description
	matches, err := h.retrieve(query, "evidence", 5, false)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	similar, err := h.similarRequests(query, id, 5)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"request": er,
		"matches": matches,
		"similar": similar,
	})
}

// similarRequests finds other evidence requests whose text overlaps q (dedup signal).
func (h *Handler) similarRequests(q string, excludeID int64, k int) ([]EvidenceRequest, error) {
	tsq := orTSQuery(q)
	if tsq == "" {
		return []EvidenceRequest{}, nil
	}
	rows, err := h.DB.Query(evidenceSelect+`
		WHERE er.id <> $2 AND er.search @@ to_tsquery('simple',$1)
		ORDER BY ts_rank(er.search, to_tsquery('simple',$1)) DESC LIMIT `+itoa(k), tsq, excludeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []EvidenceRequest{}
	for rows.Next() {
		e, err := scanEvidence(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

type resolveEvidenceReq struct {
	Action     string `json:"action"` // "fulfilled" | "gap"
	DocumentID *int64 `json:"document_id"`
	Owner      string `json:"owner"`
	Note       string `json:"note"`
}

// ResolveEvidenceRequest is the human-approval step: reuse an evidence document
// (fulfill) or flag a gap and route it to an owner. Both leave an audit trail.
func (h *Handler) ResolveEvidenceRequest(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req resolveEvidenceReq
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	req.Action = strings.ToLower(strings.TrimSpace(req.Action))
	u := auth.UserFrom(r.Context())
	hasDoc := req.DocumentID != nil && *req.DocumentID > 0

	tx, err := h.DB.Begin()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()

	var status string
	if err := tx.QueryRow(`SELECT status FROM evidence_requests WHERE id = $1 FOR UPDATE`, id).Scan(&status); err != nil {
		writeErr(w, http.StatusNotFound, "evidence request not found")
		return
	}
	if err := resolveEvidence(status, req.Action, u.Role, hasDoc, req.Owner); err != nil {
		writeEvidenceErr(w, err)
		return
	}

	var docArg, ownerArg any // stays nil for the branch that doesn't apply
	if req.Action == "fulfilled" {
		// jejak sumber must point at a real evidence document.
		var exists bool
		if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM documents WHERE id=$1 AND category='evidence')`,
			*req.DocumentID).Scan(&exists); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !exists {
			writeErr(w, http.StatusBadRequest, "evidence document not found")
			return
		}
		docArg = *req.DocumentID
	} else {
		ownerArg = strings.TrimSpace(req.Owner)
	}

	if _, err := tx.Exec(`
		UPDATE evidence_requests
		SET status=$1, document_id=$2, owner=$3, note=NULLIF($4,''), resolved_at=now()
		WHERE id=$5`, req.Action, docArg, ownerArg, req.Note, id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := tx.Commit(); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	detail := req.Action
	if ownerArg != nil {
		detail = "gap → " + ownerArg.(string)
	}
	h.audit(u.ID, "evidence.resolve", "evidence_request", id, detail)
	writeJSON(w, http.StatusOK, map[string]string{"status": req.Action})
}

func (h *Handler) DeleteEvidenceRequest(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	res, err := h.DB.Exec(`DELETE FROM evidence_requests WHERE id = $1`, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeErr(w, http.StatusNotFound, "evidence request not found")
		return
	}
	h.audit(auth.UserFrom(r.Context()).ID, "evidence.delete", "evidence_request", id, "")
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
