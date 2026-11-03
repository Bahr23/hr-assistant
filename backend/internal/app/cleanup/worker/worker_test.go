package worker

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestNewValidatesConfiguration(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	_, err := New(nil, Config{}, logger)

	require.EqualError(t, err, "database pool is required")
}

func TestRunOnceDeletesOnlyExpiredTerminalRows(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	adminPool, err := pgxpool.New(ctx, databaseURL)
	require.NoError(t, err)
	t.Cleanup(adminPool.Close)

	schema := "cleanup_test_" + uuid.NewString()
	schema = replaceHyphens(schema)
	_, err = adminPool.Exec(ctx, `CREATE SCHEMA `+schema)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = adminPool.Exec(context.Background(), `DROP SCHEMA `+schema+` CASCADE`)
	})
	_, err = adminPool.Exec(ctx, `
CREATE TABLE `+schema+`.inbox_messages (
    message_id UUID PRIMARY KEY,
    message_type TEXT NOT NULL,
    received_at TIMESTAMPTZ NOT NULL
)`)
	require.NoError(t, err)
	_, err = adminPool.Exec(ctx, `CREATE TABLE `+schema+`.outbox_events (
    id UUID PRIMARY KEY,
    published_at TIMESTAMPTZ
)`)
	require.NoError(t, err)
	_, err = adminPool.Exec(ctx, `CREATE TABLE `+schema+`.telegram_deliveries (
    id UUID PRIMARY KEY,
    status TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
)`)
	require.NoError(t, err)

	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	require.NoError(t, err)
	poolConfig.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	old := now.Add(-100 * 24 * time.Hour)
	recent := now.Add(-time.Hour)
	oldInbox, recentInbox := uuid.New(), uuid.New()
	oldPublished, recentPublished, unpublished := uuid.New(), uuid.New(), uuid.New()
	oldSent, oldFailed, recentSent, oldPending := uuid.New(), uuid.New(), uuid.New(), uuid.New()

	_, err = pool.Exec(ctx, `
INSERT INTO inbox_messages (message_id, message_type, received_at) VALUES
    ($1, 'old', $3), ($2, 'recent', $4)`, oldInbox, recentInbox, old, recent)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
INSERT INTO outbox_events (id, published_at) VALUES
    ($1, $4), ($2, $5), ($3, NULL)`, oldPublished, recentPublished, unpublished, old, recent)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
INSERT INTO telegram_deliveries (id, status, updated_at) VALUES
	    ($1, 'sent', $5), ($2, 'failed', $5), ($3, 'sent', $6), ($4, 'pending', $5)`,
		oldSent, oldFailed, recentSent, oldPending, old, recent)
	require.NoError(t, err)

	cleaner, err := New(pool, Config{
		Interval:                  time.Hour,
		BatchSize:                 1,
		InboxRetention:            30 * 24 * time.Hour,
		OutboxRetention:           7 * 24 * time.Hour,
		TelegramDeliveryRetention: 90 * 24 * time.Hour,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	cleaner.now = func() time.Time { return now }

	result, err := cleaner.RunOnce(ctx)

	require.NoError(t, err)
	require.Equal(t, Result{InboxMessages: 1, OutboxEvents: 1, TelegramDeliveries: 2}, result)
	requireIDs(t, ctx, pool, "inbox_messages", "message_id", recentInbox)
	requireIDs(t, ctx, pool, "outbox_events", "id", recentPublished, unpublished)
	requireIDs(t, ctx, pool, "telegram_deliveries", "id", recentSent, oldPending)
}

func requireIDs(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	table, column string,
	want ...uuid.UUID,
) {
	t.Helper()
	rows, err := pool.Query(ctx, `SELECT `+column+` FROM `+table+` ORDER BY `+column)
	require.NoError(t, err)
	defer rows.Close()
	got := make([]uuid.UUID, 0, len(want))
	for rows.Next() {
		var id uuid.UUID
		require.NoError(t, rows.Scan(&id))
		got = append(got, id)
	}
	require.NoError(t, rows.Err())
	require.ElementsMatch(t, want, got)
}

func replaceHyphens(value string) string {
	result := []byte(value)
	for index := range result {
		if result[index] == '-' {
			result[index] = '_'
		}
	}
	return string(result)
}
