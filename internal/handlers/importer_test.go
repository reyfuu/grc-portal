package handlers

import "testing"

func TestParseImportRow(t *testing.T) {
	good := []string{"Salesforce", "a@co.id", "", "Ana", "user", "admin", "2026-01-01"}
	if _, err := parseImportRow(good); err != nil {
		t.Fatalf("valid row rejected: %v", err)
	}

	// malformed: wrong column count
	if _, err := parseImportRow([]string{"Salesforce", "a@co.id"}); err == nil {
		t.Fatal("expected error for wrong column count")
	}

	// missing both email and username
	bad := []string{"Salesforce", "", "", "", "user", "admin", ""}
	if _, err := parseImportRow(bad); err == nil {
		t.Fatal("expected error when subject_email and subject_username are both empty")
	}

	// invalid type
	badType := []string{"Salesforce", "a@co.id", "", "", "robot", "admin", ""}
	if _, err := parseImportRow(badType); err == nil {
		t.Fatal("expected error for invalid subject_type")
	}

	// username-only row defaults type to 'user'
	p, err := parseImportRow([]string{"Salesforce", "", "svc_backup", "", "", "viewer", ""})
	if err != nil {
		t.Fatalf("username-only row rejected: %v", err)
	}
	if p.typ != "user" {
		t.Fatalf("expected default type 'user', got %q", p.typ)
	}
}

func TestSameHeader(t *testing.T) {
	if !sameHeader(importHeader, importHeader) {
		t.Fatal("identical header should match")
	}
	if sameHeader([]string{"application"}, importHeader) {
		t.Fatal("short header should not match")
	}
}
