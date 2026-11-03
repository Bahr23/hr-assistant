package messaging

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	ShardCount         = 8
	CandidateExchange  = "candidate.commands"
	TelegramExchange   = "telegram.commands"
	InterviewExchange  = "interview.events"
	DeadLetterExchange = "dead.letters"
)

type Envelope struct {
	MessageID     uuid.UUID       `json:"message_id"`
	Type          string          `json:"type"`
	SchemaVersion int             `json:"schema_version"`
	OccurredAt    time.Time       `json:"occurred_at"`
	CorrelationID uuid.UUID       `json:"correlation_id"`
	PartitionKey  int64           `json:"partition_key"`
	Payload       json.RawMessage `json:"payload"`
}

func NewEnvelope(messageType string, correlationID uuid.UUID, partitionKey int64, payload any) (Envelope, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return Envelope{}, err
	}
	if correlationID == uuid.Nil {
		correlationID = uuid.New()
	}
	return Envelope{
		MessageID: uuid.New(), Type: messageType, SchemaVersion: 1,
		OccurredAt: time.Now().UTC(), CorrelationID: correlationID,
		PartitionKey: partitionKey, Payload: encoded,
	}, nil
}

func Shard(partitionKey int64, shardCount int) int {
	value := partitionKey % int64(shardCount)
	if value < 0 {
		value += int64(shardCount)
	}
	return int(value)
}

func DeclareExchanges(channel *amqp.Channel) error {
	for _, exchange := range []string{CandidateExchange, TelegramExchange, InterviewExchange, DeadLetterExchange} {
		if err := channel.ExchangeDeclare(exchange, "topic", true, false, false, false, nil); err != nil {
			return fmt.Errorf("declare exchange %s: %w", exchange, err)
		}
	}
	return nil
}

func DeclareCandidateQueue(channel *amqp.Channel, shard int) (amqp.Queue, error) {
	return declareShardedQueue(channel, CandidateExchange, "candidate.updates", "candidate", shard)
}

func DeclareTelegramQueue(channel *amqp.Channel, shard int) (amqp.Queue, error) {
	return declareShardedQueue(channel, TelegramExchange, "telegram.delivery", "telegram", shard)
}

func DeclareAIResultQueue(channel *amqp.Channel) (amqp.Queue, error) {
	const queueName = "backend.ai-results.q"
	const dlqName = "backend.ai-results.dlq"
	const deadLetterKey = "backend.ai-results"
	arguments := amqp.Table{
		"x-queue-type":              "quorum",
		"x-single-active-consumer":  true,
		"x-dead-letter-exchange":    DeadLetterExchange,
		"x-dead-letter-routing-key": deadLetterKey,
	}
	queue, err := channel.QueueDeclare(queueName, true, false, false, false, arguments)
	if err != nil {
		return amqp.Queue{}, fmt.Errorf("declare queue %s: %w", queueName, err)
	}
	for _, key := range []string{"interview.voice.transcribed.v1", "interview.voice.transcription_failed.v1", "interview.answer.evaluated.v1", "interview.answer.evaluation_failed.v1", "interview.candidate.reported.v1", "interview.candidate.report_failed.v1", "interview.candidate.feedback_ready.v1", "interview.candidate.feedback_failed.v1"} {
		if err := channel.QueueBind(queue.Name, key, InterviewExchange, false, nil); err != nil {
			return amqp.Queue{}, fmt.Errorf("bind queue %s: %w", queueName, err)
		}
	}
	dlq, err := channel.QueueDeclare(dlqName, true, false, false, false, amqp.Table{"x-queue-type": "quorum"})
	if err != nil {
		return amqp.Queue{}, fmt.Errorf("declare DLQ %s: %w", dlqName, err)
	}
	if err := channel.QueueBind(dlq.Name, deadLetterKey, DeadLetterExchange, false, nil); err != nil {
		return amqp.Queue{}, fmt.Errorf("bind DLQ %s: %w", dlqName, err)
	}
	return queue, nil
}

func declareShardedQueue(
	channel *amqp.Channel,
	exchange, queuePrefix, routingPrefix string,
	shard int,
) (amqp.Queue, error) {
	name := fmt.Sprintf("%s.q.%d", queuePrefix, shard)
	dlqName := fmt.Sprintf("%s.dlq.%d", queuePrefix, shard)
	dlqRoutingKey := fmt.Sprintf("%s.%d", routingPrefix, shard)
	arguments := amqp.Table{
		"x-queue-type":              "quorum",
		"x-single-active-consumer":  true,
		"x-dead-letter-exchange":    DeadLetterExchange,
		"x-dead-letter-routing-key": dlqRoutingKey,
	}
	queue, err := channel.QueueDeclare(name, true, false, false, false, arguments)
	if err != nil {
		return amqp.Queue{}, fmt.Errorf("declare queue %s: %w", name, err)
	}
	if err := channel.QueueBind(name, fmt.Sprintf("%s.%d.#", routingPrefix, shard), exchange, false, nil); err != nil {
		return amqp.Queue{}, fmt.Errorf("bind queue %s: %w", name, err)
	}
	dlq, err := channel.QueueDeclare(dlqName, true, false, false, false, amqp.Table{"x-queue-type": "quorum"})
	if err != nil {
		return amqp.Queue{}, fmt.Errorf("declare DLQ %s: %w", dlqName, err)
	}
	if err := channel.QueueBind(dlq.Name, dlqRoutingKey, DeadLetterExchange, false, nil); err != nil {
		return amqp.Queue{}, fmt.Errorf("bind DLQ %s: %w", dlqName, err)
	}
	return queue, nil
}
