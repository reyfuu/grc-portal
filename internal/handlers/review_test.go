package handlers

import "testing"

func TestDecideReview(t *testing.T) {
	cases := []struct {
		name     string
		current  string // current item decision
		status   string // campaign status
		decision string // requested decision
		role     string
		justify  string
		wantErr  error
	}{
		{"certify pending active", "pending", "active", "certified", "reviewer", "", nil},
		{"admin certify", "pending", "active", "certified", "admin", "", nil},
		{"revoke without justification", "pending", "active", "revoked", "reviewer", "", errJustificationRequired},
		{"revoke with justification", "pending", "active", "revoked", "reviewer", "left the company", nil},
		{"revoke whitespace justification", "pending", "active", "revoked", "reviewer", "   ", errJustificationRequired},
		{"draft campaign", "pending", "draft", "certified", "reviewer", "", errCampaignNotActive},
		{"closed campaign", "pending", "closed", "certified", "reviewer", "", errCampaignNotActive},
		{"already decided", "certified", "active", "revoked", "admin", "x", errAlreadyDecided},
		{"readonly cannot decide", "pending", "active", "certified", "readonly", "", errPermission},
		{"unknown role", "pending", "active", "certified", "", "", errPermission},
		{"bad decision value", "pending", "active", "approve", "admin", "", errBadDecision},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := decideReview(c.current, c.status, c.decision, c.role, c.justify)
			if got != c.wantErr {
				t.Fatalf("decideReview() = %v, want %v", got, c.wantErr)
			}
		})
	}
}
