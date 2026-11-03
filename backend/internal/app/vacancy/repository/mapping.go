package repository

import (
	"time"

	"hr-assistant/backend/internal/core/database/dbgen"
	"hr-assistant/backend/internal/core/domain"

	"github.com/jackc/pgx/v5/pgtype"
)

func vacancyFromDB(value dbgen.Vacancy) domain.Vacancy {
	var expiresAt *time.Time
	if value.ExpiresAt.Valid {
		expiresAt = &value.ExpiresAt.Time
	}
	var publicToken *string
	if value.PublicToken.Valid {
		publicToken = &value.PublicToken.String
	}
	return domain.Vacancy{
		ID: value.ID, OwnerUserID: value.OwnerUserID, Title: value.Title, Description: optionalTextValue(value.Description),
		InterviewLanguage: value.InterviewLanguage, WelcomeMessage: value.WelcomeMessage,
		InterviewDurationMinutes: int(value.InterviewDurationMinutes),
		ExpiresAt:                expiresAt, Status: domain.VacancyStatus(value.Status), PublicToken: publicToken,
		CreatedAt: value.CreatedAt.Time, UpdatedAt: value.UpdatedAt.Time,
	}
}

func questionFromDB(value dbgen.Question) domain.Question {
	var guide *string
	if value.EvaluationGuide.Valid {
		guide = &value.EvaluationGuide.String
	}
	return domain.Question{
		ID: value.ID, VacancyID: value.VacancyID, Text: value.Text, EvaluationGuide: guide,
		AnswerType: domain.QuestionAnswerType(value.AnswerType), Position: value.Position,
		CreatedAt: value.CreatedAt.Time, UpdatedAt: value.UpdatedAt.Time,
	}
}

func timestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}

func optionalTimestamp(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return timestamp(*value)
}

func optionalTextValue(v pgtype.Text) *string {
	if !v.Valid {
		return nil
	}
	return &v.String
}
func optionalText(v *string) pgtype.Text {
	if v == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *v, Valid: true}
}
