package web

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/BlinovDev/rc-setup-hub/backend/api"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func NewRouter(logger *slog.Logger, ping func(context.Context) error) *chi.Mux {
	r := chi.NewRouter()
	r.Use(requestLogging(logger))
	r.Use(panicRecovery(logger))
	r.Get("/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		_, _ = w.Write(api.OpenAPI)
	})
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := ping(ctx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("{\"status\":\"unavailable\"}\n"))
			return
		}
		_, _ = w.Write([]byte("{\"status\":\"ok\"}\n"))
	})
	return r
}

func requestLogging(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			wrapped := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(wrapped, r)
			status := wrapped.Status()
			if status == 0 {
				status = http.StatusOK
			}
			logger.Info("http request", "method", r.Method, "path", r.URL.Path,
				"status", status, "duration", time.Since(start))
		})
	}
}

func panicRecovery(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					if err == http.ErrAbortHandler {
						panic(err)
					}
					logger.Error("http panic", "panic", err)
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte("{\"error\":\"internal server error\"}\n"))
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
