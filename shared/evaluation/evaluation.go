// Package evaluation is the versioned wire contract shared by the API and AI worker.
package evaluation

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	Action             = "answer_evaluation"
	Requested          = "interview.answer.evaluation_requested.v1"
	Succeeded          = "interview.answer.evaluated.v1"
	Failed             = "interview.answer.evaluation_failed.v1"
	PromptVersion      = "answer_evaluation.v1"
	SchemaVersion      = "v1"
	MaxGuideLength     = 4000
	MaxInvalidAttempts = 3
)

//go:embed result.v1.json
var Schema json.RawMessage

type Input struct {
	Question     string `json:"question"`
	Guide        string `json:"evaluation_guide"`
	Answer       string `json:"answer"`
	AnswerSource string `json:"answer_source"`
}

type Request struct {
	AnswerID      uuid.UUID `json:"answer_id"`
	SessionID     uuid.UUID `json:"session_id"`
	Version       int       `json:"version"`
	PromptVersion string    `json:"prompt_version"`
	SchemaVersion string    `json:"schema_version"`
	Input         Input     `json:"input"`
}

type Completion struct {
	AnswerID      uuid.UUID       `json:"answer_id"`
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
	Summary           string   `json:"summary"`
	Checks            []Check  `json:"checks"`
	FollowUpQuestions []string `json:"follow_up_questions"`
}

type Check struct {
	Criterion   string   `json:"criterion"`
	Type        string   `json:"type"`
	Status      string   `json:"status"`
	Evidence    []string `json:"evidence"`
	Explanation string   `json:"explanation"`
}

var resultSchema = compileSchema()

// ValidationError contains only a fixed reason, never candidate or provider text.
type ValidationError string

func (e ValidationError) Error() string          { return "invalid evaluation result: " + string(e) }
func (e ValidationError) ValidationCode() string { return string(e) }

func compileSchema() *jsonschema.Schema {
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(Schema))
	if err != nil {
		panic(err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("result.v1.json", document); err != nil {
		panic(err)
	}
	schema, err := compiler.Compile("result.v1.json")
	if err != nil {
		panic(err)
	}
	return schema
}

func (i Input) Valid() bool {
	return strings.TrimSpace(i.Question) != "" && strings.TrimSpace(i.Guide) != "" &&
		len([]rune(i.Guide)) <= MaxGuideLength && strings.TrimSpace(i.Answer) != "" &&
		(i.AnswerSource == "text" || i.AnswerSource == "transcription")
}

// Parse validates both the schema and exact quotes. Errors deliberately contain no input data.
func Parse(raw []byte, answer string) (Result, error) {
	if len(raw) > 256<<10 {
		return Result{}, ValidationError("result_too_large")
	}
	var document any
	if err := json.Unmarshal(raw, &document); err != nil {
		return Result{}, ValidationError("invalid_json")
	}
	if err := resultSchema.Validate(document); err != nil {
		return Result{}, ValidationError("schema_mismatch")
	}
	var result Result
	if err := json.Unmarshal(raw, &result); err != nil {
		return Result{}, ValidationError("schema_mismatch")
	}
	if strings.TrimSpace(result.Summary) == "" {
		return Result{}, ValidationError("empty_text")
	}
	normalizedAnswer := normalizeSpaces(answer)
	for _, check := range result.Checks {
		if strings.TrimSpace(check.Criterion) == "" || strings.TrimSpace(check.Explanation) == "" {
			return Result{}, ValidationError("empty_text")
		}
		if check.Status == "found" && len(check.Evidence) == 0 {
			return Result{}, ValidationError("missing_evidence")
		}
		if check.Status == "not_found" && len(check.Evidence) != 0 {
			return Result{}, ValidationError("unexpected_evidence")
		}
		for _, quote := range check.Evidence {
			quote = normalizeSpaces(quote)
			if quote == "" || !strings.Contains(normalizedAnswer, quote) {
				return Result{}, ValidationError("evidence_mismatch")
			}
		}
	}
	for _, question := range result.FollowUpQuestions {
		if strings.TrimSpace(question) == "" {
			return Result{}, ValidationError("empty_text")
		}
	}
	return result, nil
}

func normalizeSpaces(value string) string { return strings.Join(strings.Fields(value), " ") }
