package handlers

import (
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"grcportal/internal/auth"
)

// Handler holds shared dependencies for all HTTP handlers.
type Handler struct {
	DB     *sql.DB
	Secure bool // set Secure flag on cookies (disable for local http dev)
}

func New(db *sql.DB, secure bool) *Handler { return &Handler{DB: db, Secure: secure} }

// RegisterRoutes mounts every /api endpoint. Non-/api paths are served by the SPA (see main).
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	any := auth.RequireRole()          // any authenticated user
	admin := auth.RequireRole("admin") // admin only
	review := auth.RequireRole("admin", "reviewer")
	reader := auth.RequireRole("admin", "readonly")

	// public
	mux.HandleFunc("GET /api/health", h.Health)
	mux.HandleFunc("POST /api/login", h.Login)
	mux.HandleFunc("POST /api/logout", h.Logout)
	mux.HandleFunc("GET /api/me", h.Me)

	// applications + roles
	mux.HandleFunc("GET /api/applications", any(h.ListApplications))
	mux.HandleFunc("POST /api/applications", admin(h.CreateApplication))
	mux.HandleFunc("GET /api/applications/{id}", any(h.GetApplication))
	mux.HandleFunc("PATCH /api/applications/{id}", admin(h.UpdateApplication))
	mux.HandleFunc("GET /api/applications/{id}/roles", any(h.ListAppRoles))
	mux.HandleFunc("POST /api/applications/{id}/roles", admin(h.CreateAppRole))
	mux.HandleFunc("GET /api/roles", any(h.ListRoles))

	// subjects + bindings + matrix
	mux.HandleFunc("GET /api/subjects", any(h.ListSubjects))
	mux.HandleFunc("POST /api/subjects", admin(h.CreateSubject))
	mux.HandleFunc("GET /api/bindings", any(h.ListBindings))
	mux.HandleFunc("POST /api/bindings", admin(h.CreateBinding))
	mux.HandleFunc("DELETE /api/bindings/{id}", admin(h.DeleteBinding))
	mux.HandleFunc("GET /api/matrix", any(h.Matrix))

	// import / export
	mux.HandleFunc("POST /api/import/bindings", admin(h.ImportBindings))
	mux.HandleFunc("POST /api/import/hr", admin(h.ImportHR))
	mux.HandleFunc("GET /api/campaigns/{id}/export", reader(h.ExportCampaign))

	// hr directory + access reconciliation (UC-02: match access vs HR active status)
	mux.HandleFunc("GET /api/hr-employees", any(h.ListHREmployees))
	mux.HandleFunc("POST /api/hr-employees", admin(h.CreateHREmployee))
	mux.HandleFunc("DELETE /api/hr-employees/{id}", admin(h.DeleteHREmployee))
	mux.HandleFunc("GET /api/access-review", any(h.AccessReview))

	// campaigns + review
	mux.HandleFunc("GET /api/campaigns", any(h.ListCampaigns))
	mux.HandleFunc("POST /api/campaigns", admin(h.CreateCampaign))
	mux.HandleFunc("GET /api/campaigns/{id}", any(h.GetCampaign))
	mux.HandleFunc("POST /api/campaigns/{id}/activate", admin(h.ActivateCampaign))
	mux.HandleFunc("POST /api/campaigns/{id}/close", admin(h.CloseCampaign))
	mux.HandleFunc("GET /api/campaigns/{id}/items", any(h.ListItems))
	mux.HandleFunc("POST /api/campaigns/{id}/items/bulk-decide", review(h.BulkDecide))
	mux.HandleFunc("POST /api/review-items/{id}/decide", review(h.DecideItem))

	// audit
	mux.HandleFunc("GET /api/audit", reader(h.ListAudit))

	// documents (SOP & kebijakan / evidence) + AI RAG search
	mux.HandleFunc("GET /api/documents", any(h.ListDocuments))
	mux.HandleFunc("POST /api/documents", admin(h.CreateDocument))
	mux.HandleFunc("DELETE /api/documents/{id}", admin(h.DeleteDocument))
	mux.HandleFunc("GET /api/documents/search", any(h.SearchDocuments))
	mux.HandleFunc("POST /api/documents/ask", any(h.AskDocuments))

	// evidence requests (UC-01: reuse audit/assessment evidence + route gaps)
	mux.HandleFunc("GET /api/evidence-requests", any(h.ListEvidenceRequests))
	mux.HandleFunc("POST /api/evidence-requests", admin(h.CreateEvidenceRequest))
	mux.HandleFunc("GET /api/evidence-requests/{id}", any(h.GetEvidenceRequest))
	mux.HandleFunc("POST /api/evidence-requests/{id}/resolve", review(h.ResolveEvidenceRequest))
	mux.HandleFunc("DELETE /api/evidence-requests/{id}", admin(h.DeleteEvidenceRequest))
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// readJSON decodes a JSON body. It enforces application/json content-type, which
// (together with the SameSite=Lax session cookie) blocks cross-site form CSRF.
func readJSON(r *http.Request, v any) error {
	ct := r.Header.Get("Content-Type")
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = ct[:i]
	}
	if strings.TrimSpace(ct) != "application/json" {
		return errBadContentType
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	return dec.Decode(v)
}

func pathID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func itoa(i int) string { return strconv.Itoa(i) }

// audit writes one audit_log row. Best-effort: failures don't block the request.
func (h *Handler) audit(actorID int64, action, entityType string, entityID int64, detail string) {
	var eid any
	if entityID > 0 {
		eid = entityID
	}
	_, _ = h.DB.Exec(
		`INSERT INTO audit_log(actor_id, action, entity_type, entity_id, detail) VALUES($1,$2,$3,$4,$5)`,
		actorID, action, entityType, eid, detail)
}
