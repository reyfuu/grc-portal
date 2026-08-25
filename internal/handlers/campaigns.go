package handlers

import (
	"net/http"
	"strings"
	"time"

	"grcportal/internal/auth"
)

type Campaign struct {
	ID          int64      `json:"id"`
	Name        string     `json:"name"`
	PeriodStart string     `json:"period_start"`
	PeriodEnd   string     `json:"period_end"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	ClosedAt    *time.Time `json:"closed_at"`
}

type ReviewItem struct {
	ID            int64      `json:"id"`
	CampaignID    int64      `json:"campaign_id"`
	Subject       string     `json:"subject"`
	App           string     `json:"app"`
	Role          string     `json:"role"`
	Reviewer      string     `json:"reviewer"`
	Decision      string     `json:"decision"`
	Justification string     `json:"justification"`
	DecidedAt     *time.Time `json:"decided_at"`
}

func (h *Handler) ListCampaigns(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Query(`
		SELECT id, name, COALESCE(period_start,''), COALESCE(period_end,''), status, created_at, closed_at
		FROM review_campaigns ORDER BY created_at DESC`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := []Campaign{}
	for rows.Next() {
		var c Campaign
		if err := rows.Scan(&c.ID, &c.Name, &c.PeriodStart, &c.PeriodEnd, &c.Status, &c.CreatedAt, &c.ClosedAt); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, c)
	}
	writeJSON(w, http.StatusOK, out)
}

type campaignReq struct {
	Name           string  `json:"name"`
	PeriodStart    string  `json:"period_start"`
	PeriodEnd      string  `json:"period_end"`
	ApplicationIDs []int64 `json:"application_ids"`
}

func (h *Handler) CreateCampaign(w http.ResponseWriter, r *http.Request) {
	var req campaignReq
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	if len(req.ApplicationIDs) == 0 {
		writeErr(w, http.StatusBadRequest, "at least one application is required")
		return
	}
	u := auth.UserFrom(r.Context())
	tx, err := h.DB.Begin()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()

	var id int64
	if err := tx.QueryRow(`
		INSERT INTO review_campaigns(name, period_start, period_end, created_by)
		VALUES($1, NULLIF($2,''), NULLIF($3,''), $4) RETURNING id`,
		req.Name, req.PeriodStart, req.PeriodEnd, u.ID).Scan(&id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, appID := range req.ApplicationIDs {
		if _, err := tx.Exec(`
			INSERT INTO campaign_applications(campaign_id, application_id) VALUES($1,$2)
			ON CONFLICT DO NOTHING`, id, appID); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid application id")
			return
		}
	}
	if err := tx.Commit(); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(u.ID, "campaign.create", "campaign", id, req.Name)
	writeJSON(w, http.StatusCreated, map[string]int64{"id": id})
}

func (h *Handler) GetCampaign(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	var c Campaign
	if err := h.DB.QueryRow(`
		SELECT id, name, COALESCE(period_start,''), COALESCE(period_end,''), status, created_at, closed_at
		FROM review_campaigns WHERE id = $1`, id).
		Scan(&c.ID, &c.Name, &c.PeriodStart, &c.PeriodEnd, &c.Status, &c.CreatedAt, &c.ClosedAt); err != nil {
		writeErr(w, http.StatusNotFound, "campaign not found")
		return
	}
	// in-scope applications
	appRows, err := h.DB.Query(`
		SELECT a.id, a.name FROM campaign_applications ca
		JOIN applications a ON a.id = ca.application_id
		WHERE ca.campaign_id = $1 ORDER BY a.name`, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer appRows.Close()
	type appRef struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	apps := []appRef{}
	for appRows.Next() {
		var a appRef
		if err := appRows.Scan(&a.ID, &a.Name); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		apps = append(apps, a)
	}
	// decision counts
	var total, pending, certified, revoked int
	_ = h.DB.QueryRow(`
		SELECT COUNT(*),
		       COUNT(*) FILTER (WHERE decision='pending'),
		       COUNT(*) FILTER (WHERE decision='certified'),
		       COUNT(*) FILTER (WHERE decision='revoked')
		FROM review_items WHERE campaign_id = $1`, id).Scan(&total, &pending, &certified, &revoked)

	writeJSON(w, http.StatusOK, map[string]any{
		"campaign":     c,
		"applications": apps,
		"counts": map[string]int{
			"total": total, "pending": pending, "certified": certified, "revoked": revoked,
		},
	})
}

// ActivateCampaign snapshots the current active bindings (for the in-scope apps)
// into immutable review_items, then flips the campaign to active.
func (h *Handler) ActivateCampaign(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	tx, err := h.DB.Begin()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()

	var status string
	if err := tx.QueryRow(`SELECT status FROM review_campaigns WHERE id = $1 FOR UPDATE`, id).Scan(&status); err != nil {
		writeErr(w, http.StatusNotFound, "campaign not found")
		return
	}
	if status != "draft" {
		writeErr(w, http.StatusConflict, "only draft campaigns can be activated")
		return
	}

	// Snapshot. reviewer_id is mapped from the application owner's email when it
	// matches a portal user; otherwise left NULL (any reviewer/admin can act on it).
	if _, err := tx.Exec(`
		INSERT INTO review_items(campaign_id, binding_id, snap_subject, snap_app, snap_role, reviewer_id)
		SELECT $1, b.id,
		       COALESCE(si.email, si.username, ''),
		       a.name, r.name,
		       (SELECT pu.id FROM portal_users pu WHERE lower(pu.email) = lower(a.owner) LIMIT 1)
		FROM access_bindings b
		JOIN applications a       ON a.id  = b.application_id
		JOIN subject_identities si ON si.id = b.subject_id
		JOIN roles r              ON r.id  = b.role_id
		WHERE b.status = 'active'
		  AND b.application_id IN (SELECT application_id FROM campaign_applications WHERE campaign_id = $1)
		ON CONFLICT (campaign_id, binding_id) DO NOTHING`, id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := tx.Exec(`UPDATE review_campaigns SET status = 'active' WHERE id = $1`, id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	var count int
	_ = tx.QueryRow(`SELECT COUNT(*) FROM review_items WHERE campaign_id = $1`, id).Scan(&count)
	if err := tx.Commit(); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(auth.UserFrom(r.Context()).ID, "campaign.activate", "campaign", id, "")
	writeJSON(w, http.StatusOK, map[string]any{"status": "active", "items": count})
}

func (h *Handler) CloseCampaign(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	res, err := h.DB.Exec(`
		UPDATE review_campaigns SET status='closed', closed_at=now()
		WHERE id=$1 AND status='active'`, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeErr(w, http.StatusConflict, "only active campaigns can be closed")
		return
	}
	h.audit(auth.UserFrom(r.Context()).ID, "campaign.close", "campaign", id, "")
	writeJSON(w, http.StatusOK, map[string]string{"status": "closed"})
}

// ListItems returns review items for a campaign. Filters: reviewer=me, decision=<state>.
func (h *Handler) ListItems(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	args := []any{id}
	where := "ri.campaign_id = $1"
	if r.URL.Query().Get("reviewer") == "me" {
		args = append(args, auth.UserFrom(r.Context()).ID)
		where += " AND ri.reviewer_id = $2"
	}
	if d := r.URL.Query().Get("decision"); d == "pending" || d == "certified" || d == "revoked" {
		args = append(args, d)
		where += " AND ri.decision = $" + itoa(len(args))
	}
	rows, err := h.DB.Query(`
		SELECT ri.id, ri.campaign_id, ri.snap_subject, ri.snap_app, ri.snap_role,
		       COALESCE(pu.display_name, pu.email, ''), ri.decision, COALESCE(ri.justification,''), ri.decided_at
		FROM review_items ri
		LEFT JOIN portal_users pu ON pu.id = ri.reviewer_id
		WHERE `+where+`
		ORDER BY ri.snap_app, ri.snap_subject, ri.snap_role`, args...)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := []ReviewItem{}
	for rows.Next() {
		var it ReviewItem
		if err := rows.Scan(&it.ID, &it.CampaignID, &it.Subject, &it.App, &it.Role,
			&it.Reviewer, &it.Decision, &it.Justification, &it.DecidedAt); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, it)
	}
	writeJSON(w, http.StatusOK, out)
}
