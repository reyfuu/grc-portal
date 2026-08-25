package handlers

import (
	"net/http"
	"time"
)

type AuditEntry struct {
	ID         int64     `json:"id"`
	Actor      string    `json:"actor"`
	Action     string    `json:"action"`
	EntityType string    `json:"entity_type"`
	EntityID   *int64    `json:"entity_id"`
	Detail     string    `json:"detail"`
	Department string    `json:"department"` // departemen app terkait; "" jika tak terkait app
	CreatedAt  time.Time `json:"created_at"`
}

func (h *Handler) ListAudit(w http.ResponseWriter, r *http.Request) {
	// Resolve the department the event touches: directly when it's an application
	// event, or via the review item's snapshot app name for decision events.
	rows, err := h.DB.Query(`
		SELECT al.id, COALESCE(pu.email,''), al.action, COALESCE(al.entity_type,''),
		       al.entity_id, COALESCE(al.detail,''),
		       COALESCE(app_direct.department, app_ri.department, '') AS department,
		       al.created_at
		FROM audit_log al
		LEFT JOIN portal_users pu ON pu.id = al.actor_id
		LEFT JOIN applications app_direct ON al.entity_type = 'application' AND app_direct.id = al.entity_id
		LEFT JOIN review_items ri ON al.entity_type = 'review_item' AND ri.id = al.entity_id
		LEFT JOIN applications app_ri ON app_ri.name = ri.snap_app
		ORDER BY al.id DESC LIMIT 500`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := []AuditEntry{}
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(&e.ID, &e.Actor, &e.Action, &e.EntityType, &e.EntityID, &e.Detail, &e.Department, &e.CreatedAt); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, e)
	}
	writeJSON(w, http.StatusOK, out)
}
