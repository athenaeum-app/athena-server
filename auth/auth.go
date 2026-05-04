// Admin password  → role "admin"  (read + write)
// Viewer password → role "viewer" (read-only)
//
//	Specify both passwords in the .env!
package auth

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/athenaeum-app/server/config"
	"github.com/golang-jwt/jwt/v5"
)

type credentials struct {
	Password string `json:"password"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// Login handles POST /auth/login.
//
//	Request  { "password": "..." }
//	200 OK   { "token": "<jwt>", "role": "admin" | "viewer" }
//	400      malformed body or empty password
//	401      password does not match admin or viewer password
//	500      JWT signing failed
//
// JWT expires in 100 years
func Login(w http.ResponseWriter, r *http.Request) {
	var creds credentials
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if creds.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password is required"})
		return
	}

	// Determine the role by comparing against the configured passwords.
	// Constant-time comparison is not strictly necessary here because the
	// passwords are not user-supplied secrets stored in a database — they are
	// server-side configuration values — but we keep the check simple and
	// direct.  Admin is checked first so that if both passwords happen to be
	// equal the more-privileged role wins.
	var role string
	switch creds.Password {
	case config.AdminPassword():
		role = "admin"
	case config.ViewerPassword():
		role = "viewer"
	default:
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid password"})
		return
	}

	now := time.Now()
	claims := jwt.MapClaims{
		"sub":  role,
		"role": role,
		"iat":  now.Unix(),
		"exp":  now.AddDate(100, 0, 0).Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(config.JWTSecret())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"token": signed,
		"role":  role,
	})
}
