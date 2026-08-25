package handlers

import (
	"database/sql"
	"net/http"
	"strings"

	"grcportal/internal/auth"
)

// decideReview validates a review decision. Pure function (no DB) so it is unit-tested.
func decideReview(currentDecision, campaignStatus, decision, reviewerRole, justification string) error {
	if reviewerRole != "admin" && reviewerRole != "reviewer" {
		return errPermission
	}
	if campaignStatus != "active" {
		return errCampaignNotActive
	}
	if currentDecision != "pending" {
		return errAlreadyDecided
	}
	switch decision {
	case "certified":
		return nil
	case "revoked":
		if strings.TrimSpace(justification) == "" {
			return errJustificationRequired
		}
		return nil
	default:
		return errBadDecision
	}
}

func writeDecideErr(w http.ResponseWriter, err error) {
	switch err {
	case errPermission:
		writeErr(w, http.StatusForbidden, err.Error())
	case errCampaignNotActive, errAlreadyDecided:
		writeErr(w, http.StatusConflict, err.Error())
	default:
		writeErr(w, http.StatusBadRequest, err.Error())
	}
}

type decideReq struct {
	Decision      string `json:"decision"`
	Justification string `json:"justification"`
}

func (h *Handler) DecideItem(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req decideReq
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	u := auth.UserFrom(r.Context())

	tx, err := h.DB.Begin()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()

	var cur, status string
	var bindingID sql.NullInt64
	if err := tx.QueryRow(`
		SELECT ri.decision, rc.status, ri.binding_id
		FROM review_items ri JOIN review_campaigns rc ON rc.id = ri.campaign_id
		WHERE ri.id = $1 FOR UPDATE OF ri`, id).Scan(&cur, &status, &bindingID); err != nil {
		writeErr(w, http.StatusNotFound, "review item not found")
		return
	}
	if err := decideReview(cur, status, req.Decision, u.Role, req.Justification); err != nil {
		writeDecideErr(w, err)
		return
	}
	if _, err := tx.Exec(`
		UPDATE review_items SET decision=$1, justification=NULLIF($2,''), reviewer_id=$3, decided_at=now()
		WHERE id=$4`, req.Decision, req.Justification, u.ID, id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Revoking flips the binding so the matrix reflects it. Remediation in the source
	// app is manual (export the revoke list) — there is no live integration yet.
	if req.Decision == "revoked" && bindingID.Valid {
		if _, err := tx.Exec(`UPDATE access_bindings SET status='revoked' WHERE id=$1`, bindingID.Int64); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if err := tx.Commit(); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(u.ID, "decision", "review_item", id, req.Decision)
	writeJSON(w, http.StatusOK, map[string]string{"decision": req.Decision})
}

// BulkDecide certifies every pending item the caller may act on (admin: all;
// reviewer: only items assigned to them). Revoke is per-item (needs justification).
func (h *Handler) BulkDecide(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req struct {
		Decision string `json:"decision"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Decision != "certified" {
		writeErr(w, http.StatusBadRequest, "bulk decide supports only 'certified'")
		return
	}
	u := auth.UserFrom(r.Context())

	var status string
	if err := h.DB.QueryRow(`SELECT status FROM review_campaigns WHERE id=$1`, id).Scan(&status); err != nil {
		writeErr(w, http.StatusNotFound, "campaign not found")
		return
	}
	if status != "active" {
		writeErr(w, http.StatusConflict, errCampaignNotActive.Error())
		return
	}
	q := `UPDATE review_items SET decision='certified', reviewer_id=$2, decided_at=now()
	      WHERE campaign_id=$1 AND decision='pending'`
	if u.Role != "admin" {
		q += ` AND reviewer_id=$2`
	}
	res, err := h.DB.Exec(q, id, u.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	n, _ := res.RowsAffected()
	h.audit(u.ID, "decision.bulk", "campaign", id, "certified")
	writeJSON(w, http.StatusOK, map[string]int64{"certified": n})
}
