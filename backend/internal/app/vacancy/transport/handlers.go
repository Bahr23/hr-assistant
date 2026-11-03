package transport

import (
	"context"
	"time"

	"hr-assistant/backend/internal/app/vacancy/service"
	"hr-assistant/backend/internal/app/vacancy/transport/contract"
	"hr-assistant/backend/internal/core/criteria"
	"hr-assistant/backend/internal/core/domain"

	"github.com/google/uuid"
	"github.com/oapi-codegen/nullable"
)

func (s *Server) ListVacancies(ctx context.Context, request contract.ListVacanciesRequestObject) (contract.ListVacanciesResponseObject, error) {
	query := service.ListQuery{Limit: 20}
	if request.Params.Limit != nil {
		query.Limit = int32(*request.Params.Limit)
	}
	if request.Params.Offset != nil {
		query.Offset = int32(*request.Params.Offset)
	}
	if request.Params.Status != nil {
		status := domain.VacancyStatus(*request.Params.Status)
		query.Status = &status
	}
	values, err := s.definitions.List(ctx, principalFromContext(ctx).HRID, query)
	if err != nil {
		kind, value := fault(ctx, err)
		if kind == domain.FaultValidation {
			return contract.ListVacancies422JSONResponse{ValidationErrorJSONResponse: contract.ValidationErrorJSONResponse(value)}, nil
		}
		return nil, err
	}
	items := make([]contract.Vacancy, 0, len(values))
	for _, value := range values {
		items = append(items, vacancyResponse(value))
	}
	return contract.ListVacancies200JSONResponse{
		Items: items, Limit: int(query.Limit), Offset: int(query.Offset),
	}, nil
}

func (s *Server) CreateVacancy(ctx context.Context, request contract.CreateVacancyRequestObject) (contract.CreateVacancyResponseObject, error) {
	if request.Body == nil {
		value := contractError(ctx, "invalid_request", "Request body is required", nil)
		return contract.CreateVacancy422JSONResponse{ValidationErrorJSONResponse: contract.ValidationErrorJSONResponse(value)}, nil
	}
	questions := make([]service.QuestionFields, 0, len(request.Body.Questions))
	for _, question := range request.Body.Questions {
		questions = append(questions, service.QuestionFields{
			Text: question.Text, AnswerType: domain.QuestionAnswerType(question.AnswerType), EvaluationGuide: nullableGuide(question.EvaluationGuide),
		})
	}
	result, err := s.definitions.Change(ctx, principalFromContext(ctx).HRID, service.CreateActive{
		Fields: service.VacancyFields{
			Description: nullableGuide(request.Body.Description), EvaluationCriteria: criteriaInput(request.Body.EvaluationCriteria),
			Title: request.Body.Title, InterviewLanguage: request.Body.InterviewLanguage,
			WelcomeMessage:           request.Body.WelcomeMessage,
			InterviewDurationMinutes: int(request.Body.InterviewDurationMinutes),
		},
		Questions: questions,
	})
	if err != nil {
		kind, value := fault(ctx, err)
		if kind == domain.FaultValidation {
			return contract.CreateVacancy422JSONResponse{ValidationErrorJSONResponse: contract.ValidationErrorJSONResponse(value)}, nil
		}
		return nil, err
	}
	return contract.CreateVacancy201JSONResponse(vacancyResponse(result.Definition.Vacancy)), nil
}

func (s *Server) GetVacancy(ctx context.Context, request contract.GetVacancyRequestObject) (contract.GetVacancyResponseObject, error) {
	definition, err := s.definitions.Get(ctx, principalFromContext(ctx).HRID, request.VacancyId)
	if err != nil {
		kind, value := fault(ctx, err)
		if kind == domain.FaultNotFound {
			return contract.GetVacancy404JSONResponse{NotFoundJSONResponse: contract.NotFoundJSONResponse(value)}, nil
		}
		return nil, err
	}
	vacancy := vacancyResponse(definition.Vacancy)
	return contract.GetVacancy200JSONResponse{
		Id: vacancy.Id, Title: vacancy.Title, InterviewLanguage: vacancy.InterviewLanguage, Description: vacancy.Description, EvaluationCriteria: vacancy.EvaluationCriteria,
		WelcomeMessage:           vacancy.WelcomeMessage,
		InterviewDurationMinutes: contract.GetVacancy200JSONResponseBodyInterviewDurationMinutes(vacancy.InterviewDurationMinutes),
		ExpiresAt:                vacancy.ExpiresAt, Status: vacancy.Status,
		InterviewUrl: vacancy.InterviewUrl, CreatedAt: vacancy.CreatedAt, UpdatedAt: vacancy.UpdatedAt,
		Questions: questionResponses(definition.Questions),
	}, nil
}

