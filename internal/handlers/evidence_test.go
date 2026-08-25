package handlers

import "testing"

func TestResolveEvidence(t *testing.T) {
	cases := []struct {
		name    string
		status  string // current request status
		action  string
		role    string
		hasDoc  bool
		owner   string
		wantErr error
	}{
		{"fulfill with doc", "open", "fulfilled", "reviewer", true, "", nil},
		{"admin fulfill", "open", "fulfilled", "admin", true, "", nil},
		{"fulfill without doc", "open", "fulfilled", "reviewer", false, "", errEvidenceDocRequired},
		{"gap with owner", "open", "gap", "reviewer", false, "Infrastructure", nil},
		{"gap without owner", "open", "gap", "reviewer", false, "", errEvidenceOwnerRequired},
		{"gap whitespace owner", "open", "gap", "reviewer", false, "   ", errEvidenceOwnerRequired},
		{"already fulfilled", "fulfilled", "gap", "admin", false, "x", errEvidenceResolved},
		{"already gap", "gap", "fulfilled", "admin", true, "", errEvidenceResolved},
		{"readonly cannot resolve", "open", "fulfilled", "readonly", true, "", errEvidencePermission},
		{"unknown role", "open", "fulfilled", "", true, "", errEvidencePermission},
		{"bad action", "open", "approve", "admin", true, "", errBadEvidenceAction},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := resolveEvidence(c.status, c.action, c.role, c.hasDoc, c.owner)
			if got != c.wantErr {
				t.Fatalf("resolveEvidence() = %v, want %v", got, c.wantErr)
			}
		})
	}
}
