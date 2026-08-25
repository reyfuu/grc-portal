package handlers

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"net/http"
	"strings"

	"grcportal/internal/auth"
	"grcportal/internal/db"
)

// HREmployee is one person in the HR directory (source of truth for active status).
type HREmployee struct {
	ID          int64  `json:"id"`
	Email       string `json:"email"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Department  string `json:"department"`
	Status      string `json:"status"`
}

var validHRStatus = map[string]bool{"active": true, "inactive": true}

// hrStatus classifies an access subject against the HR directory. Pure (no DB) so it
// is unit-tested. Result feeds the access-review confirmation list; hrFlagged marks
// which results need human review.
//   - service_account -> "service"   (not an HR person; a missing match is expected)
//   - user, no match  -> "not_in_hr" (mismatch/orphan: has access but unknown to HR)
//   - user, inactive  -> "inactive"  (terminated but access still active)
//   - user, active    -> "active"
func hrStatus(subjectType, hrEmpStatus string, matched bool) string {
	if subjectType == "service_account" {
		return "service"
	}
	if !matched {
		return "not_in_hr"
	}
	if hrEmpStatus == "inactive" {
		return "inactive"
	}
	return "active"
}

func hrFlagged(status string) bool { return status == "inactive" || status == "not_in_hr" }

func (h *Handler) ListHREmployees(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Query(`
		SELECT id, COALESCE(email,''), COALESCE(username,''), COALESCE(display_name,''),
		       COALESCE(department,''), status
		FROM hr_employees ORDER BY display_name, email, username`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := []HREmployee{}
	for rows.Next() {
		var e HREmployee
		if err := rows.Scan(&e.ID, &e.Email, &e.Username, &e.DisplayName, &e.Department, &e.Status); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, e)
	}
	writeJSON(w, http.StatusOK, out)
}

type hrReq struct {
	Email       string `json:"email"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Department  string `json:"department"`
	Status      string `json:"status"`
}