func (s *Server) GetVacancyWelcomeMessage(
	ctx context.Context,
	request contract.GetVacancyWelcomeMessageRequestObject,
) (contract.GetVacancyWelcomeMessageResponseObject, error) {
	entry, err := s.definitions.GetPublicEntry(
		ctx,
		request.PublicToken,
		request.Params.TelegramUserId,
	)
	if err != nil {
		kind, value := fault(ctx, err)
		if kind == domain.FaultNotFound {
			return contract.GetVacancyWelcomeMessage404JSONResponse{
				NotFoundJSONResponse: contract.NotFoundJSONResponse(value),
			}, nil
		}
		return nil, err
	}
	return contract.GetVacancyWelcomeMessage200JSONResponse{
		WelcomeMessage: entry.WelcomeMessage, Description: guideResponse(entry.Description),
		AlreadyAttempted: entry.AlreadyAttempted,
	}, nil
}

func (s *Server) UpdateVacancy(ctx context.Context, request contract.UpdateVacancyRequestObject) (contract.UpdateVacancyResponseObject, error) {
	if request.Body == nil {
		value := contractError(ctx, "invalid_request", "Request body is required", nil)
		return contract.UpdateVacancy422JSONResponse{ValidationErrorJSONResponse: contract.ValidationErrorJSONResponse(value)}, nil
	}
	result, err := s.definitions.Change(ctx, principalFromContext(ctx).HRID, service.PatchVacancy{
		VacancyID: request.VacancyId, Patch: vacancyPatch(*request.Body),
	})
	if err != nil {
		kind, value := fault(ctx, err)
		if kind == domain.FaultNotFound {
			return contract.UpdateVacancy404JSONResponse{NotFoundJSONResponse: contract.NotFoundJSONResponse(value)}, nil
		}
		if kind == domain.FaultConflict {
			return contract.UpdateVacancy409JSONResponse{ConflictJSONResponse: contract.ConflictJSONResponse(value)}, nil
		}
		if kind == domain.FaultValidation {
			return contract.UpdateVacancy422JSONResponse{ValidationErrorJSONResponse: contract.ValidationErrorJSONResponse(value)}, nil
		}
		return nil, err
	}
	return contract.UpdateVacancy200JSONResponse(vacancyResponse(result.Definition.Vacancy)), nil
}

func (s *Server) ArchiveVacancy(ctx context.Context, request contract.ArchiveVacancyRequestObject) (contract.ArchiveVacancyResponseObject, error) {
	result, err := s.definitions.Change(ctx, principalFromContext(ctx).HRID, service.Archive{VacancyID: request.VacancyId})
	if err != nil {
		kind, value := fault(ctx, err)
		if kind == domain.FaultNotFound {
			return contract.ArchiveVacancy404JSONResponse{NotFoundJSONResponse: contract.NotFoundJSONResponse(value)}, nil
		}
		if kind == domain.FaultConflict {
			return contract.ArchiveVacancy409JSONResponse{ConflictJSONResponse: contract.ConflictJSONResponse(value)}, nil
		}
		return nil, err
	}
	return contract.ArchiveVacancy200JSONResponse{VacancyActionOKJSONResponse: contract.VacancyActionOKJSONResponse(vacancyResponse(result.Definition.Vacancy))}, nil
}

