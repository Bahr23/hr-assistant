package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"hr-assistant/openrouter-worker/internal/app/actions"
	"hr-assistant/openrouter-worker/internal/app/answerevaluation"
	"hr-assistant/openrouter-worker/internal/app/candidatefeedback"
	"hr-assistant/openrouter-worker/internal/app/candidatereport"
	"hr-assistant/openrouter-worker/internal/app/transcription"
	"hr-assistant/openrouter-worker/internal/core/config"
	"hr-assistant/openrouter-worker/internal/core/messaging"
	feedback "hr-assistant/shared/candidatefeedback"
	report "hr-assistant/shared/candidatereport"
	"hr-assistant/shared/evaluation"

	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"
)

const outboxPublisherAdvisoryLock int64 = 87342001

type Runtime struct {
	settings   config.Serve
	pool       *pgxpool.Pool
	repository *actions.Repository
	executor   *actions.Executor
	logger     *slog.Logger
}

func NewRuntime(settings config.Serve, pool *pgxpool.Pool, logger *slog.Logger) (*Runtime, error) {
	if err := transcription.PrepareTempRoot(settings.TempDir); err != nil {
		return nil, fmt.Errorf("prepare temporary directory: %w", err)
	}
	httpClient := &http.Client{
		Timeout: settings.HTTPTimeout,
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: min(settings.HTTPTimeout, 2*time.Minute),
			IdleConnTimeout:       90 * time.Second,
			MaxIdleConns:          16,
			MaxIdleConnsPerHost:   settings.Concurrency,
		},
	}
	telegram := transcription.NewTelegramClient(
		httpClient, settings.TelegramAPIBaseURL, settings.TelegramBotToken,
	)
	openRouter := transcription.NewOpenRouterClient(
		httpClient, settings.OpenRouterBaseURL, settings.OpenRouterAPIKey,
	)
	handler := transcription.NewHandler(telegram, openRouter, settings.TempDir)
	repository := actions.NewRepository(pool)
	executor := actions.NewExecutor(
		repository, actions.NewRegistry(handler, answerevaluation.NewHandler(httpClient, settings.OpenRouterBaseURL, settings.OpenRouterAPIKey), candidatereport.NewHandler(httpClient, settings.OpenRouterBaseURL, settings.OpenRouterAPIKey), candidatefeedback.NewHandler(httpClient, settings.OpenRouterBaseURL, settings.OpenRouterAPIKey)), logger,
		settings.Concurrency, settings.LeaseDuration, settings.MaxBackoff,
	)
	return &Runtime{
		settings: settings, pool: pool, repository: repository,
		executor: executor, logger: logger,
	}, nil
}

func (r *Runtime) Run(ctx context.Context) error {
	connection, err := amqp.Dial(r.settings.RabbitMQURL)
	if err != nil {
		return fmt.Errorf("connect RabbitMQ: %w", err)
	}
	defer connection.Close()
	channel, err := connection.Channel()
	if err != nil {
		return fmt.Errorf("open topology channel: %w", err)
	}
	_, err = messaging.DeclareTopology(channel)
	_ = channel.Close()
	if err != nil {
		return fmt.Errorf("declare RabbitMQ topology: %w", err)
	}

	r.executor.Run(ctx)
	errorsChannel := make(chan error, 3)
	go func() { errorsChannel <- r.consume(ctx, connection) }()
	go func() { errorsChannel <- r.publishOutbox(ctx, connection) }()
	go func() { errorsChannel <- r.cleanup(ctx) }()
	closed := connection.NotifyClose(make(chan *amqp.Error, 1))
	r.logger.Info("OpenRouter worker started", "concurrency", r.settings.Concurrency)
	select {
	case <-ctx.Done():
		return nil
	case err := <-errorsChannel:
		if ctx.Err() != nil {
			return nil
		}
		return err
	case rabbitError := <-closed:
		if rabbitError == nil || ctx.Err() != nil {
			return nil
		}
		return fmt.Errorf("RabbitMQ connection closed: %w", rabbitError)
	}
}

