package domain

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestQuestionEvaluationGuideIsOptionalAndEditable(t *testing.T) {
	definition := InterviewDefinition{Vacancy: Vacancy{ID: uuid.New(), Status: VacancyPublished}}
	id := uuid.New()
	now := time.Now()
	require.NoError(t, definition.AddQuestion(Question{ID: id, Text: "Вопрос", AnswerType: QuestionAnswerText}, now))
	require.Nil(t, definition.Questions[0].EvaluationGuide)
	guide := "  Пример действий  "
	question, err := definition.EditQuestion(id, "Вопрос", QuestionAnswerVoice, now, &guide)
	require.NoError(t, err)
	require.Equal(t, "Пример действий", *question.EvaluationGuide)
	// A legacy PATCH that omits the guide must preserve it.
	question, err = definition.EditQuestion(id, "Новый вопрос", QuestionAnswerText, now)
	require.NoError(t, err)
	require.Equal(t, "Пример действий", *question.EvaluationGuide)
	for _, value := range []*string{nil, pointerToString(" \n\t\u00a0 ")} {
		question, err = definition.EditQuestion(id, "Вопрос", QuestionAnswerText, now, value)
		require.NoError(t, err)
		require.Nil(t, question.EvaluationGuide)
	}
	guide = strings.Repeat("я", 4000)
	_, err = definition.EditQuestion(id, "Вопрос", QuestionAnswerText, now, &guide)
	require.NoError(t, err)
	guide += "я"
	_, err = definition.EditQuestion(id, "Вопрос", QuestionAnswerText, now, &guide)
	require.Error(t, err)
	require.Len(t, []rune(*definition.Questions[0].EvaluationGuide), 4000)
}

func pointerToString(s string) *string { return &s }