func (s *Server) RestoreVacancy(ctx context.Context, request contract.RestoreVacancyRequestObject) (contract.RestoreVacancyResponseObject, error) {
	result, err := s.definitions.Change(ctx, principalFromContext(ctx).HRID, service.Restore{VacancyID: request.VacancyId})
	if err != nil {
		kind, value := fault(ctx, err)
		if kind == domain.FaultNotFound {
			return contract.RestoreVacancy404JSONResponse{NotFoundJSONResponse: contract.NotFoundJSONResponse(value)}, nil
		}
		if kind == domain.FaultConflict {
			return contract.RestoreVacancy409JSONResponse{ConflictJSONResponse: contract.ConflictJSONResponse(value)}, nil
		}
		if kind == domain.FaultValidation {
			return contract.RestoreVacancy422JSONResponse{ValidationErrorJSONResponse: contract.ValidationErrorJSONResponse(value)}, nil
		}
		return nil, err
	}
	return contract.RestoreVacancy200JSONResponse{VacancyActionOKJSONResponse: contract.VacancyActionOKJSONResponse(vacancyResponse(result.Definition.Vacancy))}, nil
}

func (s *Server) RotateVacancyLink(ctx context.Context, request contract.RotateVacancyLinkRequestObject) (contract.RotateVacancyLinkResponseObject, error) {
	result, err := s.definitions.Change(ctx, principalFromContext(ctx).HRID, service.RotatePublicLink{VacancyID: request.VacancyId})
	if err != nil {
		kind, value := fault(ctx, err)
		if kind == domain.FaultNotFound {
			return contract.RotateVacancyLink404JSONResponse{NotFoundJSONResponse: contract.NotFoundJSONResponse(value)}, nil
		}
		if kind == domain.FaultConflict {
			return contract.RotateVacancyLink409JSONResponse{ConflictJSONResponse: contract.ConflictJSONResponse(value)}, nil
		}
		return nil, err
	}
	return contract.RotateVacancyLink200JSONResponse{VacancyActionOKJSONResponse: contract.VacancyActionOKJSONResponse(vacancyResponse(result.Definition.Vacancy))}, nil
}

func (s *Server) ListQuestions(ctx context.Context, request contract.ListQuestionsRequestObject) (contract.ListQuestionsResponseObject, error) {
	definition, err := s.definitions.Get(ctx, principalFromContext(ctx).HRID, request.VacancyId)
	if err != nil {
		kind, value := fault(ctx, err)
		if kind == domain.FaultNotFound {
			return contract.ListQuestions404JSONResponse{NotFoundJSONResponse: contract.NotFoundJSONResponse(value)}, nil
		}
		return nil, err
	}
	return contract.ListQuestions200JSONResponse{Items: questionResponses(definition.Questions)}, nil
}

func (s *Server) CreateQuestion(ctx context.Context, request contract.CreateQuestionRequestObject) (contract.CreateQuestionResponseObject, error) {
	if request.Body == nil {
		value := contractError(ctx, "invalid_request", "Request body is required", nil)
		return contract.CreateQuestion422JSONResponse{ValidationErrorJSONResponse: contract.ValidationErrorJSONResponse(value)}, nil
	}
	result, err := s.definitions.Change(ctx, principalFromContext(ctx).HRID, service.AddQuestion{
		VacancyID: request.VacancyId, Text: request.Body.Text,
		AnswerType: domain.QuestionAnswerType(request.Body.AnswerType), EvaluationGuide: nullableGuide(request.Body.EvaluationGuide),
	})
	if err != nil {
		kind, value := fault(ctx, err)
		if kind == domain.FaultNotFound {
			return contract.CreateQuestion404JSONResponse{NotFoundJSONResponse: contract.NotFoundJSONResponse(value)}, nil
		}
		if kind == domain.FaultConflict {
			return contract.CreateQuestion409JSONResponse{ConflictJSONResponse: contract.ConflictJSONResponse(value)}, nil
		}
		if kind == domain.FaultValidation {
			return contract.CreateQuestion422JSONResponse{ValidationErrorJSONResponse: contract.ValidationErrorJSONResponse(value)}, nil
		}
		return nil, err
	}
	return contract.CreateQuestion201JSONResponse(questionResponse(*result.Question)), nil
}

