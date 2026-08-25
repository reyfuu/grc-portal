package handlers

import (
	"net/http"
	"strings"
	"time"

	"grcportal/internal/auth"
	"grcportal/internal/db"
)

type Application struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Owner       string    `json:"owner"`
	Department  string    `json:"department"`
	CreatedAt   time.Time `json:"created_at"`
}

// Departemen yang dikenal. Kosong = belum diklasifikasi. Nilai lain ditolak.
var validDepartments = map[string]bool{"": true, "keuangan": true, "marketing": true, "hr": true}

// normDepartment lowercases/trims and validates. Returns (value, ok).
func normDepartment(s string) (string, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	return s, validDepartments[s]
}

type Role struct {
	ID            int64  `json:"id"`
	ApplicationID *int64 `json:"application_id"`
	Name          string `json:"name"`
	Description   string `json:"description"`
}

func (h *Handler) ListApplications(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Query(`
		SELECT id, name, COALESCE(description,''), COALESCE(owner,''), COALESCE(department,''), created_at
		FROM applications ORDER BY name`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := []Application{}
	for rows.Next() {
		var a Application
		if err := rows.Scan(&a.ID, &a.Name, &a.Description, &a.Owner, &a.Department, &a.CreatedAt); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, a)
	}
	writeJSON(w, http.StatusOK, out)
}

type appReq struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Owner       string `json:"owner"`
	Department  string `json:"department"`
}

func (h *Handler) CreateApplication(w http.ResponseWriter, r *http.Request) {
	var req appReq
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	dept, ok := normDepartment(req.Department)
	if !ok {
		writeErr(w, http.StatusBadRequest, "department must be one of: keuangan, marketing, hr")
		return
	}
	var a Application
	err := h.DB.QueryRow(`
		INSERT INTO applications(name, description, owner, department) VALUES($1,$2,$3,NULLIF($4,''))
		RETURNING id, name, COALESCE(description,''), COALESCE(owner,''), COALESCE(department,''), created_at`,
		req.Name, req.Description, req.Owner, dept).
		Scan(&a.ID, &a.Name, &a.Description, &a.Owner, &a.Department, &a.CreatedAt)
	if db.IsUniqueViolation(err) {
		writeErr(w, http.StatusConflict, "an application with that name already exists")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(auth.UserFrom(r.Context()).ID, "application.create", "application", a.ID, a.Name)
	writeJSON(w, http.StatusCreated, a)
}

func (h *Handler) GetApplication(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	var a Application
	err := h.DB.QueryRow(`
		SELECT id, name, COALESCE(description,''), COALESCE(owner,''), COALESCE(department,''), created_at
		FROM applications WHERE id = $1`, id).
		Scan(&a.ID, &a.Name, &a.Description, &a.Owner, &a.Department, &a.CreatedAt)
	if err != nil {
		writeErr(w, http.StatusNotFound, "application not found")
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func (h *Handler) UpdateApplication(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req appReq
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	dept, ok := normDepartment(req.Department)
	if !ok {
		writeErr(w, http.StatusBadRequest, "department must be one of: keuangan, marketing, hr")
		return
	}
	var a Application
	err := h.DB.QueryRow(`
		UPDATE applications SET name = COALESCE(NULLIF($2,''), name), description = $3, owner = $4, department = NULLIF($5,'')
		WHERE id = $1
		RETURNING id, name, COALESCE(description,''), COALESCE(owner,''), COALESCE(department,''), created_at`,
		id, strings.TrimSpace(req.Name), req.Description, req.Owner, dept).
		Scan(&a.ID, &a.Name, &a.Description, &a.Owner, &a.Department, &a.CreatedAt)
	if db.IsUniqueViolation(err) {
		writeErr(w, http.StatusConflict, "an application with that name already exists")
		return
	}
	if err != nil {
		writeErr(w, http.StatusNotFound, "application not found")
		return
	}
	h.audit(auth.UserFrom(r.Context()).ID, "application.update", "application", a.ID, a.Name)
	writeJSON(w, http.StatusOK, a)
}

func (h *Handler) ListAppRoles(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	h.queryRoles(w, `
		SELECT id, application_id, name, COALESCE(description,'')
		FROM roles WHERE application_id = $1 ORDER BY name`, id)
}

func (h *Handler) ListRoles(w http.ResponseWriter, r *http.Request) {
	h.queryRoles(w, `
		SELECT id, application_id, name, COALESCE(description,'')
		FROM roles ORDER BY application_id NULLS FIRST, name`)
}

func (h *Handler) queryRoles(w http.ResponseWriter, q string, args ...any) {
	rows, err := h.DB.Query(q, args...)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := []Role{}
	for rows.Next() {
		var role Role
		if err := rows.Scan(&role.ID, &role.ApplicationID, &role.Name, &role.Description); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, role)
	}
	writeJSON(w, http.StatusOK, out)
}

type roleReq struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (h *Handler) CreateAppRole(w http.ResponseWriter, r *http.Request) {
	appID, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req roleReq
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	var role Role
	err := h.DB.QueryRow(`
		INSERT INTO roles(application_id, name, description) VALUES($1,$2,$3)
		RETURNING id, application_id, name, COALESCE(description,'')`,
		appID, req.Name, req.Description).
		Scan(&role.ID, &role.ApplicationID, &role.Name, &role.Description)
	if db.IsUniqueViolation(err) {
		writeErr(w, http.StatusConflict, "that role already exists for this application")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(auth.UserFrom(r.Context()).ID, "role.create", "role", role.ID, role.Name)
	writeJSON(w, http.StatusCreated, role)
}
