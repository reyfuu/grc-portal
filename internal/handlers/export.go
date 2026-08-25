package handlers

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"time"
)

// ExportCampaign streams the review results as CSV. ?decision=revoked gives the
// remediation (revoke) list handed to application owners.
func (h *Handler) ExportCampaign(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	q := `
		SELECT ri.snap_app, ri.snap_subject, ri.snap_role, ri.decision,
		       COALESCE(pu.display_name, pu.email, ''), ri.decided_at, COALESCE(ri.justification,'')
		FROM review_items ri
		LEFT JOIN portal_users pu ON pu.id = ri.reviewer_id
		WHERE ri.campaign_id = $1`
	if r.URL.Query().Get("decision") == "revoked" {
		q += ` AND ri.decision = 'revoked'`
	}
	q += ` ORDER BY ri.snap_app, ri.snap_subject, ri.snap_role`

	rows, err := h.DB.Query(q, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="campaign-%d.csv"`, id))
	cw := csv.NewWriter(w)
	defer cw.Flush()
	_ = cw.Write([]string{"application", "subject", "role", "decision", "reviewer", "decided_at", "justification"})

	for rows.Next() {
		var app, subj, role, decision, reviewer, just string
		var decidedAt *time.Time
		if err := rows.Scan(&app, &subj, &role, &decision, &reviewer, &decidedAt, &just); err != nil {
			return // header already sent; best-effort
		}
		da := ""
		if decidedAt != nil {
			da = decidedAt.UTC().Format(time.RFC3339)
		}
		_ = cw.Write([]string{app, subj, role, decision, reviewer, da, just})
	}
}
