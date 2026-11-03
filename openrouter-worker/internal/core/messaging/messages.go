package messaging

import (
	"encoding/json"
	"fmt"
	feedback "hr-assistant/shared/candidatefeedback"
	report "hr-assistant/shared/candidatereport"
	"hr-assistant/shared/evaluation"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	InterviewExchange        = "interview.events"
	DeadLetterExchange       = "dead.letters"
	InterviewCompleted       = "interview.completed.v1"
	VoiceTranscribed         = "interview.voice.transcribed.v1"
	VoiceTranscriptionFailed = "interview.voice.transcription_failed.v1"
	InputQueue               = "openrouter.interview-completed.q"
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
	data, err := json.Marshal(payload)
	if err != nil {
		return Envelope{}, err
	}
	return Envelope{MessageID: uuid.New(), Type: messageType, SchemaVersion: 1,
		OccurredAt: time.Now().UTC(), CorrelationID: correlationID,
		PartitionKey: partitionKey, Payload: data}, nil
}

func DeclareTopology(channel *amqp.Channel) (amqp.Queue, error) {
	for _, name := range []string{InterviewExchange, DeadLetterExchange} {
		if err := channel.ExchangeDeclare(name, "topic", true, false, false, false, nil); err != nil {
			return amqp.Queue{}, fmt.Errorf("declare exchange %s: %w", name, err)
		}
	}
	arguments := amqp.Table{"x-queue-type": "quorum", "x-single-active-consumer": true,
		"x-dead-letter-exchange": DeadLetterExchange, "x-dead-letter-routing-key": "openrouter.interview-completed"}
	queue, err := channel.QueueDeclare(InputQueue, true, false, false, false, arguments)
	if err != nil {
		return amqp.Queue{}, err
	}
	if err := channel.QueueBind(queue.Name, InterviewCompleted, InterviewExchange, false, nil); err != nil {
		return amqp.Queue{}, err
	}
	if err := channel.QueueBind(queue.Name, feedback.Requested, InterviewExchange, false, nil); err != nil {
		return amqp.Queue{}, err
	}
	if err := channel.QueueBind(queue.Name, report.Requested, InterviewExchange, false, nil); err != nil {
		return amqp.Queue{}, err
	}
	if err := channel.QueueBind(queue.Name, evaluation.Requested, InterviewExchange, false, nil); err != nil {
		return amqp.Queue{}, err
	}
	dlq, err := channel.QueueDeclare("openrouter.interview-completed.dlq", true, false, false, false, amqp.Table{"x-queue-type": "quorum"})
	if err != nil {
		return amqp.Queue{}, err
	}
	if err := channel.QueueBind(dlq.Name, "openrouter.interview-completed", DeadLetterExchange, false, nil); err != nil {
		return amqp.Queue{}, err
	}
	return queue, nil
}
