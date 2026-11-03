package structured

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"hr-assistant/shared/evaluation"
)

func TestOriginalQuoteUsesOnlyAnUnambiguousOriginalExcerpt(t *testing.T) {
	for _, tc := range []struct {
		name, quote, source, want string
		ok                        bool
	}{
		{"case", "сейчас уточняю масштаб проблемы", "Сейчас уточняю масштаб проблемы и подключаю команду.", "Сейчас уточняю масштаб проблемы", true},
		{"whitespace", "сейчас уточняю", "Сейчас\nуточняю", "Сейчас уточняю", true},
		{"unicode offsets", "проверяю", "K: Проверяю", "Проверяю", true},
		{"preserve existing exact quote", "Сейчас", "Сейчас и СЕЙЧАС", "Сейчас", true},
		{"ambiguous", "сейчас", "Сейчас и СЕЙЧАС", "", false},
		{"punctuation", "спрошу клиента.", "Спрошу клиента?", "", false},
		{"changed words", "сейчас уточняю детали", "Сейчас уточняю масштаб", "", false},
		{"invented", "Я получил результат", "Другой ответ", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := originalQuote(tc.quote, tc.source)
			require.Equal(t, tc.ok, ok)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestRestoredAnswerStillRequiresStrictValidation(t *testing.T) {
	raw := `{"summary":"Есть пример.","checks":[{"criterion":"Уточнение","type":"expected","status":"found","evidence":["сейчас уточняю"],"explanation":"Описан шаг."}],"follow_up_questions":[]}`
	answer := "Сейчас уточняю масштаб проблемы."
	_, err := evaluation.Parse([]byte(raw), answer)
	require.Error(t, err) // The shared validator remains strictly case-sensitive.
	result, err := evaluation.Parse(RestoreAnswerEvidenceCase([]byte(raw), answer), answer)
	require.NoError(t, err)
	require.Equal(t, []string{"Сейчас уточняю"}, result.Checks[0].Evidence)
	for _, invalid := range []string{
		strings.Replace(raw, `"summary":`, `"extra":true,"summary":`, 1),
		strings.Replace(raw, `"found"`, `"not_found"`, 1),
		strings.Replace(raw, `"сейчас уточняю"`, `"придуманная цитата"`, 1),
		raw + `{}`,
	} {
		_, err := evaluation.Parse(RestoreAnswerEvidenceCase([]byte(invalid), answer), answer)
		require.Error(t, err)
	}
}

func TestReportRestorationStaysWithinReferencedAnswer(t *testing.T) {
	raw := []byte(`{"criterion_scores":[{"score":7.3,"evidence":[{"question_number":1,"quote":"я уточнил"}]}],"strengths":[{"evidence":[{"question_number":2,"quote":"я уточнил"}]}],"concerns":[],"unknown":true}`)
	restored := RestoreReportEvidenceCase(raw, map[int]string{1: "Я уточнил причину", 2: "Ответ не содержит этого примера"})
	require.JSONEq(t, `{"criterion_scores":[{"score":7.3,"evidence":[{"question_number":1,"quote":"Я уточнил"}]}],"strengths":[{"evidence":[{"question_number":2,"quote":"я уточнил"}]}],"concerns":[],"unknown":true}`, string(restored))
}
