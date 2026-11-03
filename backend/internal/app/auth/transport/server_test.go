package transport

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

func TestOpenAPIContractRejectsUnknownRequestFields(t *testing.T) {
	handler := chi.NewRouter()
	handler.Mount("/api/v1/auth", Router(Config{}))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(`{
		"email":"hr@example.com","password":"strong-password","unknown":true
	}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	require.Equal(t, http.StatusBadRequest, response.Code)
}
