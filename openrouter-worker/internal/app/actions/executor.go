package actions

import (
	"context"
	"log/slog"
	"math"
	"math/rand/v2"
	"time"
)

type Executor struct {
	repository *Repository
	registry   Registry
	logger     *slog.Logger
	workers    int
	lease      time.Duration
	maxBackoff time.Duration
}

func NewExecutor(
	repository *Repository,
	registry Registry,
	logger *slog.Logger,
	workers int,
	lease, maxBackoff time.Duration,
) *Executor {
	return &Executor{
		repository: repository, registry: registry, logger: logger,
		workers: workers, lease: lease, maxBackoff: maxBackoff,
	}
}

func (e *Executor) Run(ctx context.Context) {
	for worker := 0; worker < e.workers; worker++ {
		go e.runWorker(ctx, worker)
	}
}

func (e *Executor) runWorker(ctx context.Context, worker int) {
	logger := e.logger.With("worker", worker)
	for ctx.Err() == nil {
		job, found, err := e.repository.Claim(ctx, e.lease)
		if err != nil {
			logger.Error("claim action job", "error", err)
			wait(ctx, time.Second)
			continue
		}
		if !found {
			wait(ctx, 250*time.Millisecond)
			continue
		}
		handler, ok := e.registry.Handler(job.ActionType)
		if !ok {
			failure := Failure{Permanent: true, Code: "unknown_action", Message: "action handler is not registered"}
			if err := e.repository.PermanentFail(ctx, job, failure); err != nil {
				logger.Error("store unknown action failure", "job_id", job.ID, "error", err)
			}
			continue
		}
		result, failure := handler.Execute(ctx, job)
		if failure == nil {
			if err := e.repository.Succeed(ctx, job, result); err != nil {
				logger.Error("store action result", "job_id", job.ID, "answer_id", job.AnswerID, "error", err)
			}
			continue
		}
		if failure.Permanent {
			if err := e.repository.PermanentFail(ctx, job, *failure); err != nil {
				logger.Error("store permanent action failure", "job_id", job.ID, "answer_id", job.AnswerID, "error", err)
			} else {
				logger.Warn("action permanently failed", "job_id", job.ID, "answer_id", job.AnswerID,
					"attempt", job.Attempts, "code", failure.Code, "reason", failure.Message)
			}
			continue
		}
		delay := e.backoff(job.Attempts)
		if failure.RetryAfter > delay {
			delay = failure.RetryAfter
		}
		if delay > e.maxBackoff {
			delay = e.maxBackoff
		}
		if err := e.repository.Retry(ctx, job, *failure, time.Now().UTC().Add(delay)); err != nil {
			logger.Error("schedule action retry", "job_id", job.ID, "answer_id", job.AnswerID, "error", err)
			continue
		}
		logger.Warn("action retry scheduled", "job_id", job.ID, "answer_id", job.AnswerID,
			"attempt", job.Attempts, "code", failure.Code, "reason", failure.Message, "retry_in", delay)
	}
}

func (e *Executor) backoff(attempt int) time.Duration {
	seconds := math.Pow(2, float64(min(attempt, 20)))
	ceiling := min(time.Duration(seconds*float64(time.Second)), e.maxBackoff)
	if ceiling <= time.Second {
		return ceiling
	}
	return time.Duration(rand.Int64N(int64(ceiling-time.Second))) + time.Second
}

func wait(ctx context.Context, duration time.Duration) {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}
