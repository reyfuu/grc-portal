package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"grcportal/internal/auth"
	"grcportal/internal/db"
)

type Subject struct {
	ID          int64  `json:"id"`
	Email       string `json:"email"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Type        string `json:"type"`
}

type Binding struct {
	ID          int64  `json:"id"`
	Application string `json:"application"`
	AppID       int64  `json:"application_id"`
	Subject     string `json:"subject"`
	SubjectID   int64  `json:"subject_id"`
	Role        string `json:"role"`
	RoleID      int64  `json:"role_id"`
	Source      string `json:"source"`
	Status      string `json:"status"`
}

func (h *Handler) ListSubjects(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Query(`
		SELECT id, COALESCE(email,''), COALESCE(username,''), COALESCE(display_name,''), type
		FROM subject_identities ORDER BY COALESCE(email, username)`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := []Subject{}
	for rows.Next() {
		var s Subject
		if err := rows.Scan(&s.ID, &s.Email, &s.Username, &s.DisplayName, &s.Type); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, s)
	}
	writeJSON(w, http.StatusOK, out)
}

type subjectReq struct {
	Email       string `json:"email"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Type        string `json:"type"`
}

func (h *Handler) CreateSubject(w http.ResponseWriter, r *http.Request) {
	var req subjectReq
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.Username = strings.TrimSpace(req.Username)
	if req.Email == "" && req.Username == "" {
		writeErr(w, http.StatusBadRequest, "email or username is required")
		return
	}
	if req.Type == "" {
		req.Type = "user"
	}
	if req.Type != "user" && req.Type != "service_account" {
		writeErr(w, http.StatusBadRequest, "type must be 'user' or 'service_account'")
		return
	}
	id, err := upsertSubject(h.DB, req.Email, req.Username, req.DisplayName, req.Type)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(auth.UserFrom(r.Context()).ID, "subject.create", "subject", id, req.Email+req.Username)
	writeJSON(w, http.StatusCreated, Subject{ID: id, Email: req.Email, Username: req.Username, DisplayName: req.DisplayName, Type: req.Type})
}

func (h *Handler) ListBindings(w http.ResponseWriter, r *http.Request) {
	var where []string
	var args []any
	if v := r.URL.Query().Get("application_id"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			args = append(args, id)
			where = append(where, "b.application_id = $"+strconv.Itoa(len(args)))
		}
	}
	if v := r.URL.Query().Get("subject_id"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			args = append(args, id)
			where = append(where, "b.subject_id = $"+strconv.Itoa(len(args)))
		}
	}
	q := `
		SELECT b.id, a.name, a.id, COALESCE(si.email, si.username, ''), si.id, r.name, r.id, b.source, b.status
		FROM access_bindings b
		JOIN applications a ON a.id = b.application_id
		JOIN subject_identities si ON si.id = b.subject_id
		JOIN roles r ON r.id = b.role_id`
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY a.name, COALESCE(si.email, si.username), r.name"
	rows, err := h.DB.Query(q, args...)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := []Binding{}
	for rows.Next() {
		var b Binding
		if err := rows.Scan(&b.ID, &b.Application, &b.AppID, &b.Subject, &b.SubjectID, &b.Role, &b.RoleID, &b.Source, &b.Status); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, b)
	}
	writeJSON(w, http.StatusOK, out)
}

type bindingReq struct {
	ApplicationID int64  `json:"application_id"`
	SubjectID     int64  `json:"subject_id"`
	RoleID        int64  `json:"role_id"`
	GrantedAt     string `json:"granted_at"`
}

func (h *Handler) CreateBinding(w http.ResponseWriter, r *http.Request) {
	var req bindingReq
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.ApplicationID == 0 || req.SubjectID == 0 || req.RoleID == 0 {
		writeErr(w, http.StatusBadRequest, "application_id, subject_id and role_id are required")
		return
	}
	// The role must belong to this application (or be a global role, application_id NULL).
	var roleApp *int64
	if err := h.DB.QueryRow(`SELECT application_id FROM roles WHERE id = $1`, req.RoleID).Scan(&roleApp); err != nil {
		writeErr(w, http.StatusBadRequest, "role not found")
		return
	}
	if roleApp != nil && *roleApp != req.ApplicationID {
		writeErr(w, http.StatusBadRequest, "role does not belong to that application")
		return
	}
	var id int64
	err := h.DB.QueryRow(`
		INSERT INTO access_bindings(application_id, subject_id, role_id, source, granted_at)
		VALUES($1,$2,$3,'manual',NULLIF($4,''))
		RETURNING id`,
		req.ApplicationID, req.SubjectID, req.RoleID, req.GrantedAt).Scan(&id)
	if db.IsUniqueViolation(err) {
		writeErr(w, http.StatusConflict, "that access grant already exists")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(auth.UserFrom(r.Context()).ID, "binding.create", "binding", id, "")
	writeJSON(w, http.StatusCreated, map[string]int64{"id": id})
}

func (h *Handler) DeleteBinding(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	res, err := h.DB.Exec(`DELETE FROM access_bindings WHERE id = $1`, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeErr(w, http.StatusNotFound, "binding not found")
		return
	}
	h.audit(auth.UserFrom(r.Context()).ID, "binding.delete", "binding", id, "")
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// Matrix returns the subjects × roles grid for one application ("who has what").
func (h *Handler) Matrix(w http.ResponseWriter, r *http.Request) {
	appID, err := strconv.ParseInt(r.URL.Query().Get("application_id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "application_id is required")
		return
	}
	var app Application
	if err := h.DB.QueryRow(`
		SELECT id, name, COALESCE(description,''), COALESCE(owner,''), created_at
		FROM applications WHERE id = $1`, appID).
		Scan(&app.ID, &app.Name, &app.Description, &app.Owner, &app.CreatedAt); err != nil {
		writeErr(w, http.StatusNotFound, "application not found")
		return
	}

	roleRows, err := h.DB.Query(`SELECT id, application_id, name, COALESCE(description,'') FROM roles WHERE application_id = $1 ORDER BY name`, appID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer roleRows.Close()
	roles := []Role{}
	for roleRows.Next() {
		var role Role
		if err := roleRows.Scan(&role.ID, &role.ApplicationID, &role.Name, &role.Description); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		roles = append(roles, role)
	}

	// active bindings for this app: subject -> set of role ids
	bindRows, err := h.DB.Query(`
		SELECT si.id, COALESCE(si.email, si.username, ''), b.role_id
		FROM access_bindings b
		JOIN subject_identities si ON si.id = b.subject_id
		WHERE b.application_id = $1 AND b.status = 'active'
		ORDER BY COALESCE(si.email, si.username)`, appID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer bindRows.Close()

	type matrixRow struct {
		SubjectID int64   `json:"subject_id"`
		Subject   string  `json:"subject"`
		RoleIDs   []int64 `json:"role_ids"`
	}
	order := []int64{}
	byID := map[int64]*matrixRow{}
	for bindRows.Next() {
		var sid, roleID int64
		var subj string
		if err := bindRows.Scan(&sid, &subj, &roleID); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		row := byID[sid]
		if row == nil {
			row = &matrixRow{SubjectID: sid, Subject: subj, RoleIDs: []int64{}}
			byID[sid] = row
			order = append(order, sid)
		}
		row.RoleIDs = append(row.RoleIDs, roleID)
	}
	out := make([]*matrixRow, 0, len(order))
	for _, sid := range order {
		out = append(out, byID[sid])
	}
	writeJSON(w, http.StatusOK, map[string]any{"application": app, "roles": roles, "rows": out})
}
