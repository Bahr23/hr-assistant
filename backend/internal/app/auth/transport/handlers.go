package transport

import (
	"context"
	"net/http"
	"time"

	"hr-assistant/backend/internal/app/auth/service"
	"hr-assistant/backend/internal/app/auth/transport/contract"
	"hr-assistant/backend/internal/core/domain"

	openapi_types "github.com/oapi-codegen/runtime/types"
)

const refreshCookieName = "hr_refresh"

func (s *Server) Register(ctx context.Context, request contract.RegisterRequestObject) (contract.RegisterResponseObject, error) {
	if request.Body == nil {
		value := contractError(ctx, "invalid_request", "Request body is required", nil)
		return contract.Register422JSONResponse{ValidationErrorJSONResponse: contract.ValidationErrorJSONResponse(value)}, nil
	}
	grant, err := s.sessions.Open(ctx, service.RegisterHR{
		Email: string(request.Body.Email), Password: request.Body.Password,
	})
	if err != nil {
		kind, value := fault(ctx, err)
		if kind == domain.FaultConflict {
			return contract.Register409JSONResponse{ConflictJSONResponse: contract.ConflictJSONResponse(value)}, nil
		}
		if kind == domain.FaultValidation {
			return contract.Register422JSONResponse{ValidationErrorJSONResponse: contract.ValidationErrorJSONResponse(value)}, nil
		}
		return nil, err
	}
	cookie := s.refreshCookie(grant.RefreshToken, grant.RefreshExpiresAt).String()
	return contract.Register201JSONResponse{
		Body: authResponse(grant), Headers: contract.Register201ResponseHeaders{SetCookie: &cookie},
	}, nil
}

func (s *Server) Login(ctx context.Context, request contract.LoginRequestObject) (contract.LoginResponseObject, error) {
	if request.Body == nil {
		value := contractError(ctx, "unauthenticated", "Invalid authentication credentials", nil)
		return contract.Login401JSONResponse{UnauthorizedJSONResponse: contract.UnauthorizedJSONResponse(value)}, nil
	}
	grant, err := s.sessions.Open(ctx, service.PasswordLogin{Email: string(request.Body.Email), Password: request.Body.Password})
	if err != nil {
		kind, value := fault(ctx, err)
		if kind == domain.FaultUnauthenticated || kind == domain.FaultValidation {
			return contract.Login401JSONResponse{UnauthorizedJSONResponse: contract.UnauthorizedJSONResponse(value)}, nil
		}
		return nil, err
	}
	cookie := s.refreshCookie(grant.RefreshToken, grant.RefreshExpiresAt).String()
	return contract.Login200JSONResponse{
		Body: authResponse(grant), Headers: contract.Login200ResponseHeaders{SetCookie: &cookie},
	}, nil
}

func (s *Server) Refresh(ctx context.Context, _ contract.RefreshRequestObject) (contract.RefreshResponseObject, error) {
	rawRequest := requestFromContext(ctx)
	if !s.originAllowed(rawRequest) {
		value := contractError(ctx, "origin_not_allowed", "Request origin is not allowed", nil)
		return contract.Refresh403JSONResponse{ForbiddenJSONResponse: contract.ForbiddenJSONResponse(value)}, nil
	}
	cookie, err := rawRequest.Cookie(refreshCookieName)
	if err != nil || cookie.Value == "" {
		value := contractError(ctx, "unauthenticated", "Refresh token is required", nil)
		return contract.Refresh401JSONResponse{UnauthorizedJSONResponse: contract.UnauthorizedJSONResponse(value)}, nil
	}
	grant, err := s.sessions.Open(ctx, service.Refresh{Token: cookie.Value})
	if err != nil {
		kind, value := fault(ctx, err)
		if kind == domain.FaultUnauthenticated || kind == domain.FaultNotFound {
			return contract.Refresh401JSONResponse{UnauthorizedJSONResponse: contract.UnauthorizedJSONResponse(value)}, nil
		}
		return nil, err
	}
	setCookie := s.refreshCookie(grant.RefreshToken, grant.RefreshExpiresAt).String()
	return contract.Refresh200JSONResponse{
		Body: authResponse(grant), Headers: contract.Refresh200ResponseHeaders{SetCookie: &setCookie},
	}, nil
}

func (s *Server) Logout(ctx context.Context, _ contract.LogoutRequestObject) (contract.LogoutResponseObject, error) {
	rawRequest := requestFromContext(ctx)
	if !s.originAllowed(rawRequest) {
		value := contractError(ctx, "origin_not_allowed", "Request origin is not allowed", nil)
		return contract.Logout403JSONResponse{ForbiddenJSONResponse: contract.ForbiddenJSONResponse(value)}, nil
	}
	if cookie, err := rawRequest.Cookie(refreshCookieName); err == nil {
		if err := s.sessions.Close(ctx, cookie.Value); err != nil {
			return nil, err
		}
	}
	clearCookie := s.clearRefreshCookie().String()
	return contract.Logout204Response{Headers: contract.Logout204ResponseHeaders{SetCookie: &clearCookie}}, nil
}

func (s *Server) Me(ctx context.Context, _ contract.MeRequestObject) (contract.MeResponseObject, error) {
	return contract.Me200JSONResponse(userResponse(principalFromContext(ctx))), nil
}

func authResponse(grant service.Grant) contract.AuthResponse {
	expiresIn := int(time.Until(grant.AccessExpiresAt).Seconds())
	if expiresIn < 0 {
		expiresIn = 0
	}
	return contract.AuthResponse{
		AccessToken: grant.AccessToken, ExpiresIn: expiresIn, TokenType: contract.Bearer,
		User: userResponse(grant.Principal),
	}
}

func userResponse(principal service.Principal) contract.User {
	return contract.User{
		Id: principal.HRID, Email: openapi_types.Email(principal.Email),
		CreatedAt: principal.CreatedAt, UpdatedAt: principal.UpdatedAt,
	}
}

func (s *Server) refreshCookie(token string, expiry time.Time) *http.Cookie {
	return &http.Cookie{
		Name: refreshCookieName, Value: token, Path: "/api/v1/auth",
		Expires: expiry, MaxAge: max(0, int(time.Until(expiry).Seconds())),
		HttpOnly: true, Secure: s.secureCookies, SameSite: http.SameSiteLaxMode,
	}
}

func (s *Server) clearRefreshCookie() *http.Cookie {
	return &http.Cookie{
		Name: refreshCookieName, Value: "", Path: "/api/v1/auth", Expires: time.Unix(0, 0), MaxAge: -1,
		HttpOnly: true, Secure: s.secureCookies, SameSite: http.SameSiteLaxMode,
	}
}
