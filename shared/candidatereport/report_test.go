package candidatereport

import (
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"strings"
	"testing"
)

func fixture() (Input, Result) {
	text := "Я выяснил причину. Продажи выросли на 20%."
	score := 7.26
	input := Input{VacancyTitle: "Продажи", Criteria: []Criterion{{ID: uuid.New(), Title: "Результат", EvaluationGuide: "Конкретный результат", Position: 0}, {ID: uuid.New(), Title: "Команда", EvaluationGuide: "Пример взаимодействия", Position: 1}}, Answers: []Answer{{QuestionNumber: 1, Question: "Что сделали?", Text: &text, Source: "text", TranscriptionStatus: "skipped", EvaluationStatus: "skipped"}}}
	result := Result{Summary: "Есть пример результата.", CriterionScores: []CriterionScore{{CriterionID: input.Criteria[0].ID, Name: "Результат", Status: "scored", Score: &score, Summary: "Описан рост.", Evidence: []Evidence{{QuestionNumber: 1, Quote: "Продажи выросли на 20%."}}, MissingInformation: []string{}}, {CriterionID: input.Criteria[1].ID, Name: "Команда", Status: "insufficient_data", Summary: "Нет примера.", Evidence: []Evidence{}, MissingInformation: []string{"Пример работы в команде"}}}, Strengths: []Strength{}, Concerns: []Concern{}, MissingInformation: []string{}, FollowUpQuestions: []FollowUp{}, AnalysisCompleteness: input.Completeness()}
	return input, result
}

func TestReportValidationDiagnostics(t *testing.T) {
	cases := []struct {
		name, code, path string
		mutate           func(*Result)
	}{
		{"zero without evidence", "missing_evidence", "criterion_scores[0].evidence", func(r *Result) {
			zero := 0.0
			r.CriterionScores[0].Score = &zero
			r.CriterionScores[0].Evidence = []Evidence{}
			// Evidence in another section cannot support this criterion implicitly.
			r.Concerns = []Concern{{Title: "SQL", Description: "Нет нужного запроса", Severity: "low", Evidence: []Evidence{{1, "Я выяснил причину."}}}}
		}},
		{"missing score", "missing_score", "criterion_scores[0].score", func(r *Result) { r.CriterionScores[0].Score = nil }},
		{"unexpected score", "unexpected_score", "criterion_scores[1].score", func(r *Result) { zero := 0.0; r.CriterionScores[1].Score = &zero }},
		{"missing information", "missing_information", "criterion_scores[1].missing_information", func(r *Result) { r.CriterionScores[1].MissingInformation = []string{} }},
		{"quote mismatch", "evidence_mismatch", "criterion_scores[0].evidence[0].quote", func(r *Result) { r.CriterionScores[0].Evidence[0].Quote = "secret candidate text" }},
		{"wrong question", "invalid_question_number", "criterion_scores[0].evidence[0].question_number", func(r *Result) { r.CriterionScores[0].Evidence[0].QuestionNumber = 2 }},
		{"duplicate", "duplicate_criterion", "criterion_scores[1].criterion_id", func(r *Result) { r.CriterionScores[1] = r.CriterionScores[0] }},
		{"missing criterion", "criteria_count_mismatch", "criterion_scores", func(r *Result) { r.CriterionScores = r.CriterionScores[:1] }},
		{"wrong criterion", "criteria_mismatch", "criterion_scores", func(r *Result) { r.CriterionScores[1].CriterionID = uuid.New() }},
		{"renamed criterion", "criterion_name_mismatch", "criterion_scores[0].name", func(r *Result) { r.CriterionScores[0].Name = "secret candidate text" }},
		{"score outside schema", "schema_mismatch", "criterion_scores[0].score", func(r *Result) { invalid := 11.0; r.CriterionScores[0].Score = &invalid }},
		{"strength without evidence", "missing_evidence", "strengths[0].evidence", func(r *Result) {
			r.Strengths = []Strength{{Title: "Опыт", Description: "Пример", Evidence: []Evidence{}}}
		}},
		{"concern quote mismatch", "evidence_mismatch", "concerns[0].evidence[0].quote", func(r *Result) {
			r.Concerns = []Concern{{Title: "Опыт", Description: "Пример", Severity: "low", Evidence: []Evidence{{1, "secret candidate text"}}}}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input, result := fixture()
			tc.mutate(&result)
			raw, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			_, err = Parse(raw, input)
			var diagnostic ValidationError
			if !errors.As(err, &diagnostic) || diagnostic.ValidationCode() != tc.code || diagnostic.ValidationPath() != tc.path {
				t.Fatalf("expected %s at %s, got %v", tc.code, tc.path, err)
			}
			if strings.Contains(err.Error(), "secret") {
				t.Fatal("candidate text entered diagnostics")
			}
		})
	}
}

