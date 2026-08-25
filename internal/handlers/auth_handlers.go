package handlers

import (
	"database/sql"
	"net/http"
	"strings"

	"grcportal/internal/auth"
)

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	if err := h.DB.Ping(); err != nil {
		writeErr(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type loginReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var (
		id     int64
		hash   string
		active bool
		role   string
		dn     sql.NullString
	)
	err := h.DB.QueryRow(
		`SELECT id, password_hash, is_active, role, display_name FROM portal_users WHERE email = $1`,
		strings.ToLower(strings.TrimSpace(req.Email))).
		Scan(&id, &hash, &active, &role, &dn)
	// Constant-ish response: same 401 whether the user is missing or the password is wrong.
	if err != nil || !active || !auth.CheckPassword(hash, req.Password) {
		writeErr(w, http.StatusUnauthorized, "invalid email or password")
		return
	}
	sid, exp, err := auth.CreateSession(h.DB, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not create session")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "sid",
		Value:    sid,
		Path:     "/",
		Expires:  exp,
		HttpOnly: true,
		Secure:   h.Secure,
		SameSite: http.SameSiteLaxMode,
	})
	h.audit(id, "login", "portal_user", id, "")
	writeJSON(w, http.StatusOK, &auth.User{ID: id, Email: strings.ToLower(req.Email), DisplayName: dn.String, Role: role})
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie("sid"); err == nil {
		_ = auth.DeleteSession(h.DB, c.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "sid",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.Secure,
		SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	if u == nil {
		writeErr(w, http.StatusUnauthorized, "not logged in")
		return
	}
	writeJSON(w, http.StatusOK, u)
}
