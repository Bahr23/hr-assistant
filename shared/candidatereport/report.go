// Package candidatereport defines the immutable input and validated report wire contract.
package candidatereport

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"hr-assistant/shared/evaluation"
	"math"
	"strings"
)

const (
	Action             = "candidate_report"
	Requested          = "interview.candidate.report_requested.v1"
	Succeeded          = "interview.candidate.reported.v1"
	Failed             = "interview.candidate.report_failed.v1"
	PromptVersion      = "candidate_report.v1"
	SchemaVersion      = "v1"
	MaxInvalidAttempts = 3
)

//go:embed result.v1.json
var Schema json.RawMessage

type Criterion struct {
	ID               uuid.UUID  `json:"id"`
	Title            string     `json:"title"`
	EvaluationGuide  string     `json:"evaluationGuide"`
	Position         int        `json:"position"`
	SourceTemplateID *uuid.UUID `json:"sourceTemplateId"`
}
type Answer struct {
	QuestionNumber      int                `json:"question_number"`
	Question            string             `json:"question"`
	Guide               *string            `json:"evaluation_guide"`
	Text                *string            `json:"text"`
	Source              string             `json:"source"`
	TranscriptionStatus string             `json:"transcription_status"`
	EvaluationStatus    string             `json:"evaluation_status"`
	Evaluation          *evaluation.Result `json:"evaluation"`
}
type Input struct {
	VacancyTitle       string      `json:"vacancy_title"`
	VacancyDescription *string     `json:"vacancy_description"`
	LegacyContext      bool        `json:"legacy_context"`
	Criteria           []Criterion `json:"criteria"`
	Answers            []Answer    `json:"answers"`
}
type Request struct {
	SessionID     uuid.UUID `json:"session_id"`
	Version       int       `json:"version"`
	PromptVersion string    `json:"prompt_version"`
	SchemaVersion string    `json:"schema_version"`
	Input         Input     `json:"input"`
}
type Completion struct {
	SessionID     uuid.UUID       `json:"session_id"`
	Version       int             `json:"version"`
	PromptVersion string          `json:"prompt_version"`
	SchemaVersion string          `json:"schema_version"`
	Model         string          `json:"model"`
	Attempts      int             `json:"attempts"`
	Result        json.RawMessage `json:"result,omitempty"`
	Code          string          `json:"code,omitempty"`
	Usage         json.RawMessage `json:"usage,omitempty"`
}
type Evidence struct {
	QuestionNumber int    `json:"question_number"`
	Quote          string `json:"quote"`
}
type CriterionScore struct {
	CriterionID        uuid.UUID  `json:"criterion_id"`
	Name               string     `json:"name"`
	Status             string     `json:"status"`
	Score              *float64   `json:"score"`
	Summary            string     `json:"summary"`
	Evidence           []Evidence `json:"evidence"`
	MissingInformation []string   `json:"missing_information"`
}
type Strength struct {
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Evidence    []Evidence `json:"evidence"`
}
type Concern struct {
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Severity    string     `json:"severity"`
	Evidence    []Evidence `json:"evidence"`
}
type FollowUp struct {
	Question string `json:"question"`
	Reason   string `json:"reason"`
}
type Completeness struct {
	Status           string   `json:"status"`
	ProcessedAnswers int      `json:"processed_answers"`
	TotalAnswers     int      `json:"total_answers"`
	Issues           []string `json:"issues"`
}
type Result struct {
	Summary              string           `json:"summary"`
	CriterionScores      []CriterionScore `json:"criterion_scores"`
	Strengths            []Strength       `json:"strengths"`
	Concerns             []Concern        `json:"concerns"`
	MissingInformation   []string         `json:"missing_information"`
	FollowUpQuestions    []FollowUp       `json:"follow_up_questions"`
	AnalysisCompleteness Completeness     `json:"analysis_completeness"`
}
type Score struct {
	OverallScore   *float64
	ScoredCriteria int
	TotalCriteria  int
}

func (i Input) Valid() bool {
	if len(i.Criteria) > 20 || len(i.Answers) == 0 || (!i.LegacyContext && strings.TrimSpace(i.VacancyTitle) == "") {
		return false
	}
	ids := map[uuid.UUID]bool{}
	for pos, c := range i.Criteria {
		if c.ID == uuid.Nil || ids[c.ID] || c.Position != pos || strings.TrimSpace(c.Title) == "" || len([]rune(c.Title)) > 120 || strings.TrimSpace(c.EvaluationGuide) == "" || len([]rune(c.EvaluationGuide)) > 2000 {
			return false
		}
		ids[c.ID] = true
	}
	for n, a := range i.Answers {
		if a.QuestionNumber != n+1 || strings.TrimSpace(a.Question) == "" || (a.Source != "text" && a.Source != "transcription") {
			return false
		}
		if a.EvaluationStatus != "succeeded" && a.EvaluationStatus != "permanent_failed" && a.EvaluationStatus != "skipped" {
			return false
		}
		if a.Source == "transcription" && a.TranscriptionStatus != "succeeded" && a.TranscriptionStatus != "permanent_failed" {
			return false
		}
	}
	return true
}

