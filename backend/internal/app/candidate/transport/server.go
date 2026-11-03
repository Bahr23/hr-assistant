package transport

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	authservice "hr-assistant/backend/internal/app/auth/service"
	candidateservice "hr-assistant/backend/internal/app/candidate/service"
	"hr-assistant/backend/internal/app/candidate/transport/contract"
	"hr-assistant/backend/internal/core/domain"

	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	nethttpmiddleware "github.com/oapi-codegen/nethttp-middleware"
)

type Server struct {
	sessions   *authservice.Service
	candidates *candidateservice.Service
	logger     *slog.Logger
}

type Config struct {
	Auth    *authservice.Service
	Service *candidateservice.Service
	Logger  *slog.Logger
}

var _ contract.StrictServerInterface = (*Server)(nil)

func Router(config Config) http.Handler {
	server := &Server{sessions: config.Auth, candidates: config.Service, logger: config.Logger}
	if server.logger == nil {
		server.logger = slog.Default()
	}
	router := chi.NewRouter()
	strict := contract.NewStrictHandlerWithOptions(
		server,
		[]contract.StrictMiddlewareFunc{server.requireHRSession},
		contract.StrictHTTPServerOptions{
			RequestErrorHandlerFunc:  server.requestError,
			ResponseErrorHandlerFunc: server.responseError,
		},
	)
	spec, err := contract.GetSwagger()
	if err != nil {
		panic("embedded candidate OpenAPI contract is invalid: " + err.Error())
	}
	validator := nethttpmiddleware.OapiRequestValidatorWithOptions(spec, &nethttpmiddleware.Options{
		Options:              openapi3filter.Options{AuthenticationFunc: openapi3filter.NoopAuthenticationFunc},
		DoNotValidateServers: true,
		Prefix:               "/api/v1",
		ErrorHandlerWithOpts: func(_ context.Context, err error, w http.ResponseWriter, r *http.Request, options nethttpmiddleware.ErrorHandlerOpts) {
			writeContractError(w, r, options.StatusCode, "invalid_request", "Request does not match the API contract", map[string]any{"reason": err.Error()})
		},
	})
	router.Use(validator)
	wrapper := contract.ServerInterfaceWrapper{Handler: strict, ErrorHandlerFunc: server.requestError}
	router.Get("/", wrapper.ListCandidates)
	router.Get("/{sessionId}", wrapper.GetCandidate)
	router.Post("/{sessionId}/report/rebuild", wrapper.RebuildCandidateReport)
	router.Post("/{sessionId}/answers/{answerId}/evaluation/retry", wrapper.RetryAnswerEvaluation)
	return router
}

type principalContextKey struct{}

func principalFromContext(ctx context.Context) authservice.Principal {
	value, _ := ctx.Value(principalContextKey{}).(authservice.Principal)
	return value
}

func (s *Server) requireHRSession(next contract.StrictHandlerFunc, _ string) contract.StrictHandlerFunc {
	return func(ctx context.Context, w http.ResponseWriter, r *http.Request, request any) (any, error) {
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			writeContractError(w, r, http.StatusUnauthorized, "unauthenticated", "Authentication is required", nil)
			return nil, nil
		}
		principal, err := s.sessions.Authenticate(ctx, strings.TrimSpace(strings.TrimPrefix(header, "Bearer ")))
		if err != nil {
			var value *domain.Fault
			if errors.As(err, &value) && value.Kind == domain.FaultUnauthenticated {
				writeContractError(w, r, http.StatusUnauthorized, "unauthenticated", "Authentication is required", nil)
				return nil, nil
			}
			return nil, err
		}
		return next(context.WithValue(ctx, principalContextKey{}, principal), w, r, request)
	}
}

func (s *Server) requestError(w http.ResponseWriter, r *http.Request, err error) {
	writeContractError(w, r, http.StatusBadRequest, "invalid_request", "Request does not match the API contract", map[string]any{"reason": err.Error()})
}

func (s *Server) responseError(w http.ResponseWriter, r *http.Request, err error) {
	s.logger.Error("candidate response failed", "error", err, "request_id", middleware.GetReqID(r.Context()))
	writeContractError(w, r, http.StatusInternalServerError, "internal_error", "An internal error occurred", nil)
}

func writeContractError(w http.ResponseWriter, r *http.Request, status int, code, message string, details map[string]any) {
	value := contractError(r.Context(), code, message, details)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func contractError(ctx context.Context, code, message string, details map[string]any) contract.Error {
	var result contract.Error
	result.Error.Code = code
	result.Error.Message = message
	if requestID := middleware.GetReqID(ctx); requestID != "" {
		result.Error.RequestId = &requestID
	}
	if details != nil {
		result.Error.Details = &details
	}
	return result
}

func fault(ctx context.Context, err error) (domain.FaultKind, contract.Error) {
	var value *domain.Fault
	if errors.As(err, &value) {
		details := map[string]any{}
		if value.Field != "" {
			details["field"] = value.Field
		}
		if len(details) == 0 {
			details = nil
		}
		return value.Kind, contractError(ctx, value.Code, value.Message, details)
	}
	return "", contractError(ctx, "internal_error", "An internal error occurred", nil)
}
