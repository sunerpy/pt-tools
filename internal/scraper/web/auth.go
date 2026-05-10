package web

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
)

func BasicAuthMiddleware(username, password string) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			u, p, ok := r.BasicAuth()
			if !ok || subtle.ConstantTimeCompare([]byte(u), []byte(username)) != 1 || subtle.ConstantTimeCompare([]byte(p), []byte(password)) != 1 {
				w.Header().Set("WWW-Authenticate", `Basic realm="scraper"`)
				writeError(w, http.StatusUnauthorized, "Unauthorized")
				return
			}
			next(w, r)
		}
	}
}

func APIKeyMiddleware(key string) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			got := r.Header.Get("X-API-Key")
			if got == "" {
				got = r.URL.Query().Get("api_key")
			}
			if subtle.ConstantTimeCompare([]byte(got), []byte(key)) != 1 {
				writeError(w, http.StatusUnauthorized, "Unauthorized")
				return
			}
			next(w, r)
		}
	}
}

func SessionAuthMiddleware(checkSession func(*http.Request) bool) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if checkSession == nil || !checkSession(r) {
				writeError(w, http.StatusUnauthorized, "Unauthorized")
				return
			}
			next(w, r)
		}
	}
}

func NoAuthMiddleware() func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc { return next }
}

func GenerateAPIKey() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
