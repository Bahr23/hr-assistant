package server

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

type readinessStub struct{ err error }

func (s readinessStub) Ping(context.Context) error { return s.err }

func TestHealthEndpoints(t *testing.T) {
	handler := New(Config{Readiness: readinessStub{}, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})

	live := httptest.NewRecorder()
	handler.ServeHTTP(live, httptest.NewRequest(http.MethodGet, "/health/live", nil))
	require.Equal(t, http.StatusOK, live.Code)

	ready := httptest.NewRecorder()
	handler.ServeHTTP(ready, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	require.Equal(t, http.StatusOK, ready.Code)
}

func TestReadinessFailsWhenDatabaseIsUnavailable(t *testing.T) {
	handler := New(Config{Readiness: readinessStub{err: errors.New("database unavailable")}})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	require.Equal(t, http.StatusServiceUnavailable, response.Code)
}
