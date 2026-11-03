package transport

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

func TestCandidateRouterRequiresJWT(t *testing.T) {
	handler := chi.NewRouter()
	handler.Mount("/api/v1/candidates", Router(Config{}))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/candidates", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	require.Equal(t, http.StatusUnauthorized, response.Code)
}
