package repository

import (
	"context"
	"crypto/subtle"
	"errors"
	"time"

	"hr-assistant/backend/internal/app/auth/service"
	"hr-assistant/backend/internal/core/database/dbgen"
	"hr-assistant/backend/internal/core/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SessionStore struct {
	pool    *pgxpool.Pool
	queries *dbgen.Queries
}

func NewSessionStore(pool *pgxpool.Pool) *SessionStore {
	return &SessionStore{pool: pool, queries: dbgen.New(pool)}
}

func (s *SessionStore) Register(ctx context.Context, hr domain.User, session domain.Session) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	queries := s.queries.WithTx(tx)
	if _, err := queries.CreateUser(ctx, dbgen.CreateUserParams{
		ID: hr.ID, Email: hr.Email, PasswordHash: hr.PasswordHash,
	}); err != nil {
		return sessionError(err)
	}
	if _, err := queries.CreateSession(ctx, dbgen.CreateSessionParams{
		ID: session.ID, UserID: session.UserID, RefreshTokenHash: session.RefreshTokenHash,
		ExpiresAt: timestamp(session.ExpiresAt),
	}); err != nil {
		return sessionError(err)
	}
	return tx.Commit(ctx)
}

func (s *SessionStore) FindHRByEmail(ctx context.Context, email string) (domain.User, error) {
	value, err := s.queries.GetUserByEmail(ctx, email)
	if err != nil {
		return domain.User{}, sessionError(err)
	}
	return userFromDB(value), nil
}

func (s *SessionStore) Start(ctx context.Context, session domain.Session) error {
	_, err := s.queries.CreateSession(ctx, dbgen.CreateSessionParams{
		ID: session.ID, UserID: session.UserID, RefreshTokenHash: session.RefreshTokenHash,
		ExpiresAt: timestamp(session.ExpiresAt),
	})
	return sessionError(err)
}

func (s *SessionStore) RotateOrRevoke(ctx context.Context, attempt service.RotationAttempt) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var userID uuid.UUID
	var currentHash string
	var expiresAt time.Time
	var revokedAt *time.Time
	err = tx.QueryRow(ctx, `
SELECT user_id, refresh_token_hash, expires_at, revoked_at
FROM sessions
WHERE id = $1
FOR UPDATE`, attempt.SessionID).Scan(&userID, &currentHash, &expiresAt, &revokedAt)
	if err != nil {
		return sessionError(err)
	}
	if userID != attempt.HRID || revokedAt != nil || !expiresAt.After(attempt.Now) {
		return unauthenticatedSession()
	}
	if subtle.ConstantTimeCompare([]byte(currentHash), []byte(attempt.PresentedHash)) != 1 {
		if _, err := tx.Exec(ctx, "UPDATE sessions SET revoked_at = $2, updated_at = $2 WHERE id = $1", attempt.SessionID, attempt.Now); err != nil {
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		return unauthenticatedSession()
	}
	command, err := tx.Exec(ctx, `
UPDATE sessions
SET refresh_token_hash = $2, expires_at = $3, updated_at = $4
WHERE id = $1 AND revoked_at IS NULL`,
		attempt.SessionID, attempt.NextHash, attempt.NextExpiresAt, attempt.Now)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return unauthenticatedSession()
	}
	return tx.Commit(ctx)
}

func (s *SessionStore) Authenticate(ctx context.Context, sessionID, hrID uuid.UUID, now time.Time) (domain.User, error) {
	row := s.pool.QueryRow(ctx, `
SELECT u.id, u.email, u.password_hash, u.created_at, u.updated_at
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.id = $1
  AND s.user_id = $2
  AND s.revoked_at IS NULL
  AND s.expires_at > $3`, sessionID, hrID, now)
	var hr domain.User
	if err := row.Scan(&hr.ID, &hr.Email, &hr.PasswordHash, &hr.CreatedAt, &hr.UpdatedAt); err != nil {
		return domain.User{}, sessionError(err)
	}
	return hr, nil
}

func (s *SessionStore) Revoke(ctx context.Context, sessionID uuid.UUID) error {
	return s.queries.RevokeSession(ctx, sessionID)
}

func sessionError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.NewFault(domain.FaultNotFound, "session_not_found", "session not found")
	}
	var pgError *pgconn.PgError
	if errors.As(err, &pgError) && pgError.Code == "23505" {
		if pgError.ConstraintName == "users_email_key" {
			return domain.NewFault(domain.FaultConflict, "email_already_registered", "email is already registered")
		}
		return domain.NewFault(domain.FaultConflict, "conflict", "resource conflict")
	}
	return err
}

func unauthenticatedSession() error {
	return domain.NewFault(domain.FaultUnauthenticated, "unauthenticated", "invalid authentication credentials")
}
