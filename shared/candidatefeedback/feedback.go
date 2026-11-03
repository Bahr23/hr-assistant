// Package candidatefeedback defines the candidate-facing feedback contract.
package candidatefeedback

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	report "hr-assistant/shared/candidatereport"
)

const (
	Action        = "candidate_feedback"
	Requested     = "interview.candidate.feedback_requested.v1"
	Succeeded     = "interview.candidate.feedback_ready.v1"
	Failed        = "interview.candidate.feedback_failed.v1"
	PromptVersion = "candidate_feedback.v1"
	SchemaVersion = "v1"
)

//go:embed result.v1.json
var Schema json.RawMessage

type Answer struct {
	QuestionNumber int     `json:"question_number"`
	Question       string  `json:"question"`
	Text           *string `json:"text"`
}

// Input deliberately excludes HR guides, numerical scores and hiring recommendations.
type Input struct {
	VacancyTitle       string            `json:"vacancy_title"`
	Answers            []Answer          `json:"answers"`
	Summary            string            `json:"summary"`
	Strengths          []report.Strength `json:"strengths"`
	Concerns           []report.Concern  `json:"concerns"`
	MissingInformation []string          `json:"missing_information"`
	Partial            bool              `json:"partial"`
}

func FromReport(source report.Input, result report.Result) Input {
	input := Input{VacancyTitle: source.VacancyTitle, Summary: result.Summary,
		Strengths: result.Strengths, Concerns: result.Concerns,
		MissingInformation: result.MissingInformation, Partial: result.AnalysisCompleteness.Status == "partial",
		Answers: make([]Answer, 0, len(source.Answers))}
	for _, answer := range source.Answers {
		input.Answers = append(input.Answers, Answer{answer.QuestionNumber, answer.Question, answer.Text})
	}
	return input
}

func (i Input) Valid() bool {
	if len(i.Answers) == 0 || strings.TrimSpace(i.Summary) == "" {
		return false
	}
	for n, answer := range i.Answers {
		if answer.QuestionNumber != n+1 || strings.TrimSpace(answer.Question) == "" {
			return false
		}
	}
	return true
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
}

type Result struct {
	Paragraphs []string `json:"paragraphs"`
}

func (r Result) Text() string { return strings.Join(r.Paragraphs, "\n\n") }

// Parse enforces paragraph boundaries in code, including Unicode line separators.
// Three paragraphs of at most 600 runes also fit in a single Telegram message.
func Parse(raw []byte) (Result, error) {
	invalid := func() (Result, error) { return Result{}, fmt.Errorf("invalid candidate feedback") }
	if len(raw) > 32<<10 {
		return invalid()
	}
	var result Result
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&result) != nil || decoder.Decode(&struct{}{}) != io.EOF || len(result.Paragraphs) < 1 || len(result.Paragraphs) > 3 {
		return invalid()
	}
	for n, paragraph := range result.Paragraphs {
		if strings.ContainsAny(paragraph, "\r\n\u0085\u2028\u2029") || strings.IndexFunc(paragraph, unicode.IsControl) >= 0 {
			return invalid()
		}
		paragraph = strings.TrimSpace(paragraph)
		if paragraph == "" || utf8.RuneCountInString(paragraph) > 600 {
			return invalid()
		}
		result.Paragraphs[n] = paragraph
	}
	return result, nil
}
