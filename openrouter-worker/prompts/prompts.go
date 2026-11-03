package prompts

import _ "embed"

//go:embed answer_evaluation.v1.md
var AnswerEvaluationV1 string

//go:embed candidate_report.v1.md
var CandidateReportV1 string

//go:embed candidate_feedback.v1.md
var CandidateFeedbackV1 string
