package service

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"time"

	"hr-assistant/backend/internal/core/domain"

	"github.com/google/uuid"
)

type store interface {
	Register(context.Context, domain.User, domain.Session) error
	FindHRByEmail(context.Context, string) (domain.User, error)
	Start(context.Context, domain.Session) error
	RotateOrRevoke(context.Context, RotationAttempt) error
	Authenticate(context.Context, uuid.UUID, uuid.UUID, time.Time) (domain.User, error)
	Revoke(context.Context, uuid.UUID) error
}

type Config struct {
	Secret     string
	AccessTTL  time.Duration
	RefreshTTL time.Duration
}

type Service struct {
	store     store
	passwords passwordCodec
	tokens    tokenCodec
	dummyHash string
	now       func() time.Time
}

func New(store store, config Config) (*Service, error) {
	passwords := newPasswordCodec()
	dummyHash, err := passwords.hash("dummy-password-not-used")
	if err != nil {
		return nil, err
	}
	return &Service{
		store: store, passwords: passwords,
		tokens:    newTokenCodec(config.Secret, config.AccessTTL, config.RefreshTTL),
		dummyHash: dummyHash, now: func() time.Time { return time.Now().UTC() },
	}, nil
}

type OpenIntent interface{ hrSessionOpenIntent() }

type RegisterHR struct {
	Email    string
	Password string
}

type PasswordLogin struct {
	Email    string
	Password string
}

type Refresh struct{ Token string }

func (RegisterHR) hrSessionOpenIntent()    {}
func (PasswordLogin) hrSessionOpenIntent() {}
func (Refresh) hrSessionOpenIntent()       {}

