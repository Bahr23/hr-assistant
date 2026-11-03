package domain

import (
	"github.com/google/uuid"
	"hr-assistant/backend/internal/core/criteria"
	"strings"
	"time"
)

type EvaluationCriterion struct {
	ID               uuid.UUID  `json:"id"`
	Title            string     `json:"title"`
	EvaluationGuide  string     `json:"evaluationGuide"`
	Position         int        `json:"position"`
	SourceTemplateID *uuid.UUID `json:"sourceTemplateId"`
}

func (d *InterviewDefinition) replaceCriteria(values []EvaluationCriterion, now time.Time) error {
	if len(values) > 20 {
		return ValidationFault("validation_failed", "evaluationCriteria", "must contain at most 20 criteria")
	}
	ids := map[uuid.UUID]bool{}
	result := make([]EvaluationCriterion, len(values))
	for _, value := range values {
		value.Title = strings.TrimSpace(value.Title)
		value.EvaluationGuide = strings.TrimSpace(value.EvaluationGuide)
		if value.ID == uuid.Nil || ids[value.ID] || value.Position < 0 || value.Position >= len(values) || result[value.Position].ID != uuid.Nil {
			return ValidationFault("validation_failed", "evaluationCriteria", "identities and contiguous positions must be unique")
		}
		if len([]rune(value.Title)) < 1 || len([]rune(value.Title)) > 120 || len([]rune(value.EvaluationGuide)) < 1 || len([]rune(value.EvaluationGuide)) > 2000 {
			return ValidationFault("validation_failed", "evaluationCriteria", "title must contain 1..120 and guide 1..2000 characters")
		}
		if value.SourceTemplateID != nil && !criteria.Exists(*value.SourceTemplateID) {
			return ValidationFault("validation_failed", "sourceTemplateId", "unknown template")
		}
		ids[value.ID] = true
		result[value.Position] = value
	}
	d.Vacancy.EvaluationCriteria = result
	d.touch(now)
	return nil
}