// Completeness is computed from processing state, never from the model's self-assessment.
func (i Input) Completeness() Completeness {
	c := Completeness{Status: "complete", TotalAnswers: len(i.Answers), Issues: []string{}}
	if i.LegacyContext {
		c.Issues = append(c.Issues, "Историческое описание и критерии вакансии не были сохранены.")
	}
	for _, a := range i.Answers {
		complete := true
		if a.Text == nil || strings.TrimSpace(*a.Text) == "" {
			complete = false
			c.Issues = append(c.Issues, fmt.Sprintf("Вопрос %d: исходный ответ недоступен.", a.QuestionNumber))
		}
		if a.EvaluationStatus == "permanent_failed" {
			complete = false
			c.Issues = append(c.Issues, fmt.Sprintf("Вопрос %d: проверка ответа завершилась ошибкой.", a.QuestionNumber))
		}
		if complete {
			c.ProcessedAnswers++
		}
	}
	if len(c.Issues) > 0 {
		c.Status = "partial"
	}
	return c
}

var compiled = func() *jsonschema.Schema {
	d, err := jsonschema.UnmarshalJSON(bytes.NewReader(Schema))
	if err != nil {
		panic(err)
	}
	c := jsonschema.NewCompiler()
	if err = c.AddResource("candidate-report.v1.json", d); err != nil {
		panic(err)
	}
	s, err := c.Compile("candidate-report.v1.json")
	if err != nil {
		panic(err)
	}
	return s
}()

func normalize(s string) string { return strings.Join(strings.Fields(s), " ") }

// Parse rejects unknown/missing criteria, inconsistent scores, and quotes in the wrong answer.
// It canonicalizes criterion order, rounds each score, and replaces completeness with source truth.
func Parse(raw []byte, input Input) (Result, error) {
	if len(raw) > 512<<10 {
		return Result{}, invalidResult("result_too_large", "$")
	}
	if !input.Valid() {
		return Result{}, invalidResult("invalid_input", "$")
	}
	var doc any
	if json.Unmarshal(raw, &doc) != nil {
		return Result{}, invalidResult("invalid_json", "$")
	}
	if err := compiled.Validate(doc); err != nil {
		return Result{}, invalidResult("schema_mismatch", schemaErrorPath(err))
	}
	var result Result
	if json.Unmarshal(raw, &result) != nil {
		return Result{}, invalidResult("schema_mismatch", "$")
	}
	if len(result.CriterionScores) != len(input.Criteria) {
		return Result{}, invalidResult("criteria_count_mismatch", "criterion_scores")
	}
	validateQuotes := func(values []Evidence, path string) error {
		for i, e := range values {
			field := fmt.Sprintf("%s[%d]", path, i)
			n := e.QuestionNumber - 1
			if n < 0 || n >= len(input.Answers) || input.Answers[n].Text == nil {
				return invalidResult("invalid_question_number", field+".question_number")
			}
			if normalize(e.Quote) == "" || !strings.Contains(normalize(*input.Answers[n].Text), normalize(e.Quote)) {
				return invalidResult("evidence_mismatch", field+".quote")
			}
		}
		return nil
	}
	byID := map[uuid.UUID]CriterionScore{}
	positions := map[uuid.UUID]int{}
	for i, s := range result.CriterionScores {
		path := fmt.Sprintf("criterion_scores[%d]", i)
		if _, exists := byID[s.CriterionID]; exists {
			return Result{}, invalidResult("duplicate_criterion", path+".criterion_id")
		}
		if err := validateQuotes(s.Evidence, path+".evidence"); err != nil {
			return Result{}, err
		}
		if s.Status == "scored" {
			if s.Score == nil {
				return Result{}, invalidResult("missing_score", path+".score")
			}
			if len(s.Evidence) == 0 {
				return Result{}, invalidResult("missing_evidence", path+".evidence")
			}
			v := math.Round(*s.Score*10) / 10
			s.Score = &v
		} else if s.Score != nil {
			return Result{}, invalidResult("unexpected_score", path+".score")
		} else if len(s.MissingInformation) == 0 {
			return Result{}, invalidResult("missing_information", path+".missing_information")
		}
		byID[s.CriterionID] = s
		positions[s.CriterionID] = i
	}
	result.CriterionScores = make([]CriterionScore, 0, len(input.Criteria))
	for _, c := range input.Criteria {
		s, ok := byID[c.ID]
		if !ok {
			return Result{}, invalidResult("criteria_mismatch", "criterion_scores")
		}
		if s.Name != c.Title {
			return Result{}, invalidResult("criterion_name_mismatch", fmt.Sprintf("criterion_scores[%d].name", positions[c.ID]))
		}
		result.CriterionScores = append(result.CriterionScores, s)
	}
	for i, s := range result.Strengths {
		path := fmt.Sprintf("strengths[%d].evidence", i)
		if len(s.Evidence) == 0 {
			return Result{}, invalidResult("missing_evidence", path)
		}
		if err := validateQuotes(s.Evidence, path); err != nil {
			return Result{}, err
		}
	}
	for i, c := range result.Concerns {
		if err := validateQuotes(c.Evidence, fmt.Sprintf("concerns[%d].evidence", i)); err != nil {
			return Result{}, err
		}
	}
	result.AnalysisCompleteness = input.Completeness()
	return result, nil
}
func (r Result) Score() Score {
	s := Score{TotalCriteria: len(r.CriterionScores)}
	sum := 0.0
	for _, c := range r.CriterionScores {
		if c.Status == "scored" && c.Score != nil {
			s.ScoredCriteria++
			sum += math.Round(*c.Score * 10)
		}
	}
	if s.ScoredCriteria > 0 {
		v := math.Round(sum/float64(s.ScoredCriteria)) / 10
		s.OverallScore = &v
	}
	return s
}
