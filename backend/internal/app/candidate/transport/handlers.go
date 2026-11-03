package transport

import (
	"context"

	"hr-assistant/backend/internal/app/candidate/service"
	"hr-assistant/backend/internal/app/candidate/transport/contract"
	"hr-assistant/backend/internal/app/candidatereport"
	"hr-assistant/backend/internal/core/domain"

	"github.com/oapi-codegen/nullable"
)

func (s *Server) ListCandidates(ctx context.Context, request contract.ListCandidatesRequestObject) (contract.ListCandidatesResponseObject, error) {
	query := service.ListQuery{Limit: 20}
	if request.Params.Search != nil {
		query.Search = *request.Params.Search
	}
	if request.Params.VacancyId != nil {
		query.VacancyID = request.Params.VacancyId
	}
	if request.Params.Limit != nil {
		query.Limit = int32(*request.Params.Limit)
	}
	if request.Params.Offset != nil {
		query.Offset = int32(*request.Params.Offset)
	}
	page, err := s.candidates.List(ctx, principalFromContext(ctx).HRID, query)
	if err != nil {
		kind, value := fault(ctx, err)
		if kind == domain.FaultValidation {
			return contract.ListCandidates422JSONResponse{ValidationErrorJSONResponse: contract.ValidationErrorJSONResponse(value)}, nil
		}
		return nil, err
	}
	items := make([]contract.CandidateInterviewListItem, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, summaryResponse(item))
	}
	return contract.ListCandidates200JSONResponse{
		Items: items, Total: page.Total, Limit: int(page.Limit), Offset: int(page.Offset),
	}, nil
}

func (s *Server) GetCandidate(ctx context.Context, request contract.GetCandidateRequestObject) (contract.GetCandidateResponseObject, error) {
	interview, err := s.candidates.Get(ctx, principalFromContext(ctx).HRID, request.SessionId)
	if err != nil {
		kind, value := fault(ctx, err)
		if kind == domain.FaultNotFound {
			return contract.GetCandidate404JSONResponse{NotFoundJSONResponse: contract.NotFoundJSONResponse(value)}, nil
		}
		return nil, err
	}
	return contract.GetCandidate200JSONResponse(detailResponse(interview)), nil
}

func summaryResponse(value service.Summary) contract.CandidateInterviewListItem {
	return contract.CandidateInterviewListItem{
		Id: value.ID, Candidate: profileResponse(value.Candidate),
		Vacancy: vacancyResponse(value.Vacancy), StartedAt: value.StartedAt,
		CompletedAt: value.CompletedAt, DurationSeconds: value.DurationSeconds,
		AnswerCount: value.AnswerCount,
	}
}

func detailResponse(value service.Interview) contract.CandidateInterviewDetail {
	answers := make([]contract.CandidateInterviewAnswer, 0, len(value.Answers))
	for _, answer := range value.Answers {
		response := contract.CandidateInterviewAnswer{
			Id: answer.ID, Position: answer.Position, Question: answer.Question,
			AnswerType: contract.QuestionAnswerType(answer.AnswerType), AnsweredAt: answer.AnsweredAt,
			Text:          nullableString(answer.Text),
			Evaluation:    evaluationResponse(answer.Evaluation),
			Transcription: nullable.NewNullNullable[contract.CandidateVoiceTranscription](),
		}
		if answer.Transcription != nil {
			response.Transcription = nullable.NewNullableWithValue(contract.CandidateVoiceTranscription{
				Status: contract.VoiceTranscriptionStatus(answer.Transcription.Status),
				Text:   nullableString(answer.Transcription.Text),
			})
		}
		answers = append(answers, response)
	}
	return contract.CandidateInterviewDetail{
		Report: reportResponse(value.Report),
		Id:     value.ID, Candidate: profileResponse(value.Candidate), Vacancy: vacancyResponse(value.Vacancy),
		StartedAt: value.StartedAt, CompletedAt: value.CompletedAt,
		DurationSeconds: value.DurationSeconds, Answers: answers,
	}
}

func profileResponse(value service.Profile) contract.CandidateProfile {
	return contract.CandidateProfile{
		TelegramUserId: value.TelegramUserID, Username: nullableString(value.Username),
		FirstName: value.FirstName, LastName: nullableString(value.LastName),
		IsPremium: value.IsPremium, LanguageCode: nullableString(value.LanguageCode),
		FirstSeenAt: value.FirstSeenAt, LastSeenAt: value.LastSeenAt,
	}
}

func vacancyResponse(value service.Vacancy) contract.CandidateVacancy {
	return contract.CandidateVacancy{Id: value.ID, Title: value.Title}
}

func nullableString(value *string) nullable.Nullable[string] {
	if value == nil {
		return nullable.NewNullNullable[string]()
	}
	return nullable.NewNullableWithValue(*value)
}

