package handlers

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"net/http"
	"strings"

	"grcportal/internal/auth"
)

const maxImportBytes = 5 << 20 // 5 MB

var importHeader = []string{
	"application", "subject_email", "subject_username",
	"subject_display_name", "subject_type", "role", "granted_at",
}

// upsertSubject inserts or updates an identity, keyed by lower(email) or, when no
// email, by lower(username). Returns the row id.
func upsertSubject(db *sql.DB, email, username, displayName, typ string) (int64, error) {
	if typ == "" {
		typ = "user"
	}
	var id int64
	if email != "" {
		err := db.QueryRow(`
			INSERT INTO subject_identities(email, username, display_name, type)
			VALUES($1, NULLIF($2,''), NULLIF($3,''), $4)
			ON CONFLICT (lower(email)) WHERE email IS NOT NULL
			DO UPDATE SET username     = COALESCE(EXCLUDED.username, subject_identities.username),
			              display_name = COALESCE(EXCLUDED.display_name, subject_identities.display_name),
			              type         = EXCLUDED.type
			RETURNING id`, email, username, displayName, typ).Scan(&id)
		return id, err
	}
	err := db.QueryRow(`
		INSERT INTO subject_identities(email, username, display_name, type)
		VALUES(NULL, $1, NULLIF($2,''), $3)
		ON CONFLICT (lower(username)) WHERE email IS NULL
		DO UPDATE SET display_name = COALESCE(EXCLUDED.display_name, subject_identities.display_name),
		              type         = EXCLUDED.type
		RETURNING id`, username, displayName, typ).Scan(&id)
	return id, err
}

// upsertRole inserts or returns a role by (application_id, name).
func upsertRole(db *sql.DB, appID int64, name string) (int64, error) {
	var id int64
	err := db.QueryRow(`
		INSERT INTO roles(application_id, name) VALUES($1,$2)
		ON CONFLICT (application_id, name) DO UPDATE SET name = EXCLUDED.name
		RETURNING id`, appID, name).Scan(&id)
	return id, err
}

type importError struct {
	Line int    `json:"line"`
	Msg  string `json:"msg"`
}

type importResult struct {
	Imported int           `json:"imported"`
	Skipped  int           `json:"skipped"`
	Errors   []importError `json:"errors"`
}

func (h *Handler) ImportBindings(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxImportBytes)
	file, _, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "missing 'file' upload")
		return
	}
	defer file.Close()

	cr := csv.NewReader(file)
	cr.FieldsPerRecord = len(importHeader)
	cr.TrimLeadingSpace = true

	header, err := cr.Read()
	if err != nil {
		writeErr(w, http.StatusBadRequest, "could not read CSV header")
		return
	}
	if !sameHeader(header, importHeader) {
		writeErr(w, http.StatusBadRequest, "unexpected header; expected: "+strings.Join(importHeader, ","))
		return
	}

	// Preload application name -> id so we never silently create apps from a typo.
	apps, err := h.appNameIndex()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	res := importResult{Errors: []importError{}}
	line := 1 // header was line 1
	for {
		rec, err := cr.Read()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			line++
			res.Errors = append(res.Errors, importError{Line: line, Msg: "malformed row: " + err.Error()})
			continue
		}
		line++
		if isBlankRow(rec) {
			continue
		}
		imported, skipped, ierr := h.importRow(rec, apps)
		switch {
		case ierr != nil:
			res.Errors = append(res.Errors, importError{Line: line, Msg: ierr.Error()})
		case skipped:
			res.Skipped++
		case imported:
			res.Imported++
		}
	}

	h.audit(auth.UserFrom(r.Context()).ID, "csv.import", "binding", 0,
		fmt.Sprintf("imported=%d skipped=%d errors=%d", res.Imported, res.Skipped, len(res.Errors)))
	writeJSON(w, http.StatusOK, res)
}

type parsedRow struct {
	appName, email, username, displayName, typ, roleName, grantedAt string
}

// parseImportRow validates a CSV record and returns cleaned fields. Pure (no DB),
// so it is unit-tested. It does not check that the application exists — that needs
// the DB and happens in importRow.
func parseImportRow(rec []string) (parsedRow, error) {
	if len(rec) != len(importHeader) {
		return parsedRow{}, fmt.Errorf("expected %d columns, got %d", len(importHeader), len(rec))
	}
	p := parsedRow{
		appName:     strings.TrimSpace(rec[0]),
		email:       strings.ToLower(strings.TrimSpace(rec[1])),
		username:    strings.TrimSpace(rec[2]),
		displayName: strings.TrimSpace(rec[3]),
		typ:         strings.TrimSpace(rec[4]),
		roleName:    strings.TrimSpace(rec[5]),
		grantedAt:   strings.TrimSpace(rec[6]),
	}
	if p.appName == "" {
		return p, fmt.Errorf("application is required")
	}
	if p.email == "" && p.username == "" {
		return p, fmt.Errorf("subject_email or subject_username is required")
	}
	if p.roleName == "" {
		return p, fmt.Errorf("role is required")
	}
	if p.typ == "" {
		p.typ = "user"
	}
	if p.typ != "user" && p.typ != "service_account" {
		return p, fmt.Errorf("subject_type must be 'user' or 'service_account'")
	}
	return p, nil
}

// importRow processes one CSV record. Returns (imported, skipped, error).
func (h *Handler) importRow(rec []string, apps map[string]int64) (bool, bool, error) {
	p, err := parseImportRow(rec)
	if err != nil {
		return false, false, err
	}
	appID, ok := apps[strings.ToLower(p.appName)]
	if !ok {
		return false, false, fmt.Errorf("unknown application %q (create it first)", p.appName)
	}
	subjectID, err := upsertSubject(h.DB, p.email, p.username, p.displayName, p.typ)
	if err != nil {
		return false, false, fmt.Errorf("subject: %v", err)
	}
	roleID, err := upsertRole(h.DB, appID, p.roleName)
	if err != nil {
		return false, false, fmt.Errorf("role: %v", err)
	}
	res, err := h.DB.Exec(`
		INSERT INTO access_bindings(application_id, subject_id, role_id, source, granted_at)
		VALUES($1,$2,$3,'csv',NULLIF($4,''))
		ON CONFLICT (application_id, subject_id, role_id) DO NOTHING`,
		appID, subjectID, roleID, p.grantedAt)
	if err != nil {
		return false, false, fmt.Errorf("binding: %v", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return false, true, nil // duplicate — idempotent skip
	}
	return true, false, nil
}

func (h *Handler) appNameIndex() (map[string]int64, error) {
	rows, err := h.DB.Query(`SELECT id, name FROM applications`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[string]int64{}
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		m[strings.ToLower(name)] = id
	}
	return m, nil
}

func sameHeader(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if !strings.EqualFold(strings.TrimSpace(got[i]), want[i]) {
			return false
		}
	}
	return true
}

func isBlankRow(rec []string) bool {
	for _, f := range rec {
		if strings.TrimSpace(f) != "" {
			return false
		}
	}
	return true
}
