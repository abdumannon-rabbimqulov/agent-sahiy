// Brauzerdagi frontend uchun CORS: ruxsat etilgan manzillar
// (.env: CORS_ORIGIN) va preflight (OPTIONS) javobi.
package main

import (
	"net/http"
	"os"
	"strings"
)

// corsOrigins - ruxsat etilgan frontend manzillari (.env: CORS_ORIGIN,
// vergul bilan; "*" — hammasi). Bo'sh bo'lsa Vite dev serveri.
func corsOrigins() []string {
	v := os.Getenv("CORS_ORIGIN")
	if v == "" {
		return []string{"http://localhost:5173", "http://127.0.0.1:5173"}
	}
	var out []string
	for _, s := range strings.Split(v, ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// withCORS - brauzerdagi frontend uchun CORS sarlavhalari va preflight javobi.
func withCORS(next http.Handler) http.Handler {
	allowed := corsOrigins()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		for _, a := range allowed {
			if a == "*" || a == origin {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				if a == "*" && origin == "" {
					w.Header().Set("Access-Control-Allow-Origin", "*")
				}
				w.Header().Set("Vary", "Origin")
				break
			}
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Max-Age", "86400")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