func (s *Server) RetryAnswerEvaluation(ctx context.Context, request contract.RetryAnswerEvaluationRequestObject) (contract.RetryAnswerEvaluationResponseObject, error) {
	version := 0
	if request.Body != nil {
		version = request.Body.Version
	}
	next, err := s.candidates.RetryEvaluation(ctx, principalFromContext(ctx).HRID, request.SessionId, request.AnswerId, version)
	if err != nil {
		kind, value := fault(ctx, err)
		switch kind {
		case domain.FaultNotFound:
			return contract.RetryAnswerEvaluation404JSONResponse{NotFoundJSONResponse: contract.NotFoundJSONResponse(value)}, nil
		case domain.FaultConflict:
			return contract.RetryAnswerEvaluation409JSONResponse{ConflictJSONResponse: contract.ConflictJSONResponse(value)}, nil
		case domain.FaultValidation:
			return contract.RetryAnswerEvaluation422JSONResponse{ValidationErrorJSONResponse: contract.ValidationErrorJSONResponse(value)}, nil
		}
		return nil, err
	}
	return contract.RetryAnswerEvaluation202JSONResponse{Version: next}, nil
}

func evaluationResponse(value service.Evaluation) contract.AnswerEvaluation {
	status := value.Status
	if status == "" {
		status = "skipped"
	}
	response := contract.AnswerEvaluation{
		Attempts: &value.Attempts, ErrorCode: nullable.NewNullNullable[string](),
		Status:  contract.AnswerEvaluationStatus(status),
		Version: nullable.NewNullNullable[int](), ResultVersion: nullable.NewNullNullable[int](),
		Result: nullable.NewNullNullable[contract.AnswerEvaluationResult](),
		Reason: nullable.NewNullNullable[contract.AnswerEvaluationReason](),
	}
	if value.ErrorCode != nil {
		response.ErrorCode.Set(*value.ErrorCode)
	}
	if value.Version != nil {
		response.Version.Set(*value.Version)
	}
	if value.ResultVersion != nil {
		response.ResultVersion.Set(*value.ResultVersion)
	}
	if value.Reason != nil {
		response.Reason.Set(contract.AnswerEvaluationReason(*value.Reason))
	}
	if value.Result != nil {
		checks := make([]contract.AnswerEvaluationCheck, 0, len(value.Result.Checks))
		for _, check := range value.Result.Checks {
			checks = append(checks, contract.AnswerEvaluationCheck{
				Criterion: check.Criterion, Type: contract.AnswerEvaluationCheckType(check.Type),
				Status: contract.AnswerEvaluationCheckStatus(check.Status), Evidence: check.Evidence, Explanation: check.Explanation,
			})
		}
		response.Result.Set(contract.AnswerEvaluationResult{Summary: value.Result.Summary, Checks: checks, FollowUpQuestions: value.Result.FollowUpQuestions})
	}
	return response
}

func (s *Server) RebuildCandidateReport(ctx context.Context, request contract.RebuildCandidateReportRequestObject) (contract.RebuildCandidateReportResponseObject, error) {
	version := -1
	if request.Body != nil {
		version = request.Body.Version
	}
	next, err := s.candidates.RebuildReport(ctx, principalFromContext(ctx).HRID, request.SessionId, version)
	if err != nil {
		kind, value := fault(ctx, err)
		switch kind {
		case domain.FaultNotFound:
			return contract.RebuildCandidateReport404JSONResponse{NotFoundJSONResponse: contract.NotFoundJSONResponse(value)}, nil
		case domain.FaultConflict:
			return contract.RebuildCandidateReport409JSONResponse{ConflictJSONResponse: contract.ConflictJSONResponse(value)}, nil
		case domain.FaultValidation:
			return contract.RebuildCandidateReport422JSONResponse{ValidationErrorJSONResponse: contract.ValidationErrorJSONResponse(value)}, nil
		}
		return nil, err
	}
	return contract.RebuildCandidateReport202JSONResponse{Version: next}, nil
}
func reportResponse(v candidatereport.View) contract.CandidateReport {
	if v.Status == "" {
		v.Status = "not_started"
	}
	result := contract.CandidateReport{Status: contract.CandidateReportStatus(v.Status), Version: v.Version,
		Result: nullable.NewNullNullable[contract.CandidateReportResult](), ResultVersion: nullable.NewNullNullable[int](),
		OverallScore: nullable.NewNullNullable[float32](), ScoredCriteria: v.ScoredCriteria, TotalCriteria: v.TotalCriteria,
		ErrorCode: nullableString(v.ErrorCode), Model: nullableString(v.Model), PromptVersion: nullableString(v.PromptVersion), SchemaVersion: nullableString(v.SchemaVersion)}
	if v.Result != nil {
		result.Result.Set(*v.Result)
	}
	if v.ResultVersion != nil {
		result.ResultVersion.Set(*v.ResultVersion)
	}
	if v.OverallScore != nil {
		result.OverallScore.Set(float32(*v.OverallScore))
	}
	return result
}