func (h *Handler) CreateHREmployee(w http.ResponseWriter, r *http.Request) {
	var req hrReq
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.Username = strings.TrimSpace(req.Username)
	req.Status = strings.ToLower(strings.TrimSpace(req.Status))
	if req.Status == "" {
		req.Status = "active"
	}
	if req.Email == "" && req.Username == "" {
		writeErr(w, http.StatusBadRequest, "email or username is required")
		return
	}
	if !validHRStatus[req.Status] {
		writeErr(w, http.StatusBadRequest, "status must be 'active' or 'inactive'")
		return
	}
	var e HREmployee
	err := h.DB.QueryRow(`
		INSERT INTO hr_employees(email, username, display_name, department, status)
		VALUES(NULLIF($1,''), NULLIF($2,''), NULLIF($3,''), NULLIF($4,''), $5)
		RETURNING id, COALESCE(email,''), COALESCE(username,''), COALESCE(display_name,''), COALESCE(department,''), status`,
		req.Email, req.Username, req.DisplayName, req.Department, req.Status).
		Scan(&e.ID, &e.Email, &e.Username, &e.DisplayName, &e.Department, &e.Status)
	if db.IsUniqueViolation(err) {
		writeErr(w, http.StatusConflict, "an HR record with that email/username already exists")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(auth.UserFrom(r.Context()).ID, "hr.create", "hr_employee", e.ID, e.Email+e.Username)
	writeJSON(w, http.StatusCreated, e)
}

func (h *Handler) DeleteHREmployee(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	res, err := h.DB.Exec(`DELETE FROM hr_employees WHERE id = $1`, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeErr(w, http.StatusNotFound, "hr record not found")
		return
	}
	h.audit(auth.UserFrom(r.Context()).ID, "hr.delete", "hr_employee", id, "")
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// AccessReviewRow is one subject reconciled against HR.
type AccessReviewRow struct {
	SubjectID      int64  `json:"subject_id"`
	Subject        string `json:"subject"`
	Type           string `json:"type"`
	Department     string `json:"department"`
	HRStatus       string `json:"hr_status"`
	Flagged        bool   `json:"flagged"`
	ActiveBindings int    `json:"active_bindings"`
}

// AccessReview consolidates access subjects (those with active bindings) and matches
// them to the HR directory, flagging inactive/orphan accounts for confirmation. This
// only prepares the list — actual revoke/keep decisions stay in review Campaigns.
// ?flagged=1 returns only the accounts that need attention.
func (h *Handler) AccessReview(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Query(`
		SELECT si.id,
		       COALESCE(NULLIF(si.email,''), NULLIF(si.username,''), si.display_name, '') AS subject,
		       si.type, COALESCE(he.department,''), COALESCE(he.status,''), (he.id IS NOT NULL) AS matched,
		       COUNT(ab.id) FILTER (WHERE ab.status='active') AS active_bindings
		FROM subject_identities si
		LEFT JOIN LATERAL (
		    SELECT id, status, department FROM hr_employees h
		    WHERE (si.email    IS NOT NULL AND lower(h.email)    = lower(si.email))
		       OR (si.username IS NOT NULL AND lower(h.username) = lower(si.username))
		    ORDER BY (h.email IS NOT NULL AND lower(h.email) = lower(si.email)) DESC
		    LIMIT 1
		) he ON true
		LEFT JOIN access_bindings ab ON ab.subject_id = si.id
		GROUP BY si.id, he.id, he.status, he.department
		HAVING COUNT(ab.id) FILTER (WHERE ab.status='active') > 0
		ORDER BY subject`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	onlyFlagged := r.URL.Query().Get("flagged") == "1"
	out := []AccessReviewRow{}
	counts := map[string]int{"active": 0, "inactive": 0, "not_in_hr": 0, "service": 0}
	for rows.Next() {
		var row AccessReviewRow
		var heStatus string
		var matched bool
		if err := rows.Scan(&row.SubjectID, &row.Subject, &row.Type, &row.Department,
			&heStatus, &matched, &row.ActiveBindings); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		row.HRStatus = hrStatus(row.Type, heStatus, matched)
		row.Flagged = hrFlagged(row.HRStatus)
		counts[row.HRStatus]++
		if !onlyFlagged || row.Flagged {
			out = append(out, row)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"rows": out, "counts": counts})
}

// --- CSV import (reuses the readers from importer.go) ---

var hrImportHeader = []string{"email", "username", "display_name", "department", "status"}

type parsedHR struct{ email, username, displayName, department, status string }

// parseHRRow validates one HR CSV record. Pure (no DB), so it is unit-tested.
func parseHRRow(rec []string) (parsedHR, error) {
	if len(rec) != len(hrImportHeader) {
		return parsedHR{}, fmt.Errorf("expected %d columns, got %d", len(hrImportHeader), len(rec))
	}
	p := parsedHR{
		email:       strings.ToLower(strings.TrimSpace(rec[0])),
		username:    strings.TrimSpace(rec[1]),
		displayName: strings.TrimSpace(rec[2]),
		department:  strings.TrimSpace(rec[3]),
		status:      strings.ToLower(strings.TrimSpace(rec[4])),
	}
	if p.email == "" && p.username == "" {
		return p, fmt.Errorf("email or username is required")
	}
	if p.status == "" {
		p.status = "active"
	}
	if !validHRStatus[p.status] {
		return p, fmt.Errorf("status must be 'active' or 'inactive'")
	}
	return p, nil
}

func upsertHR(dbx *sql.DB, p parsedHR) error {
	if p.email != "" {
		_, err := dbx.Exec(`
			INSERT INTO hr_employees(email, username, display_name, department, status)
			VALUES($1, NULLIF($2,''), NULLIF($3,''), NULLIF($4,''), $5)
			ON CONFLICT (lower(email)) WHERE email IS NOT NULL
			DO UPDATE SET username=COALESCE(EXCLUDED.username, hr_employees.username),
			              display_name=COALESCE(EXCLUDED.display_name, hr_employees.display_name),
			              department=COALESCE(EXCLUDED.department, hr_employees.department),
			              status=EXCLUDED.status`,
			p.email, p.username, p.displayName, p.department, p.status)
		return err
	}
	_, err := dbx.Exec(`
		INSERT INTO hr_employees(email, username, display_name, department, status)
		VALUES(NULL, $1, NULLIF($2,''), NULLIF($3,''), $4)
		ON CONFLICT (lower(username)) WHERE email IS NULL
		DO UPDATE SET display_name=COALESCE(EXCLUDED.display_name, hr_employees.display_name),
		              department=COALESCE(EXCLUDED.department, hr_employees.department),
		              status=EXCLUDED.status`,
		p.username, p.displayName, p.department, p.status)
	return err
}

func (h *Handler) ImportHR(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxImportBytes)
	file, _, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "missing 'file' upload")
		return
	}
	defer file.Close()

	cr := csv.NewReader(file)
	cr.FieldsPerRecord = len(hrImportHeader)
	cr.TrimLeadingSpace = true

	header, err := cr.Read()
	if err != nil {
		writeErr(w, http.StatusBadRequest, "could not read CSV header")
		return
	}
	if !sameHeader(header, hrImportHeader) {
		writeErr(w, http.StatusBadRequest, "unexpected header; expected: "+strings.Join(hrImportHeader, ","))
		return
	}

	res := importResult{Errors: []importError{}}
	line := 1
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
		p, perr := parseHRRow(rec)
		if perr != nil {
			res.Errors = append(res.Errors, importError{Line: line, Msg: perr.Error()})
			continue
		}
		if err := upsertHR(h.DB, p); err != nil {
			res.Errors = append(res.Errors, importError{Line: line, Msg: err.Error()})
			continue
		}
		res.Imported++
	}

	h.audit(auth.UserFrom(r.Context()).ID, "csv.import", "hr_employee", 0,
		fmt.Sprintf("imported=%d errors=%d", res.Imported, len(res.Errors)))
	writeJSON(w, http.StatusOK, res)
}
