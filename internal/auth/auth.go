package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"net/http"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// User is the authenticated portal account attached to a request context.
type User struct {
	ID          int64  `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
}

type ctxKey int

const userKey ctxKey = 0

// SessionTTL is how long a login stays valid.
const SessionTTL = 12 * time.Hour

func HashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(b), err
}

func CheckPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

// CreateSession issues an opaque session id and stores it.
func CreateSession(db *sql.DB, userID int64) (string, time.Time, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", time.Time{}, err
	}
	id := hex.EncodeToString(b)
	exp := time.Now().Add(SessionTTL).UTC()
	_, err := db.Exec(`INSERT INTO sessions(id, portal_user_id, expires_at) VALUES($1,$2,$3)`, id, userID, exp)
	return id, exp, err
}

// LookupSession returns the active user for a session id, or an error if none.
func LookupSession(db *sql.DB, sid string) (*User, error) {
	var u User
	var dn sql.NullString
	err := db.QueryRow(`
		SELECT u.id, u.email, u.display_name, u.role
		FROM sessions s JOIN portal_users u ON u.id = s.portal_user_id
		WHERE s.id = $1 AND s.expires_at > now() AND u.is_active = TRUE`, sid).
		Scan(&u.ID, &u.Email, &dn, &u.Role)
	if err != nil {
		return nil, err
	}
	u.DisplayName = dn.String
	return &u, nil
}

func DeleteSession(db *sql.DB, sid string) error {
	_, err := db.Exec(`DELETE FROM sessions WHERE id = $1`, sid)
	return err
}

// Middleware attaches the authenticated user (if any) to the request context.
// It never rejects — route guards (RequireRole) do that.
func Middleware(db *sql.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if c, err := r.Cookie("sid"); err == nil {
				if u, err := LookupSession(db, c.Value); err == nil {
					r = r.WithContext(context.WithValue(r.Context(), userKey, u))
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// UserFrom returns the authenticated user, or nil.
func UserFrom(ctx context.Context) *User {
	u, _ := ctx.Value(userKey).(*User)
	return u
}

// RequireRole guards a handler. No roles = any authenticated user; otherwise the
// user's role must be in the allowed set. Enforced server-side regardless of UI.
func RequireRole(roles ...string) func(http.HandlerFunc) http.HandlerFunc {
	allowed := make(map[string]bool, len(roles))
	for _, r := range roles {
		allowed[r] = true
	}
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			u := UserFrom(r.Context())
			if u == nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			if len(allowed) > 0 && !allowed[u.Role] {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			next(w, r)
		}
	}
}
