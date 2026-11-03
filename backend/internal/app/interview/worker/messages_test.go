package worker

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestTelegramQuestionPayloadContainsRawQuestionData(t *testing.T) {
	payload := TelegramSendPayload{
		DeliveryID:     uuid.New(),
		ChatID:         13,
		Text:           "Расскажите о себе",
		AnswerType:     "text",
		QuestionNumber: 1,
		QuestionTotal:  2,
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"delivery_id":"`+payload.DeliveryID.String()+`",
		"chat_id":13,
		"text":"Расскажите о себе",
		"answer_type":"text",
		"question_number":1,
		"question_total":2
	}`, string(body))
	require.NotContains(t, string(body), "rich_html")
}
