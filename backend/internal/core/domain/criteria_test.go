package domain

import (
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"hr-assistant/backend/internal/core/criteria"
	"strings"
	"testing"
	"time"
)

func TestCriteriaAndDescriptionValidation(t *testing.T) {
	now := time.Now()
	template := criteria.Templates().Items[0]
	valid := EvaluationCriterion{ID: uuid.New(), Title: "  Опыт  ", EvaluationGuide: "  Пример  ", Position: 0, SourceTemplateID: &template.ID}
	values := []EvaluationCriterion{valid}
	description := "  Описание  "
	d := InterviewDefinition{Vacancy: Vacancy{Status: VacancyDraft}}
	require.NoError(t, d.PatchVacancy(VacancyPatch{Description: &description, DescriptionSet: true, EvaluationCriteria: &values}, now))
	require.Equal(t, "Описание", *d.Vacancy.Description)
	require.Equal(t, "Опыт", d.Vacancy.EvaluationCriteria[0].Title)
	require.NoError(t, d.PatchVacancy(VacancyPatch{}, now))
	require.Len(t, d.Vacancy.EvaluationCriteria, 1)
	require.NoError(t, d.PatchVacancy(VacancyPatch{DescriptionSet: true}, now))
	require.Nil(t, d.Vacancy.Description)
	for _, mutate := range []func(*EvaluationCriterion){func(c *EvaluationCriterion) { c.ID = uuid.Nil }, func(c *EvaluationCriterion) { c.Title = " " }, func(c *EvaluationCriterion) { c.Title = strings.Repeat("я", 121) }, func(c *EvaluationCriterion) { c.EvaluationGuide = strings.Repeat("я", 2001) }, func(c *EvaluationCriterion) { c.Position = 1 }, func(c *EvaluationCriterion) { v := uuid.New(); c.SourceTemplateID = &v }} {
		c := valid
		mutate(&c)
		values := []EvaluationCriterion{c}
		require.Error(t, d.PatchVacancy(VacancyPatch{EvaluationCriteria: &values}, now))
	}
	values = []EvaluationCriterion{valid, valid}
	values[1].Position = 1
	require.Error(t, d.PatchVacancy(VacancyPatch{EvaluationCriteria: &values}, now))
	values[1].ID = uuid.New()
	require.NoError(t, d.PatchVacancy(VacancyPatch{EvaluationCriteria: &values}, now), "duplicate names allowed")
	values = []EvaluationCriterion{}
	require.NoError(t, d.PatchVacancy(VacancyPatch{EvaluationCriteria: &values}, now))
	require.Empty(t, d.Vacancy.EvaluationCriteria)
	description = strings.Repeat("я", 10001)
	require.Error(t, d.PatchVacancy(VacancyPatch{DescriptionSet: true, Description: &description}, now))
	d.Vacancy.Status = VacancyArchived
	require.Error(t, d.PatchVacancy(VacancyPatch{EvaluationCriteria: &values}, now))
}
