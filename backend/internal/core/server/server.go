package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type readinessChecker interface {
	Ping(context.Context) error
}

type Config struct {
	Readiness      readinessChecker
	Logger         *slog.Logger
	FrontendOrigin string
}

func New(config Config) *chi.Mux {
	root := &runtime{
		readiness: config.Readiness, logger: config.Logger,
		frontendOrigin: strings.TrimRight(config.FrontendOrigin, "/"),
	}
	if root.logger == nil {
		root.logger = slog.Default()
	}
	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(root.cors)
	router.Use(root.requestLogger)
	router.Use(middleware.Recoverer)
	router.Use(middleware.Timeout(30 * time.Second))
	router.Get("/health/live", root.live)
	router.Get("/health/ready", root.ready)
	return router
}

type runtime struct {
	readiness      readinessChecker
	logger         *slog.Logger
	frontendOrigin string
}

func (s *runtime) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && origin == s.frontendOrigin {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
			w.Header().Add("Vary", "Origin")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *runtime) requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		wrapped := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(wrapped, r)
		s.logger.Info("http request", "method", r.Method, "path", r.URL.Path,
			"status", wrapped.Status(), "duration_ms", time.Since(started).Milliseconds(),
			"request_id", middleware.GetReqID(r.Context()))
	})
}

func (s *runtime) live(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *runtime) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if s.readiness == nil || s.readiness.Ping(ctx) != nil {
		writeJSON(w, http.StatusServiceUnavailable, errorBody("not_ready", "Database is unavailable", middleware.GetReqID(r.Context())))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func errorBody(code, message, requestID string) map[string]any {
	return map[string]any{"error": map[string]any{"code": code, "message": message, "request_id": requestID}}
}
