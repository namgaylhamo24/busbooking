package main

import (
	"log"
	"net/http"
	"time"
)

// statusRecorder remembers the status code so the logger can print it.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// withMiddleware adds request logging and panic recovery around every handler.
func withMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		defer func() {
			if p := recover(); p != nil {
				log.Printf("panic: %v", p)
				writeError(rec, http.StatusInternalServerError, "internal server error")
			}
			log.Printf("%s %s -> %d (%v)", r.Method, r.URL.Path, rec.status,
				time.Since(start).Round(time.Millisecond))
		}()

		next.ServeHTTP(rec, r)
	})
}