func (s *Server) UpdateQuestion(ctx context.Context, request contract.UpdateQuestionRequestObject) (contract.UpdateQuestionResponseObject, error) {
	if request.Body == nil {
		value := contractError(ctx, "invalid_request", "Request body is required", nil)
		return contract.UpdateQuestion422JSONResponse{ValidationErrorJSONResponse: contract.ValidationErrorJSONResponse(value)}, nil
	}
	result, err := s.definitions.Change(ctx, principalFromContext(ctx).HRID, service.EditQuestion{
		VacancyID: request.VacancyId, QuestionID: request.QuestionId, Text: request.Body.Text,
		EvaluationGuideSet: request.Body.EvaluationGuide.IsSpecified(),
		AnswerType:         domain.QuestionAnswerType(request.Body.AnswerType), EvaluationGuide: nullableGuide(request.Body.EvaluationGuide),
	})
	if err != nil {
		kind, value := fault(ctx, err)
		if kind == domain.FaultNotFound {
			return contract.UpdateQuestion404JSONResponse{NotFoundJSONResponse: contract.NotFoundJSONResponse(value)}, nil
		}
		if kind == domain.FaultConflict {
			return contract.UpdateQuestion409JSONResponse{ConflictJSONResponse: contract.ConflictJSONResponse(value)}, nil
		}
		if kind == domain.FaultValidation {
			return contract.UpdateQuestion422JSONResponse{ValidationErrorJSONResponse: contract.ValidationErrorJSONResponse(value)}, nil
		}
		return nil, err
	}
	return contract.UpdateQuestion200JSONResponse(questionResponse(*result.Question)), nil
}

func (s *Server) DeleteQuestion(ctx context.Context, request contract.DeleteQuestionRequestObject) (contract.DeleteQuestionResponseObject, error) {
	_, err := s.definitions.Change(ctx, principalFromContext(ctx).HRID, service.RemoveQuestion{
		VacancyID: request.VacancyId, QuestionID: request.QuestionId,
	})
	if err != nil {
		kind, value := fault(ctx, err)
		if kind == domain.FaultNotFound {
			return contract.DeleteQuestion404JSONResponse{NotFoundJSONResponse: contract.NotFoundJSONResponse(value)}, nil
		}
		if kind == domain.FaultConflict {
			return contract.DeleteQuestion409JSONResponse{ConflictJSONResponse: contract.ConflictJSONResponse(value)}, nil
		}
		return nil, err
	}
	return contract.DeleteQuestion204Response{}, nil
}

func (s *Server) ReorderQuestions(ctx context.Context, request contract.ReorderQuestionsRequestObject) (contract.ReorderQuestionsResponseObject, error) {
	if request.Body == nil {
		value := contractError(ctx, "invalid_request", "Request body is required", nil)
		return contract.ReorderQuestions422JSONResponse{ValidationErrorJSONResponse: contract.ValidationErrorJSONResponse(value)}, nil
	}
	ids := append([]uuid.UUID(nil), request.Body.QuestionIds...)
	result, err := s.definitions.Change(ctx, principalFromContext(ctx).HRID, service.ReorderQuestions{
		VacancyID: request.VacancyId, QuestionIDs: ids,
	})
	if err != nil {
		kind, value := fault(ctx, err)
		if kind == domain.FaultNotFound {
			return contract.ReorderQuestions404JSONResponse{NotFoundJSONResponse: contract.NotFoundJSONResponse(value)}, nil
		}
		if kind == domain.FaultConflict {
			return contract.ReorderQuestions409JSONResponse{ConflictJSONResponse: contract.ConflictJSONResponse(value)}, nil
		}
		if kind == domain.FaultValidation {
			return contract.ReorderQuestions422JSONResponse{ValidationErrorJSONResponse: contract.ValidationErrorJSONResponse(value)}, nil
		}
		return nil, err
	}
	return contract.ReorderQuestions200JSONResponse{Items: questionResponses(result.Definition.Questions)}, nil
}

func vacancyPatch(input contract.VacancyPatch) domain.VacancyPatch {
	return domain.VacancyPatch{
		Description: nullableGuide(input.Description), DescriptionSet: input.Description.IsSpecified(), EvaluationCriteria: criteriaPatch(input.EvaluationCriteria),
		Title: input.Title, InterviewLanguage: input.InterviewLanguage, WelcomeMessage: input.WelcomeMessage,
		InterviewDurationMinutes: vacancyDurationPatch(input.InterviewDurationMinutes),
		ExpiresAt:                nullableTime(input.ExpiresAt), ExpiresAtSet: input.ExpiresAt.IsSpecified(),
	}
}

