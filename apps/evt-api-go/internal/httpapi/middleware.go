package httpapi

import (
	"fmt"
	"net/http"
)

func withCORS(next http.Handler) http.Handler {
	allowed := map[string]bool{
		// Local dev (Expo web / local browser)
		"http://localhost:8081":  true,
		"http://localhost:19006": true,

		// Your real domains (browser origin)
		"https://cvera.app":                            true,
		"https://www.cvera.app":                        true,
		"https://cvera-iiw-mvp-expo-app--mvp.expo.app": true,
		"https://mvp.cvera.app":                        true,
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		fmt.Printf("testing 123 %s \n\n", origin)
		// Only set CORS when a browser sends an Origin header and it’s allowed.
		if origin != "" && allowed[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}

		// Preflight requests
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// JSON endpoints accept metadata and compact credentials, never file contents.
// Match the existing verification endpoint's limit before decoding any body.
func withRequestLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		next.ServeHTTP(w, r)
	})
}
