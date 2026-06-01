package server

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions || r.URL.Path == "/api/health" || s.cfg.Server.AuthToken == "" {
			next.ServeHTTP(w, r)
			return
		}

		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if subtle.ConstantTimeCompare([]byte(got), []byte(s.cfg.Server.AuthToken)) != 1 {
			writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Missing or invalid bearer token", nil)
			return
		}
		next.ServeHTTP(w, r)
	})
}