type Principal struct {
	HRID      uuid.UUID
	SessionID uuid.UUID
	Email     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Grant struct {
	Principal        Principal
	AccessToken      string
	RefreshToken     string
	AccessExpiresAt  time.Time
	RefreshExpiresAt time.Time
}

type RotationAttempt struct {
	SessionID     uuid.UUID
	HRID          uuid.UUID
	PresentedHash string
	NextHash      string
	NextExpiresAt time.Time
	Now           time.Time
}

func (m *Service) Open(ctx context.Context, intent OpenIntent) (Grant, error) {
	switch value := intent.(type) {
	case RegisterHR:
		return m.register(ctx, value)
	case PasswordLogin:
		return m.login(ctx, value)
	case Refresh:
		return m.refresh(ctx, value.Token)
	default:
		return Grant{}, domain.NewFault(domain.FaultValidation, "unsupported_session_intent", "unsupported HR session intent")
	}
}

func (m *Service) Authenticate(ctx context.Context, accessToken string) (Principal, error) {
	claims, err := m.tokens.parseAccess(accessToken)
	if err != nil {
		return Principal{}, unauthenticated()
	}
	hr, err := m.store.Authenticate(ctx, claims.SessionID, claims.HRID, m.now())
	if err != nil {
		var fault *domain.Fault
		if errors.As(err, &fault) && (fault.Kind == domain.FaultNotFound || fault.Kind == domain.FaultUnauthenticated) {
			return Principal{}, unauthenticated()
		}
		return Principal{}, err
	}
	return principal(hr, claims.SessionID), nil
}

func (m *Service) Close(ctx context.Context, refreshToken string) error {
	claims, err := m.tokens.parseRefresh(refreshToken)
	if err != nil {
		return nil
	}
	return m.store.Revoke(ctx, claims.SessionID)
}

func (m *Service) register(ctx context.Context, input RegisterHR) (Grant, error) {
	email, err := normalizeEmail(input.Email)
	if err != nil {
		return Grant{}, err
	}
	if len(input.Password) < 10 || len(input.Password) > 128 {
		return Grant{}, domain.ValidationFault("validation_failed", "password", "must contain 10 to 128 characters")
	}
	passwordHash, err := m.passwords.hash(input.Password)
	if err != nil {
		return Grant{}, err
	}
	now := m.now()
	hr := domain.User{
		ID: uuid.New(), Email: email, PasswordHash: passwordHash,
		CreatedAt: now, UpdatedAt: now,
	}
	sessionID := uuid.New()
	pair, err := m.tokens.issuePair(hr.ID, sessionID, now)
	if err != nil {
		return Grant{}, err
	}
	session := domain.Session{
		ID: sessionID, UserID: hr.ID, RefreshTokenHash: m.tokens.hash(pair.RefreshToken),
		ExpiresAt: pair.RefreshExpiresAt,
	}
	if err := m.store.Register(ctx, hr, session); err != nil {
		return Grant{}, err
	}
	return grant(hr, sessionID, pair), nil
}

func (m *Service) login(ctx context.Context, input PasswordLogin) (Grant, error) {
	email, err := normalizeEmail(input.Email)
	if err != nil {
		_ = m.passwords.compare(m.dummyHash, input.Password)
		return Grant{}, unauthenticated()
	}
	hr, err := m.store.FindHRByEmail(ctx, email)
	if err != nil {
		_ = m.passwords.compare(m.dummyHash, input.Password)
		var fault *domain.Fault
		if errors.As(err, &fault) && fault.Kind == domain.FaultNotFound {
			return Grant{}, unauthenticated()
		}
		return Grant{}, err
	}
	if err := m.passwords.compare(hr.PasswordHash, input.Password); err != nil {
		return Grant{}, unauthenticated()
	}
	now := m.now()
	sessionID := uuid.New()
	pair, err := m.tokens.issuePair(hr.ID, sessionID, now)
	if err != nil {
		return Grant{}, err
	}
	if err := m.store.Start(ctx, domain.Session{
		ID: sessionID, UserID: hr.ID, RefreshTokenHash: m.tokens.hash(pair.RefreshToken),
		ExpiresAt: pair.RefreshExpiresAt,
	}); err != nil {
		return Grant{}, err
	}
	return grant(hr, sessionID, pair), nil
}

func (m *Service) refresh(ctx context.Context, raw string) (Grant, error) {
	claims, err := m.tokens.parseRefresh(raw)
	if err != nil {
		return Grant{}, unauthenticated()
	}
	now := m.now()
	pair, err := m.tokens.issuePair(claims.HRID, claims.SessionID, now)
	if err != nil {
		return Grant{}, err
	}
	if err := m.store.RotateOrRevoke(ctx, RotationAttempt{
		SessionID: claims.SessionID, HRID: claims.HRID,
		PresentedHash: m.tokens.hash(raw), NextHash: m.tokens.hash(pair.RefreshToken),
		NextExpiresAt: pair.RefreshExpiresAt, Now: now,
	}); err != nil {
		var fault *domain.Fault
		if errors.As(err, &fault) && (fault.Kind == domain.FaultNotFound || fault.Kind == domain.FaultUnauthenticated) {
			return Grant{}, unauthenticated()
		}
		return Grant{}, err
	}
	hr, err := m.store.Authenticate(ctx, claims.SessionID, claims.HRID, now)
	if err != nil {
		return Grant{}, unauthenticated()
	}
	return grant(hr, claims.SessionID, pair), nil
}

func normalizeEmail(raw string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(raw))
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email || len(email) > 254 {
		return "", domain.ValidationFault("validation_failed", "email", "is invalid")
	}
	return email, nil
}

func grant(hr domain.User, sessionID uuid.UUID, pair tokenPair) Grant {
	return Grant{
		Principal: principal(hr, sessionID), AccessToken: pair.AccessToken,
		RefreshToken: pair.RefreshToken, AccessExpiresAt: pair.AccessExpiresAt,
		RefreshExpiresAt: pair.RefreshExpiresAt,
	}
}

func principal(hr domain.User, sessionID uuid.UUID) Principal {
	return Principal{
		HRID: hr.ID, SessionID: sessionID, Email: hr.Email,
		CreatedAt: hr.CreatedAt, UpdatedAt: hr.UpdatedAt,
	}
}

func unauthenticated() error {
	return domain.NewFault(domain.FaultUnauthenticated, "unauthenticated", "invalid authentication credentials")
}
