package handlers

import "testing"

func TestHRStatus(t *testing.T) {
	cases := []struct {
		name    string
		typ     string
		hrStat  string
		matched bool
		want    string
		flagged bool
	}{
		{"active user", "user", "active", true, "active", false},
		{"inactive user keeps access", "user", "inactive", true, "inactive", true},
		{"user not in HR", "user", "", false, "not_in_hr", true},
		{"service account, no match expected", "service_account", "", false, "service", false},
		{"service account even if matched", "service_account", "active", true, "service", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := hrStatus(c.typ, c.hrStat, c.matched)
			if got != c.want {
				t.Fatalf("hrStatus() = %q, want %q", got, c.want)
			}
			if hrFlagged(got) != c.flagged {
				t.Fatalf("hrFlagged(%q) = %v, want %v", got, hrFlagged(got), c.flagged)
			}
		})
	}
}

func TestParseHRRow(t *testing.T) {
	if _, err := parseHRRow([]string{"a@co.id", "", "Ana", "Keuangan", "active"}); err != nil {
		t.Fatalf("valid row rejected: %v", err)
	}
	// wrong column count
	if _, err := parseHRRow([]string{"a@co.id", "active"}); err == nil {
		t.Fatal("expected error for wrong column count")
	}
	// both email and username empty
	if _, err := parseHRRow([]string{"", "", "", "", "active"}); err == nil {
		t.Fatal("expected error when email and username both empty")
	}
	// invalid status
	if _, err := parseHRRow([]string{"a@co.id", "", "", "", "cuti"}); err == nil {
		t.Fatal("expected error for invalid status")
	}
	// empty status defaults to active
	p, err := parseHRRow([]string{"", "svc_ana", "", "", ""})
	if err != nil {
		t.Fatalf("username-only row rejected: %v", err)
	}
	if p.status != "active" {
		t.Fatalf("expected default status 'active', got %q", p.status)
	}
}