func (r *Runtime) consume(ctx context.Context, connection *amqp.Connection) error {
	channel, err := connection.Channel()
	if err != nil {
		return fmt.Errorf("open completion consumer channel: %w", err)
	}
	defer channel.Close()
	if err := channel.Qos(1, 0, false); err != nil {
		return fmt.Errorf("set completion consumer QoS: %w", err)
	}
	queue, err := messaging.DeclareTopology(channel)
	if err != nil {
		return err
	}
	deliveries, err := channel.ConsumeWithContext(
		ctx, queue.Name, "openrouter-interview-completed", false, false, false, false, nil,
	)
	if err != nil {
		return fmt.Errorf("consume interview completions: %w", err)
	}
	for delivery := range deliveries {
		var envelope messaging.Envelope
		if err := json.Unmarshal(delivery.Body, &envelope); err != nil {
			r.logger.Error("completion event moved to DLQ", "error", "invalid JSON")
			if err := delivery.Reject(false); err != nil {
				return err
			}
			continue
		}
		var created int
		var ingestErr error
		if envelope.Type == feedback.Requested {
			created, ingestErr = r.repository.IngestFeedback(ctx, envelope, r.settings.ReportModel)
		} else if envelope.Type == report.Requested {
			created, ingestErr = r.repository.IngestReport(ctx, envelope, r.settings.ReportModel)
		} else if envelope.Type == evaluation.Requested {
			created, ingestErr = r.repository.IngestEvaluation(ctx, envelope, r.settings.EvaluationModel)
		} else {
			created, ingestErr = r.repository.IngestCompletion(ctx, envelope, r.settings.OpenRouterModel)
		}
		if ingestErr == nil {
			r.logger.Info("completion event stored", "message_id", envelope.MessageID, "jobs_created", created)
			if err := delivery.Ack(false); err != nil {
				return fmt.Errorf("ACK completion event: %w", err)
			}
			continue
		}
		if errors.Is(ingestErr, actions.ErrInvalidEvent) {
			r.logger.Error("completion event moved to DLQ", "message_id", envelope.MessageID, "error", ingestErr)
			if err := delivery.Reject(false); err != nil {
				return err
			}
			continue
		}
		r.logger.Error("completion event processing failed; requeueing", "message_id", envelope.MessageID, "error", ingestErr)
		if !wait(ctx, time.Second) {
			return nil
		}
		if err := delivery.Nack(false, true); err != nil {
			return fmt.Errorf("NACK completion event: %w", err)
		}
	}
	if ctx.Err() != nil {
		return nil
	}
	return fmt.Errorf("completion delivery channel closed")
}

func (r *Runtime) publishOutbox(ctx context.Context, connection *amqp.Connection) error {
	leadership, err := r.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire outbox leadership connection: %w", err)
	}
	defer leadership.Release()
	for {
		var acquired bool
		if err := leadership.QueryRow(ctx,
			"SELECT pg_try_advisory_lock($1)", outboxPublisherAdvisoryLock,
		).Scan(&acquired); err != nil {
			return err
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
		if !wait(ctx, time.Second) {
			return nil
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
		event, found, err := r.repository.NextOutbox(ctx)
		if err != nil {
			return err
		}
		if !found {
			if !wait(ctx, 200*time.Millisecond) {
				return nil
			}
			continue
		}
		if err := channel.PublishWithContext(ctx, event.Exchange, event.RoutingKey, false, false, amqp.Publishing{
			DeliveryMode: amqp.Persistent, ContentType: "application/json",
			MessageId: event.ID.String(), Timestamp: time.Now().UTC(), Body: event.Body,
		}); err != nil {
			return fmt.Errorf("publish outbox event %s: %w", event.ID, err)
		}
		select {
		case confirmation, ok := <-confirms:
			if !ok || !confirmation.Ack {
				return fmt.Errorf("RabbitMQ did not confirm outbox event %s", event.ID)
			}
		case <-ctx.Done():
			return nil
		case <-time.After(10 * time.Second):
			return fmt.Errorf("publisher confirm timed out for outbox event %s", event.ID)
		}
		if err := r.repository.MarkPublished(ctx, event.ID); err != nil {
			return err
		}
	}
}

func (r *Runtime) cleanup(ctx context.Context) error {
	ticker := time.NewTicker(r.settings.CleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			deleted, err := r.repository.Cleanup(
				ctx, r.settings.InboxRetention, r.settings.OutboxRetention, r.settings.JobRetention,
			)
			if err != nil {
				r.logger.Error("cleanup failed", "error", err)
				continue
			}
			if deleted > 0 {
				r.logger.Info("old worker records removed", "rows", deleted)
			}
		}
	}
}

func wait(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
