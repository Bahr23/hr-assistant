package transport

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"hr-assistant/backend/internal/app/auth/service"
	"hr-assistant/backend/internal/app/auth/transport/contract"
	"hr-assistant/backend/internal/core/domain"

	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	nethttpmiddleware "github.com/oapi-codegen/nethttp-middleware"
)

type Server struct {
	sessions       *service.Service
	logger         *slog.Logger
	frontendOrigin string
	secureCookies  bool
	authLimiter    *ipRateLimiter
}

type Config struct {
	Service        *service.Service
	Logger         *slog.Logger
	FrontendOrigin string
	SecureCookies  bool
}

var _ contract.StrictServerInterface = (*Server)(nil)

func Router(config Config) http.Handler {
	server := &Server{
		sessions: config.Service, logger: config.Logger,
		frontendOrigin: strings.TrimRight(config.FrontendOrigin, "/"),
		secureCookies:  config.SecureCookies, authLimiter: newIPRateLimiter(10, time.Minute),
	}
	if server.logger == nil {
		server.logger = slog.Default()
	}
	router := chi.NewRouter()
	strict := contract.NewStrictHandlerWithOptions(
		server,
		[]contract.StrictMiddlewareFunc{server.operationContext},
		contract.StrictHTTPServerOptions{
			RequestErrorHandlerFunc:  server.requestError,
			ResponseErrorHandlerFunc: server.responseError,
		},
	)
	spec, err := contract.GetSwagger()
	if err != nil {
		panic("embedded auth OpenAPI contract is invalid: " + err.Error())
	}
	validator := nethttpmiddleware.OapiRequestValidatorWithOptions(spec, &nethttpmiddleware.Options{
		Options:              openapi3filter.Options{AuthenticationFunc: openapi3filter.NoopAuthenticationFunc},
		DoNotValidateServers: true,
		Prefix:               "/api/v1",
		ErrorHandlerWithOpts: func(_ context.Context, err error, w http.ResponseWriter, r *http.Request, options nethttpmiddleware.ErrorHandlerOpts) {
			writeContractError(w, r, options.StatusCode, "invalid_request", "Request does not match the API contract", map[string]any{"reason": err.Error()})
		},
	})
	router.Use(server.limitAuthAttempts, validator)
	wrapper := contract.ServerInterfaceWrapper{Handler: strict, ErrorHandlerFunc: server.requestError}
	router.Post("/register", wrapper.Register)
	router.Post("/login", wrapper.Login)
	router.Post("/refresh", wrapper.Refresh)
	router.Post("/logout", wrapper.Logout)
	router.Get("/me", wrapper.Me)
	return router
}

type principalContextKey struct{}
type requestContextKey struct{}

func principalFromContext(ctx context.Context) service.Principal {
	value, _ := ctx.Value(principalContextKey{}).(service.Principal)
	return value
}

func requestFromContext(ctx context.Context) *http.Request {
	value, _ := ctx.Value(requestContextKey{}).(*http.Request)
	return value
}

func (s *Server) operationContext(next contract.StrictHandlerFunc, operationID string) contract.StrictHandlerFunc {
	return func(ctx context.Context, w http.ResponseWriter, r *http.Request, request any) (any, error) {
		ctx = context.WithValue(ctx, requestContextKey{}, r)
		if operationID != "Me" {
			return next(ctx, w, r, request)
		}
		principal, err := s.authenticate(ctx, r)
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

func (s *Server) authenticate(ctx context.Context, r *http.Request) (service.Principal, error) {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		return service.Principal{}, domain.NewFault(domain.FaultUnauthenticated, "unauthenticated", "authentication is required")
	}
	return s.sessions.Authenticate(ctx, strings.TrimSpace(strings.TrimPrefix(header, "Bearer ")))
}

func (s *Server) limitAuthAttempts(next http.Handler) http.Handler {
	limited := s.authLimiter.middleware(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && (r.URL.Path == "/api/v1/auth/register" || r.URL.Path == "/api/v1/auth/login") {
			limited.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requestError(w http.ResponseWriter, r *http.Request, err error) {
	writeContractError(w, r, http.StatusBadRequest, "invalid_request", "Request does not match the API contract", map[string]any{"reason": err.Error()})
}

func (s *Server) responseError(w http.ResponseWriter, r *http.Request, err error) {
	s.logger.Error("auth response failed", "error", err, "request_id", middleware.GetReqID(r.Context()))
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

func (s *Server) originAllowed(r *http.Request) bool {
	if r == nil {
		return false
	}
	origin := r.Header.Get("Origin")
	return origin == "" || origin == s.frontendOrigin
}
