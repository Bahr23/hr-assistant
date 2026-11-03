package structured

import (
	"bytes"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

// RestoreAnswerEvidenceCase resolves case-only mismatches to a unique excerpt
// of the original answer. The caller must still run the strict result validator.
func RestoreAnswerEvidenceCase(raw []byte, answer string) []byte {
	return restoreEvidenceCase(raw, func(document map[string]any) {
		checks, _ := document["checks"].([]any)
		for _, value := range checks {
			check, _ := value.(map[string]any)
			quotes, _ := check["evidence"].([]any)
			for i, value := range quotes {
				if quote, ok := value.(string); ok {
					if original, found := originalQuote(quote, answer); found {
						quotes[i] = original
					}
				}
			}
		}
	})
}

// Report quotes may only be resolved against their referenced answer, never a
// different answer, a question, an evaluation guide or an earlier AI result.
func RestoreReportEvidenceCase(raw []byte, answers map[int]string) []byte {
	return restoreEvidenceCase(raw, func(document map[string]any) {
		for _, field := range []string{"criterion_scores", "strengths", "concerns"} {
			items, _ := document[field].([]any)
			for _, value := range items {
				item, _ := value.(map[string]any)
				quotes, _ := item["evidence"].([]any)
				for _, value := range quotes {
					evidence, _ := value.(map[string]any)
					quote, ok := evidence["quote"].(string)
					number, _ := evidence["question_number"].(json.Number)
					question, err := strconv.Atoi(string(number))
					answer, exists := answers[question]
					if !ok || err != nil || !exists {
						continue
					}
					if original, found := originalQuote(quote, answer); found {
						evidence["quote"] = original
					}
				}
			}
		}
	})
}

func restoreEvidenceCase(raw []byte, restore func(map[string]any)) []byte {
	if len(raw) > 512<<10 {
		return raw
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var document map[string]any
	if decoder.Decode(&document) != nil || document == nil {
		return raw
	}
	// Do not make malformed output valid by dropping trailing JSON or unknown fields.
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return raw
	}
	restore(document)
	result, err := json.Marshal(document)
	if err != nil {
		return raw
	}
	return result
}

func originalQuote(quote, source string) (string, bool) {
	quote = strings.Join(strings.Fields(quote), " ")
	source = strings.Join(strings.Fields(source), " ")
	if quote == "" || utf8.RuneCountInString(quote) > 1000 {
		return "", false
	}
	if strings.Contains(source, quote) {
		return quote, true
	}
	foldedSource, foldedQuote := strings.ToLower(source), strings.ToLower(quote)
	index := strings.Index(foldedSource, foldedQuote)
	if index < 0 || strings.Contains(foldedSource[index+1:], foldedQuote) {
		return "", false
	}
	// Unicode case conversion can change UTF-8 byte widths. Map rune positions
	// back to the source instead of applying folded byte offsets to original text.
	start := utf8.RuneCountInString(foldedSource[:index])
	runes := []rune(source)
	end := start + utf8.RuneCountInString(quote)
	if end > len(runes) {
		return "", false
	}
	original := string(runes[start:end])
	if !strings.EqualFold(original, quote) {
		return "", false
	}
	return original, true
}