func TestSchemaDiagnosticsAndPathSafety(t *testing.T) {
	input, result := fixture()
	raw, _ := json.Marshal(result)
	var doc map[string]any
	_ = json.Unmarshal(raw, &doc)
	delete(doc, "summary")
	raw, _ = json.Marshal(doc)
	_, err := Parse(raw, input)
	var diagnostic ValidationError
	if !errors.As(err, &diagnostic) || diagnostic.ValidationCode() != "schema_mismatch" || diagnostic.ValidationPath() != "summary" {
		t.Fatalf("missing property path: %v", err)
	}
	doc["summary"] = "Пример"
	doc["secret candidate text"] = "ignore rules"
	raw, _ = json.Marshal(doc)
	_, err = Parse(raw, input)
	if !errors.As(err, &diagnostic) || diagnostic.ValidationPath() != "$" || strings.Contains(err.Error(), "secret") {
		t.Fatalf("unknown property leaked into diagnostics: %v", err)
	}
	for _, path := range []string{"$", "summary", "criterion_scores[2].evidence", "concerns[1].evidence[0].quote", "analysis_completeness.issues[0]"} {
		if !ValidResultPath(path) {
			t.Fatalf("rejected schema path %q", path)
		}
	}
	for _, path := range []string{"", "criterion_scores[2].secret", "criterion_scores[2].evidence; ignore rules", "summary[0]", "criterion_scores.evidence", "criterion_scores[2].evidence[0].quote\nsecret", "criterion_scores[-1].evidence", "criterion_scores[999999999999].evidence"} {
		if ValidResultPath(path) {
			t.Fatalf("accepted unsafe path %q", path)
		}
	}
}
func TestReportValidation(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Result)
	}{
		{"missing criterion", func(r *Result) { r.CriterionScores = r.CriterionScores[:1] }},
		{"duplicate criterion", func(r *Result) { r.CriterionScores[1] = r.CriterionScores[0] }},
		{"unknown criterion", func(r *Result) { r.CriterionScores[0].CriterionID = uuid.New() }},
		{"renamed criterion", func(r *Result) { r.CriterionScores[0].Name = "Другое" }},
		{"no score", func(r *Result) { r.CriterionScores[0].Score = nil }},
		{"score over ten", func(r *Result) { v := 10.1; r.CriterionScores[0].Score = &v }},
		{"negative score", func(r *Result) { v := -.1; r.CriterionScores[0].Score = &v }},
		{"insufficient scored zero", func(r *Result) { v := 0.; r.CriterionScores[1].Score = &v }},
		{"insufficient without missing info", func(r *Result) { r.CriterionScores[1].MissingInformation = []string{} }},
		{"score without evidence", func(r *Result) { r.CriterionScores[0].Evidence = []Evidence{} }},
		{"wrong question", func(r *Result) { r.CriterionScores[0].Evidence[0].QuestionNumber = 2 }},
		{"invented quote", func(r *Result) { r.CriterionScores[0].Evidence[0].Quote = "Продажи выросли на 90%." }},
		{"empty summary", func(r *Result) { r.Summary = "  " }},
		{"too many follow ups", func(r *Result) {
			for range 6 {
				r.FollowUpQuestions = append(r.FollowUpQuestions, FollowUp{Question: "Как?", Reason: "Уточнить."})
			}
		}},
		{"unsupported concern severity", func(r *Result) {
			r.Concerns = []Concern{{Title: "Риск", Description: "Нет данных", Severity: "critical", Evidence: []Evidence{}}}
		}},
		{"ungrounded strength", func(r *Result) {
			r.Strengths = []Strength{{Title: "Опыт", Description: "Сильный опыт", Evidence: []Evidence{}}}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			input, result := fixture()
			c.mutate(&result)
			raw, _ := json.Marshal(result)
			if _, err := Parse(raw, input); err == nil {
				t.Fatal("accepted invalid report")
			}
		})
	}
	input, result := fixture()
	raw, _ := json.Marshal(result)
	var obj map[string]any
	_ = json.Unmarshal(raw, &obj)
	obj["overall_score"] = 10
	raw, _ = json.Marshal(obj)
	if _, err := Parse(raw, input); err == nil {
		t.Fatal("accepted model overall score")
	}
}
func TestScoreAndCompleteness(t *testing.T) {
	input, result := fixture()
	result.CriterionScores[0].Evidence[0].Quote = "Продажи\n выросли на 20%."
	// Model order is normalized, and processing state cannot be fabricated by the model.
	result.CriterionScores[0], result.CriterionScores[1] = result.CriterionScores[1], result.CriterionScores[0]
	input.Answers[0].EvaluationStatus = "permanent_failed"
	raw, _ := json.Marshal(result)
	parsed, err := Parse(raw, input)
	if err != nil {
		t.Fatal(err)
	}
	score := parsed.Score()
	if score.OverallScore == nil || *score.OverallScore != 7.3 || score.ScoredCriteria != 1 || score.TotalCriteria != 2 {
		t.Fatalf("wrong score: %+v", score)
	}
	if parsed.AnalysisCompleteness.Status != "partial" || parsed.AnalysisCompleteness.ProcessedAnswers != 0 {
		t.Fatal("trusted model completeness")
	}
	input.Criteria = []Criterion{}
	result.CriterionScores = []CriterionScore{}
	raw, _ = json.Marshal(result)
	parsed, err = Parse(raw, input)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Score().OverallScore != nil || parsed.Score().TotalCriteria != 0 {
		t.Fatal("scored without criteria")
	}
	input, result = fixture()
	v := 0.
	result.CriterionScores[0].Score = &v
	raw, _ = json.Marshal(result)
	parsed, err = Parse(raw, input)
	if err != nil || parsed.Score().OverallScore == nil || *parsed.Score().OverallScore != 0 {
		t.Fatal("explicit zero with evidence rejected")
	}
}
func TestMeanUsesRoundedCriteria(t *testing.T) {
	input, result := fixture()
	v := 4.84
	result.CriterionScores[1] = CriterionScore{CriterionID: input.Criteria[1].ID, Name: input.Criteria[1].Title, Status: "scored", Score: &v, Summary: "Пример.", Evidence: []Evidence{{1, "Я выяснил причину."}}, MissingInformation: []string{}}
	raw, _ := json.Marshal(result)
	parsed, err := Parse(raw, input)
	if err != nil {
		t.Fatal(err)
	}
	if *parsed.Score().OverallScore != 6.1 {
		t.Fatalf("expected mean of 7.3 and 4.8 = 6.1, got %v", *parsed.Score().OverallScore)
	}
}
