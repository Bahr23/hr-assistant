package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"hr-assistant/backend/internal/core/messaging"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"
)

type Runtime struct {
	rabbitURL  string
	shardCount int
	pool       *pgxpool.Pool
	processor  *Processor
	logger     *slog.Logger
}

const outboxPublisherAdvisoryLock int64 = 726001

func NewRuntime(rabbitURL string, shardCount int, pool *pgxpool.Pool, logger *slog.Logger) *Runtime {
	return &Runtime{
		rabbitURL: rabbitURL, shardCount: shardCount, pool: pool,
		processor: NewProcessor(pool, shardCount), logger: logger,
	}
}

func (r *Runtime) Run(ctx context.Context) error {
	connection, err := amqp.Dial(r.rabbitURL)
	if err != nil {
		return fmt.Errorf("connect RabbitMQ: %w", err)
	}
	defer connection.Close()

	if err := r.declareTopology(connection); err != nil {
		return err
	}

	errChannel := make(chan error, r.shardCount+3)
	for shard := range r.shardCount {
		go func() {
			if err := r.consumeShard(ctx, connection, shard); err != nil && ctx.Err() == nil {
				errChannel <- err
			}
		}()
	}
	go func() {
		if err := r.publishOutbox(ctx, connection); err != nil && ctx.Err() == nil {
			errChannel <- err
		}
	}()
	go func() {
		if err := r.expireSessions(ctx); err != nil && ctx.Err() == nil {
			errChannel <- err
		}
	}()
	go func() {
		if err := r.consumeAIResults(ctx, connection); err != nil && ctx.Err() == nil {
			errChannel <- err
		}
	}()

	connectionClosed := connection.NotifyClose(make(chan *amqp.Error, 1))
	r.logger.Info("RabbitMQ interview runtime started", "shards", r.shardCount)
	select {
	case <-ctx.Done():
		return nil
	case err := <-errChannel:
		return err
	case rabbitError := <-connectionClosed:
		if rabbitError == nil || ctx.Err() != nil {
			return nil
		}
		return fmt.Errorf("RabbitMQ connection closed: %w", rabbitError)
	}
}

func (r *Runtime) expireSessions(ctx context.Context) error {
	for {
		expired, err := r.processor.ExpireNext(ctx)
		if err != nil {
			return fmt.Errorf("expire interview session: %w", err)
		}
		if expired {
			continue
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(time.Second):
		}
	}
}

func (r *Runtime) declareTopology(connection *amqp.Connection) error {
	channel, err := connection.Channel()
	if err != nil {
		return fmt.Errorf("open topology channel: %w", err)
	}
	defer channel.Close()
	if err := messaging.DeclareExchanges(channel); err != nil {
		return err
	}
	for shard := range r.shardCount {
		if _, err := messaging.DeclareCandidateQueue(channel, shard); err != nil {
			return err
		}
		if _, err := messaging.DeclareTelegramQueue(channel, shard); err != nil {
			return err
		}
	}
	if _, err := messaging.DeclareAIResultQueue(channel); err != nil {
		return err
	}
	return nil
}

func (r *Runtime) consumeAIResults(ctx context.Context, connection *amqp.Connection) error {
	channel, err := connection.Channel()
	if err != nil {
		return fmt.Errorf("open AI result consumer channel: %w", err)
	}
	defer channel.Close()
	if err := channel.Qos(1, 0, false); err != nil {
		return fmt.Errorf("set AI result QoS: %w", err)
	}
	queue, err := messaging.DeclareAIResultQueue(channel)
	if err != nil {
		return err
	}
	deliveries, err := channel.ConsumeWithContext(
		ctx, queue.Name, "backend-ai-results", false, false, false, false, nil,
	)
	if err != nil {
		return fmt.Errorf("consume AI results: %w", err)
	}
	for delivery := range deliveries {
		var envelope messaging.Envelope
		if err := json.Unmarshal(delivery.Body, &envelope); err != nil {
			r.logger.Error("AI result moved to DLQ", "error", err)
			if rejectErr := delivery.Reject(false); rejectErr != nil {
				return rejectErr
			}
			continue
		}
		permanent, processErr := r.processor.ProcessAIResult(ctx, envelope)
		if processErr == nil {
			if err := delivery.Ack(false); err != nil {
				return fmt.Errorf("ACK AI result: %w", err)
			}
			continue
		}
		if permanent {
			r.logger.Error("AI result moved to DLQ",
				"message_id", envelope.MessageID, "type", envelope.Type, "error", processErr)
			if err := delivery.Reject(false); err != nil {
				return fmt.Errorf("reject AI result: %w", err)
			}
			continue
		}
		r.logger.Error("AI result processing failed; requeueing",
			"message_id", envelope.MessageID, "type", envelope.Type, "error", processErr)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(time.Second):
		}
		if err := delivery.Nack(false, true); err != nil {
			return fmt.Errorf("NACK AI result: %w", err)
		}
	}
	if ctx.Err() != nil {
		return nil
	}
	return fmt.Errorf("AI result delivery channel closed")
}

