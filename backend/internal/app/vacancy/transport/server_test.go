package transport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"hr-assistant/backend/internal/app/vacancy/transport/contract"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

func TestOpenAPIContractRejectsInvalidPaginationBeforeAuthentication(t *testing.T) {
	handler := chi.NewRouter()
	handler.Mount("/api/v1/vacancies", Router(Config{}))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/vacancies?limit=not-a-number", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	require.Equal(t, http.StatusBadRequest, response.Code)
}

func TestMasterTokenAuthorizesPublicVacancyOperation(t *testing.T) {
	server := &Server{masterAPIToken: "master-secret"}
	called := false
	next := func(_ context.Context, _ http.ResponseWriter, _ *http.Request, _ any) (any, error) {
		called = true
		return contract.GetVacancyWelcomeMessage200JSONResponse{
			WelcomeMessage: "Welcome", AlreadyAttempted: false,
		}, nil
	}
	handler := server.requireHRSession(next, "GetVacancyWelcomeMessage")
	request := httptest.NewRequest(http.MethodGet, "/api/v1/vacancies/public/link-token", nil)
	request.Header.Set("Authorization", "Bearer master-secret")
	response := httptest.NewRecorder()

	_, err := handler(context.Background(), response, request, nil)

	require.NoError(t, err)
	require.True(t, called)
}

func TestMasterTokenDoesNotMatchAnotherValue(t *testing.T) {
	server := &Server{masterAPIToken: "master-secret"}
	require.False(t, server.matchesMasterToken("another-secret"))
}
