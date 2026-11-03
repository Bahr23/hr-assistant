package domain

import (
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

type InterviewDefinition struct {
	Vacancy   Vacancy
	Questions []Question
}

type VacancyPatch struct {
	Description              *string
	DescriptionSet           bool
	EvaluationCriteria       *[]EvaluationCriterion
	Title                    *string
	InterviewLanguage        *string
	WelcomeMessage           *string
	InterviewDurationMinutes *int
	ExpiresAt                *time.Time
	ExpiresAtSet             bool
}

func (d *InterviewDefinition) PatchVacancy(patch VacancyPatch, now time.Time) error {
	if d.Vacancy.Status == VacancyArchived {
		return NewFault(FaultConflict, "invalid_vacancy_state", "archived vacancy must be restored before editing")
	}
	applyString := func(target *string, value *string) {
		if value != nil {
			*target = strings.TrimSpace(*value)
		}
	}
	if patch.DescriptionSet {
		d.Vacancy.Description = nil
		if patch.Description != nil {
			value := strings.TrimSpace(*patch.Description)
			if len([]rune(value)) > 10000 {
				return ValidationFault("validation_failed", "description", "must not exceed 10000 characters")
			}
			if value != "" {
				d.Vacancy.Description = &value
			}
		}
	}
	if patch.EvaluationCriteria != nil {
		if err := d.replaceCriteria(*patch.EvaluationCriteria, now); err != nil {
			return err
		}
	}
	applyString(&d.Vacancy.Title, patch.Title)
	applyString(&d.Vacancy.InterviewLanguage, patch.InterviewLanguage)
	applyString(&d.Vacancy.WelcomeMessage, patch.WelcomeMessage)
	if patch.InterviewDurationMinutes != nil {
		d.Vacancy.InterviewDurationMinutes = *patch.InterviewDurationMinutes
	}
	if patch.ExpiresAtSet {
		d.Vacancy.ExpiresAt = nil
		if patch.ExpiresAt != nil {
			value := patch.ExpiresAt.UTC()
			d.Vacancy.ExpiresAt = &value
		}
	}
	if err := validateVacancyLengths(d.Vacancy); err != nil {
		return err
	}
	if d.Vacancy.Status == VacancyPublished {
		if err := d.validateForPublication(now); err != nil {
			return err
		}
	}
	d.touch(now)
	return nil
}

func (d *InterviewDefinition) Publish(token string, now time.Time) error {
	if d.Vacancy.Status != VacancyDraft {
		return NewFault(FaultConflict, "invalid_vacancy_state", "only a draft vacancy can be published")
	}
	if d.Vacancy.InterviewDurationMinutes == 0 {
		d.Vacancy.InterviewDurationMinutes = 15
	}
	if err := d.validateForPublication(now); err != nil {
		return err
	}
	d.Vacancy.Status = VacancyPublished
	d.Vacancy.PublicToken = &token
	d.touch(now)
	return nil
}

func (d *InterviewDefinition) Archive(now time.Time) error {
	if d.Vacancy.Status != VacancyPublished {
		return NewFault(FaultConflict, "invalid_vacancy_state", "only a published vacancy can be archived")
	}
	d.Vacancy.Status = VacancyArchived
	d.Vacancy.PublicToken = nil
	d.touch(now)
	return nil
}

func (d *InterviewDefinition) Restore(token string, now time.Time) error {
	if d.Vacancy.Status != VacancyArchived {
		return NewFault(FaultConflict, "invalid_vacancy_state", "only an archived vacancy can be restored")
	}
	if err := d.validateForPublication(now); err != nil {
		return err
	}
	d.Vacancy.Status = VacancyPublished
	d.Vacancy.PublicToken = &token
	d.touch(now)
	return nil
}

func (d *InterviewDefinition) RotatePublicLink(token string, now time.Time) error {
	if d.Vacancy.Status != VacancyPublished {
		return NewFault(FaultConflict, "invalid_vacancy_state", "public link belongs to a published vacancy")
	}
	d.Vacancy.PublicToken = &token
	d.touch(now)
	return nil
}

func (d *InterviewDefinition) AddQuestion(question Question, now time.Time) error {
	if err := d.ensureEditable(); err != nil {
		return err
	}
	text, err := normalizeQuestionText(question.Text)
	if err != nil {
		return err
	}
	if !question.AnswerType.Valid() {
		return ValidationFault("validation_failed", "answer_type", "must be text or voice")
	}
	guide, err := normalizeEvaluationGuide(question.EvaluationGuide)
	if err != nil {
		return err
	}
	question.EvaluationGuide = guide
	question.Text = text
	question.VacancyID = d.Vacancy.ID
	question.Position = int32(len(d.Questions))
	question.CreatedAt = now
	question.UpdatedAt = now
	d.Questions = append(d.Questions, question)
	d.touch(now)
	return nil
}

func (d *InterviewDefinition) EditQuestion(questionID uuid.UUID, text string, answerType QuestionAnswerType, now time.Time, guidePatch ...*string) (*Question, error) {
	if err := d.ensureEditable(); err != nil {
		return nil, err
	}
	value, err := normalizeQuestionText(text)
	if err != nil {
		return nil, err
	}
	if !answerType.Valid() {
		return nil, ValidationFault("validation_failed", "answer_type", "must be text or voice")
	}
	var guide *string
	if len(guidePatch) > 0 {
		guide, err = normalizeEvaluationGuide(guidePatch[0])
		if err != nil {
			return nil, err
		}
	}
	for index := range d.Questions {
		if d.Questions[index].ID == questionID {
			if len(guidePatch) > 0 {
				d.Questions[index].EvaluationGuide = guide
			}
			d.Questions[index].Text = value
			d.Questions[index].AnswerType = answerType
			d.Questions[index].UpdatedAt = now
			d.touch(now)
			return &d.Questions[index], nil
		}
	}
	return nil, NewFault(FaultNotFound, "question_not_found", "question not found")
}

func (d *InterviewDefinition) RemoveQuestion(questionID uuid.UUID, now time.Time) error {
	if err := d.ensureEditable(); err != nil {
		return err
	}
	index := slices.IndexFunc(d.Questions, func(question Question) bool { return question.ID == questionID })
	if index < 0 {
		return NewFault(FaultNotFound, "question_not_found", "question not found")
	}
	if d.Vacancy.Status == VacancyPublished && len(d.Questions) == 1 {
		return NewFault(FaultConflict, "last_question_required", "published vacancy must keep at least one question")
	}
	d.Questions = append(d.Questions[:index], d.Questions[index+1:]...)
	d.reindexQuestions(now)
	d.touch(now)
	return nil
}

func (d *InterviewDefinition) ReorderQuestions(ids []uuid.UUID, now time.Time) error {
	if err := d.ensureEditable(); err != nil {
		return err
	}
	if len(ids) != len(d.Questions) {
		return ValidationFault("invalid_question_order", "question_ids", "must contain every question exactly once")
	}
	byID := make(map[uuid.UUID]Question, len(d.Questions))
	for _, question := range d.Questions {
		byID[question.ID] = question
	}
	reordered := make([]Question, 0, len(ids))
	for _, id := range ids {
		question, ok := byID[id]
		if !ok {
			return ValidationFault("invalid_question_order", "question_ids", "must contain every question exactly once")
		}
		delete(byID, id)
		reordered = append(reordered, question)
	}
	if len(byID) != 0 {
		return ValidationFault("invalid_question_order", "question_ids", "must contain every question exactly once")
	}
	d.Questions = reordered
	d.reindexQuestions(now)
	d.touch(now)
	return nil
}

func (d *InterviewDefinition) validateForPublication(now time.Time) error {
	required := []struct{ field, value string }{
		{"title", d.Vacancy.Title}, {"interview_language", d.Vacancy.InterviewLanguage},
		{"welcome_message", d.Vacancy.WelcomeMessage},
	}
	for _, item := range required {
		if strings.TrimSpace(item.value) == "" {
			return ValidationFault("publication_incomplete", item.field, "is required for publication")
		}
	}
	if d.Vacancy.ExpiresAt != nil && !d.Vacancy.ExpiresAt.After(now) {
		return ValidationFault("publication_incomplete", "expires_at", "must be in the future")
	}
	if !validInterviewDuration(d.Vacancy.InterviewDurationMinutes) {
		return ValidationFault("publication_incomplete", "interview_duration_minutes", "must be one of 5, 15, 30 or 60")
	}
	if len(d.Questions) == 0 {
		return ValidationFault("publication_incomplete", "questions", "at least one question is required")
	}
	return nil
}

func validInterviewDuration(value int) bool {
	return value == 5 || value == 15 || value == 30 || value == 60
}

func validateVacancyLengths(v Vacancy) error {
	for field, value := range map[string]string{
		"title": v.Title, "interview_language": v.InterviewLanguage,
	} {
		if len([]rune(value)) > 255 {
			return ValidationFault("validation_failed", field, "must not exceed 255 characters")
		}
	}
	for field, value := range map[string]string{
		"welcome_message": v.WelcomeMessage,
	} {
		if len([]rune(value)) > 10000 {
			return ValidationFault("validation_failed", field, "must not exceed 10000 characters")
		}
	}
	return nil
}

func normalizeQuestionText(text string) (string, error) {
	value := strings.TrimSpace(text)
	if value == "" || len([]rune(value)) > 2000 {
		return "", ValidationFault("validation_failed", "text", "must contain 1 to 2000 characters")
	}
	return value, nil
}

func (d *InterviewDefinition) reindexQuestions(now time.Time) {
	for index := range d.Questions {
		if d.Questions[index].Position != int32(index) {
			d.Questions[index].Position = int32(index)
			d.Questions[index].UpdatedAt = now
		}
	}
}

func (d *InterviewDefinition) touch(now time.Time) { d.Vacancy.UpdatedAt = now }

func (d *InterviewDefinition) ensureEditable() error {
	if d.Vacancy.Status == VacancyArchived {
		return NewFault(FaultConflict, "invalid_vacancy_state", "archived vacancy must be restored before editing")
	}
	return nil
}

func normalizeEvaluationGuide(value *string) (*string, error) {
	if value == nil {
		return nil, nil
	}
	text := strings.TrimSpace(*value)
	if len([]rune(text)) > 4000 {
		return nil, ValidationFault("validation_failed", "evaluationGuide", "must not exceed 4000 characters")
	}
	if text == "" {
		return nil, nil
	}
	return &text, nil
}
