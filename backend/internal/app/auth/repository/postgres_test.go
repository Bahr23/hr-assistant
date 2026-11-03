package repository

import (
	"context"
	"os"
	"testing"
	"time"

	"hr-assistant/backend/internal/app/auth/service"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func integrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	pool, err := pgxpool.New(context.Background(), databaseURL)
	require.NoError(t, err)
	require.NoError(t, pool.Ping(context.Background()))
	t.Cleanup(pool.Close)
	return pool
}

func TestLogoutImmediatelyInvalidatesAccessAuthentication(t *testing.T) {
	pool := integrationPool(t)
	sessions, err := service.New(NewSessionStore(pool), service.Config{
		Secret: "integration-secret-with-at-least-thirty-two-characters", AccessTTL: time.Minute, RefreshTTL: time.Hour,
	})
	require.NoError(t, err)
	grant, err := sessions.Open(context.Background(), service.RegisterHR{
		Email: "session-" + uuid.NewString() + "@example.com", Password: "strong-password",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", grant.Principal.HRID)
	})

	_, err = sessions.Authenticate(context.Background(), grant.AccessToken)
	require.NoError(t, err)
	require.NoError(t, sessions.Close(context.Background(), grant.RefreshToken))
	_, err = sessions.Authenticate(context.Background(), grant.AccessToken)
	require.Error(t, err)
}
