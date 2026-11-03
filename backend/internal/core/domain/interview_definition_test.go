package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestPublishedDefinitionCannotLoseItsLastQuestion(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	expiresAt := now.Add(time.Hour)
	definition := InterviewDefinition{
		Vacancy: Vacancy{
			ID: uuid.New(), Title: "Developer",
			InterviewLanguage: "ru", WelcomeMessage: "Welcome", ExpiresAt: &expiresAt,
			Status: VacancyDraft,
		},
	}
	require.NoError(t, definition.AddQuestion(Question{ID: uuid.New(), Text: "Experience?", AnswerType: QuestionAnswerVoice}, now))
	require.NoError(t, definition.Publish("token", now))

	err := definition.RemoveQuestion(definition.Questions[0].ID, now.Add(time.Second))

	var fault *Fault
	require.ErrorAs(t, err, &fault)
	require.Equal(t, "last_question_required", fault.Code)
	require.Len(t, definition.Questions, 1)
}

func TestDefinitionCanBePublishedWithoutExpiration(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	definition := InterviewDefinition{
		Vacancy: Vacancy{
			ID: uuid.New(), Title: "Developer",
			InterviewLanguage: "ru", WelcomeMessage: "Welcome", Status: VacancyDraft,
		},
	}
	require.NoError(t, definition.AddQuestion(Question{ID: uuid.New(), Text: "Experience?", AnswerType: QuestionAnswerText}, now))

	require.NoError(t, definition.Publish("token", now))
	require.Equal(t, VacancyPublished, definition.Vacancy.Status)
	require.Equal(t, 15, definition.Vacancy.InterviewDurationMinutes)
}

func TestDefinitionRejectsUnsupportedInterviewDuration(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	definition := InterviewDefinition{
		Vacancy: Vacancy{
			ID: uuid.New(), Title: "Developer", InterviewLanguage: "ru",
			WelcomeMessage: "Welcome", InterviewDurationMinutes: 10, Status: VacancyDraft,
		},
	}
	require.NoError(t, definition.AddQuestion(Question{
		ID: uuid.New(), Text: "Experience?", AnswerType: QuestionAnswerText,
	}, now))

	err := definition.Publish("token", now)
	var fault *Fault
	require.ErrorAs(t, err, &fault)
	require.Equal(t, "interview_duration_minutes", fault.Field)
}

func TestDefinitionCannotBePublishedWithExpiredExpiration(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	expiresAt := now.Add(-time.Minute)
	definition := InterviewDefinition{
		Vacancy: Vacancy{
			ID: uuid.New(), Title: "Developer",
			InterviewLanguage: "ru", WelcomeMessage: "Welcome", ExpiresAt: &expiresAt, Status: VacancyDraft,
		},
	}
	require.NoError(t, definition.AddQuestion(Question{ID: uuid.New(), Text: "Experience?", AnswerType: QuestionAnswerVoice}, now))

	err := definition.Publish("token", now)
	var fault *Fault
	require.ErrorAs(t, err, &fault)
	require.Equal(t, "expires_at", fault.Field)
}

func TestQuestionRequiresSupportedAnswerType(t *testing.T) {
	now := time.Now().UTC()
	definition := InterviewDefinition{Vacancy: Vacancy{ID: uuid.New(), Status: VacancyDraft}}

	err := definition.AddQuestion(Question{ID: uuid.New(), Text: "Experience?", AnswerType: "video"}, now)

	var fault *Fault
	require.ErrorAs(t, err, &fault)
	require.Equal(t, "answer_type", fault.Field)
	require.Empty(t, definition.Questions)
}

func TestReorderIsAllOrNothing(t *testing.T) {
	now := time.Now().UTC()
	definition := InterviewDefinition{Vacancy: Vacancy{ID: uuid.New(), Status: VacancyDraft}}
	firstID, secondID := uuid.New(), uuid.New()
	require.NoError(t, definition.AddQuestion(Question{ID: firstID, Text: "First", AnswerType: QuestionAnswerText}, now))
	require.NoError(t, definition.AddQuestion(Question{ID: secondID, Text: "Second", AnswerType: QuestionAnswerVoice}, now))

	err := definition.ReorderQuestions([]uuid.UUID{secondID, uuid.New()}, now.Add(time.Second))

	require.Error(t, err)
	require.Equal(t, firstID, definition.Questions[0].ID)
	require.Equal(t, secondID, definition.Questions[1].ID)
}
