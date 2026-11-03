package worker

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const cleanupAdvisoryLock int64 = 726002

type Config struct {
	Interval                  time.Duration
	BatchSize                 int
	InboxRetention            time.Duration
	OutboxRetention           time.Duration
	TelegramDeliveryRetention time.Duration
}

type Result struct {
	InboxMessages      int64
	OutboxEvents       int64
	TelegramDeliveries int64
}

func (r Result) empty() bool {
	return r.InboxMessages == 0 && r.OutboxEvents == 0 && r.TelegramDeliveries == 0
}

type Worker struct {
	pool   *pgxpool.Pool
	config Config
	logger *slog.Logger
	now    func() time.Time
}

func New(pool *pgxpool.Pool, config Config, logger *slog.Logger) (*Worker, error) {
	if pool == nil {
		return nil, fmt.Errorf("database pool is required")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}
	if config.Interval <= 0 {
		return nil, fmt.Errorf("cleanup interval must be positive")
	}
	if config.BatchSize <= 0 {
		return nil, fmt.Errorf("cleanup batch size must be positive")
	}
	if config.InboxRetention <= 0 || config.OutboxRetention <= 0 || config.TelegramDeliveryRetention <= 0 {
		return nil, fmt.Errorf("cleanup retention periods must be positive")
	}
	return &Worker{pool: pool, config: config, logger: logger, now: time.Now}, nil
}

func (w *Worker) Run(ctx context.Context) error {
	connection, err := w.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire cleanup leadership connection: %w", err)
	}
	defer connection.Release()

	for {
		var acquired bool
		if err := connection.QueryRow(ctx,
			"SELECT pg_try_advisory_lock($1)", cleanupAdvisoryLock,
		).Scan(&acquired); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("acquire cleanup advisory lock: %w", err)
		}
		if acquired {
			defer func() {
				unlockContext, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				_, _ = connection.Exec(unlockContext,
					"SELECT pg_advisory_unlock($1)", cleanupAdvisoryLock)
			}()
			break
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(time.Second):
		}
	}

	w.logger.Info("cleanup worker started",
		"interval", w.config.Interval.String(),
		"batch_size", w.config.BatchSize,
		"inbox_retention", w.config.InboxRetention.String(),
		"outbox_retention", w.config.OutboxRetention.String(),
		"telegram_delivery_retention", w.config.TelegramDeliveryRetention.String(),
	)
	if _, err := w.RunOnce(ctx); err != nil {
		return err
	}

	ticker := time.NewTicker(w.config.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if _, err := w.RunOnce(ctx); err != nil {
				return err
			}
		}
	}
}

func (w *Worker) RunOnce(ctx context.Context) (Result, error) {
	now := w.now().UTC()
	cutoffs := struct {
		inbox, outbox, deliveries time.Time
	}{
		inbox:      now.Add(-w.config.InboxRetention),
		outbox:     now.Add(-w.config.OutboxRetention),
		deliveries: now.Add(-w.config.TelegramDeliveryRetention),
	}

	var total Result
	for {
		batch, err := w.deleteBatch(ctx, cutoffs.inbox, cutoffs.outbox, cutoffs.deliveries)
		if err != nil {
			return total, err
		}
		total.InboxMessages += batch.InboxMessages
		total.OutboxEvents += batch.OutboxEvents
		total.TelegramDeliveries += batch.TelegramDeliveries
		if batch.InboxMessages < int64(w.config.BatchSize) &&
			batch.OutboxEvents < int64(w.config.BatchSize) &&
			batch.TelegramDeliveries < int64(w.config.BatchSize) {
			break
		}
	}
	if !total.empty() {
		w.logger.Info("cleanup sweep completed",
			"inbox_messages_deleted", total.InboxMessages,
			"outbox_events_deleted", total.OutboxEvents,
			"telegram_deliveries_deleted", total.TelegramDeliveries,
		)
	}
	return total, nil
}

func (w *Worker) deleteBatch(
	ctx context.Context,
	inboxCutoff, outboxCutoff, deliveriesCutoff time.Time,
) (Result, error) {
	var result Result
	command, err := w.pool.Exec(ctx, `
WITH doomed AS (
    SELECT message_id
    FROM inbox_messages
    WHERE received_at < $1
    ORDER BY received_at, message_id
    LIMIT $2
    FOR UPDATE SKIP LOCKED
)
DELETE FROM inbox_messages AS target
USING doomed
WHERE target.message_id = doomed.message_id`, inboxCutoff, w.config.BatchSize)
	if err != nil {
		return result, fmt.Errorf("delete inbox messages: %w", err)
	}
	result.InboxMessages = command.RowsAffected()

	command, err = w.pool.Exec(ctx, `
WITH doomed AS (
    SELECT id
    FROM outbox_events
    WHERE published_at IS NOT NULL AND published_at < $1
    ORDER BY published_at, id
    LIMIT $2
    FOR UPDATE SKIP LOCKED
)
DELETE FROM outbox_events AS target
USING doomed
WHERE target.id = doomed.id`, outboxCutoff, w.config.BatchSize)
	if err != nil {
		return result, fmt.Errorf("delete outbox events: %w", err)
	}
	result.OutboxEvents = command.RowsAffected()

	command, err = w.pool.Exec(ctx, `
WITH doomed AS (
    SELECT id
    FROM telegram_deliveries
    WHERE status IN ('sent', 'failed') AND updated_at < $1
    ORDER BY updated_at, id
    LIMIT $2
    FOR UPDATE SKIP LOCKED
)
DELETE FROM telegram_deliveries AS target
USING doomed
WHERE target.id = doomed.id`, deliveriesCutoff, w.config.BatchSize)
	if err != nil {
		return result, fmt.Errorf("delete Telegram deliveries: %w", err)
	}
	result.TelegramDeliveries = command.RowsAffected()
	return result, nil
}