func nullableTime(value nullable.Nullable[time.Time]) *time.Time {
	if value.IsNull() || !value.IsSpecified() {
		return nil
	}
	result, err := value.Get()
	if err != nil {
		return nil
	}
	return &result
}

func vacancyResponse(value service.Vacancy) contract.Vacancy {
	result := contract.Vacancy{
		Description: guideResponse(value.Description), EvaluationCriteria: criteriaResponse(value.EvaluationCriteria),
		Id: value.ID, Title: value.Title, InterviewLanguage: value.InterviewLanguage,
		WelcomeMessage:           value.WelcomeMessage,
		InterviewDurationMinutes: contract.VacancyInterviewDurationMinutes(value.InterviewDurationMinutes),
		Status:                   contract.VacancyStatus(value.Status),
		CreatedAt:                value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
	if value.ExpiresAt == nil {
		result.ExpiresAt.SetNull()
	} else {
		result.ExpiresAt.Set(*value.ExpiresAt)
	}
	if value.PublicLink == nil {
		result.InterviewUrl.SetNull()
	} else {
		result.InterviewUrl.Set(*value.PublicLink)
	}
	return result
}

func vacancyDurationPatch(value *contract.VacancyInputInterviewDurationMinutes) *int {
	if value == nil {
		return nil
	}
	result := int(*value)
	return &result
}

func questionResponses(values []domain.Question) []contract.Question {
	result := make([]contract.Question, 0, len(values))
	for _, value := range values {
		result = append(result, questionResponse(value))
	}
	return result
}

func questionResponse(value domain.Question) contract.Question {
	return contract.Question{
		Id: value.ID, VacancyId: value.VacancyID, Text: value.Text, EvaluationGuide: guideResponse(value.EvaluationGuide),
		AnswerType: contract.QuestionAnswerType(value.AnswerType), Position: int(value.Position),
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
}

func nullableGuide(value nullable.Nullable[string]) *string {
	if !value.IsSpecified() || value.IsNull() {
		return nil
	}
	result, err := value.Get()
	if err != nil {
		return nil
	}
	return &result
}
func guideResponse(value *string) nullable.Nullable[string] {
	if value == nil {
		return nullable.NewNullNullable[string]()
	}
	return nullable.NewNullableWithValue(*value)
}

func criteriaInput(values *[]contract.EvaluationCriterion) []domain.EvaluationCriterion {
	result := []domain.EvaluationCriterion{}
	if values == nil {
		return result
	}
	for _, c := range *values {
		var source *uuid.UUID
		if c.SourceTemplateId.IsSpecified() && !c.SourceTemplateId.IsNull() {
			v, _ := c.SourceTemplateId.Get()
			source = &v
		}
		result = append(result, domain.EvaluationCriterion{ID: c.Id, Title: c.Title, EvaluationGuide: c.EvaluationGuide, Position: c.Position, SourceTemplateID: source})
	}
	return result
}
func criteriaPatch(values *[]contract.EvaluationCriterion) *[]domain.EvaluationCriterion {
	if values == nil {
		return nil
	}
	v := criteriaInput(values)
	return &v
}
func criteriaResponse(values []domain.EvaluationCriterion) []contract.EvaluationCriterion {
	result := make([]contract.EvaluationCriterion, 0, len(values))
	for _, c := range values {
		source := nullable.NewNullNullable[uuid.UUID]()
		if c.SourceTemplateID != nil {
			source.Set(*c.SourceTemplateID)
		}
		result = append(result, contract.EvaluationCriterion{Id: c.ID, Title: c.Title, EvaluationGuide: c.EvaluationGuide, Position: c.Position, SourceTemplateId: source})
	}
	return result
}
func (s *Server) ListEvaluationCriterionTemplates(ctx context.Context, request contract.ListEvaluationCriterionTemplatesRequestObject) (contract.ListEvaluationCriterionTemplatesResponseObject, error) {
	catalog := criteria.Templates()
	items := make([]contract.EvaluationCriterionTemplate, 0, len(catalog.Items))
	for _, t := range catalog.Items {
		items = append(items, contract.EvaluationCriterionTemplate{Id: t.ID, Title: t.Title, EvaluationGuide: t.EvaluationGuide})
	}
	return contract.ListEvaluationCriterionTemplates200JSONResponse{Version: catalog.Version, Items: items}, nil
}