func (r *Runtime) consumeShard(ctx context.Context, connection *amqp.Connection, shard int) error {
	channel, err := connection.Channel()
	if err != nil {
		return fmt.Errorf("open candidate consumer channel for shard %d: %w", shard, err)
	}
	defer channel.Close()
	if err := channel.Qos(1, 0, false); err != nil {
		return fmt.Errorf("set candidate QoS for shard %d: %w", shard, err)
	}
	queue, err := messaging.DeclareCandidateQueue(channel, shard)
	if err != nil {
		return err
	}
	deliveries, err := channel.ConsumeWithContext(
		ctx, queue.Name, fmt.Sprintf("backend-candidate-%d", shard),
		false, false, false, false, nil,
	)
	if err != nil {
		return fmt.Errorf("consume candidate shard %d: %w", shard, err)
	}

	for delivery := range deliveries {
		var envelope messaging.Envelope
		if err := json.Unmarshal(delivery.Body, &envelope); err != nil {
			r.logger.Error("candidate message moved to DLQ", "shard", shard, "error", err)
			if rejectErr := delivery.Reject(false); rejectErr != nil {
				return rejectErr
			}
			continue
		}
		permanent, processErr := r.processor.Process(ctx, envelope, shard)
		if processErr == nil {
			if err := delivery.Ack(false); err != nil {
				return fmt.Errorf("ACK candidate message: %w", err)
			}
			continue
		}
		if permanent {
			r.logger.Error("candidate message moved to DLQ",
				"message_id", envelope.MessageID, "type", envelope.Type,
				"shard", shard, "error", processErr)
			if err := delivery.Reject(false); err != nil {
				return fmt.Errorf("reject candidate message: %w", err)
			}
			continue
		}

		r.logger.Error("candidate message processing failed; requeueing",
			"message_id", envelope.MessageID, "type", envelope.Type,
			"shard", shard, "error", processErr)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(time.Second):
		}
		if err := delivery.Nack(false, true); err != nil {
			return fmt.Errorf("NACK candidate message: %w", err)
		}
	}
	if ctx.Err() != nil {
		return nil
	}
	return fmt.Errorf("candidate delivery channel for shard %d closed", shard)
}

func (r *Runtime) publishOutbox(ctx context.Context, connection *amqp.Connection) error {
	leadership, err := r.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire outbox leadership connection: %w", err)
	}
	defer leadership.Release()
	for {
		var acquired bool
		err := leadership.QueryRow(ctx,
			"SELECT pg_try_advisory_lock($1)", outboxPublisherAdvisoryLock,
		).Scan(&acquired)
		if err != nil {
			return fmt.Errorf("acquire outbox publisher advisory lock: %w", err)
		}
		if acquired {
			defer func() {
				unlockContext, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				_, _ = leadership.Exec(unlockContext,
					"SELECT pg_advisory_unlock($1)", outboxPublisherAdvisoryLock)
			}()
			break
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(time.Second):
		}
	}

	channel, err := connection.Channel()
	if err != nil {
		return fmt.Errorf("open outbox channel: %w", err)
	}
	defer channel.Close()
	if err := channel.Confirm(false); err != nil {
		return fmt.Errorf("enable publisher confirms: %w", err)
	}
	confirms := channel.NotifyPublish(make(chan amqp.Confirmation, 1))

	for {
		published, err := r.publishNext(ctx, channel, confirms)
		if err != nil {
			return err
		}
		if published {
			continue
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func (r *Runtime) publishNext(
	ctx context.Context,
	channel *amqp.Channel,
	confirms <-chan amqp.Confirmation,
) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	var id uuid.UUID
	var exchange, routingKey string
	var body []byte
	err = tx.QueryRow(ctx, `
SELECT id, exchange_name, routing_key, body
FROM outbox_events
WHERE published_at IS NULL
ORDER BY created_at, id
FOR UPDATE SKIP LOCKED
LIMIT 1`).Scan(&id, &exchange, &routingKey, &body)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	err = channel.PublishWithContext(ctx, exchange, routingKey, false, false, amqp.Publishing{
		DeliveryMode: amqp.Persistent,
		ContentType:  "application/json",
		MessageId:    id.String(),
		Timestamp:    time.Now().UTC(),
		Body:         body,
	})
	if err != nil {
		return false, fmt.Errorf("publish outbox event %s: %w", id, err)
	}
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case confirmation, ok := <-confirms:
		if !ok {
			return false, fmt.Errorf("publisher confirm channel closed")
		}
		if !confirmation.Ack {
			return false, fmt.Errorf("RabbitMQ rejected outbox event %s", id)
		}
	case <-time.After(10 * time.Second):
		return false, fmt.Errorf("publisher confirm timed out for outbox event %s", id)
	}

	if _, err := tx.Exec(ctx, "UPDATE outbox_events SET published_at = now() WHERE id = $1", id); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}
