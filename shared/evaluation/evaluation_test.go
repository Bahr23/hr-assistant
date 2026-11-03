package evaluation

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseEnforcesSchemaAndVerbatimEvidence(t *testing.T) {
	valid := `{"summary":"Описаны действия.","checks":[{"criterion":"Уточнение причины","type":"expected","status":"found","evidence":["Я спросил, с чем сравнивает"],"explanation":"Есть уточняющий вопрос."}],"follow_up_questions":[]}`
	answer := "Я\nспросил,\tс чем\u00a0сравнивает клиент."
	if _, err := Parse([]byte(valid), answer); err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"invented quote":          strings.Replace(valid, "Я спросил, с чем сравнивает", "Я дал скидку", 1),
		"changed case":            strings.Replace(valid, "Я спросил", "я спросил", 1),
		"extra field":             strings.Replace(valid, `"summary":`, `"score":10,"summary":`, 1),
		"missing field":           strings.Replace(valid, `,"follow_up_questions":[]`, "", 1),
		"null array":              strings.Replace(valid, `"follow_up_questions":[]`, `"follow_up_questions":null`, 1),
		"invalid type":            strings.Replace(valid, `"expected"`, `"skill"`, 1),
		"invalid status":          strings.Replace(valid, `"found"`, `"passed"`, 1),
		"not found with evidence": strings.Replace(valid, `"found"`, `"not_found"`, 1),
		"found without evidence":  strings.Replace(valid, `["Я спросил, с чем сравнивает"]`, `[]`, 1),
		"empty quote":             strings.Replace(valid, "Я спросил, с чем сравнивает", " \t ", 1),
		"four follow ups":         strings.Replace(valid, `"follow_up_questions":[]`, `"follow_up_questions":["1","2","3","4"]`, 1),
		"blank summary":           strings.Replace(valid, "Описаны действия.", "  ", 1),
		"trailing JSON":           valid + `{}`,
		"null result":             "null",
		"markdown":                "```json\n" + valid + "\n```",
	}
	var tooMany Result
	_ = json.Unmarshal([]byte(valid), &tooMany)
	for len(tooMany.Checks) <= 20 {
		tooMany.Checks = append(tooMany.Checks, tooMany.Checks[0])
	}
	raw, _ := json.Marshal(tooMany)
	cases["too many checks"] = string(raw)
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse([]byte(raw), answer); err == nil {
				t.Fatal("invalid result accepted")
			}
		})
	}
}

func TestParseAcceptsUnclearAndAbsentEvidence(t *testing.T) {
	for _, status := range []string{"unclear", "not_found"} {
		raw := `{"summary":"Не подтверждено.","checks":[{"criterion":"Результат","type":"unwanted","status":"` + status + `","evidence":[],"explanation":"Не указано."}],"follow_up_questions":[]}`
		if _, err := Parse([]byte(raw), "Ответ."); err != nil {
			t.Fatal(err)
		}
	}
}

func TestParseReportsSafeValidationReasons(t *testing.T) {
	base := `{"summary":"Пример.","checks":[{"criterion":"Признак","type":"expected","status":"found","evidence":["секретный ответ"],"explanation":"Пример."}],"follow_up_questions":[]}`
	cases := []struct{ raw, reason string }{
		{base, "evidence_mismatch"},
		{strings.Replace(base, `"found"`, `"not_found"`, 1), "unexpected_evidence"},
		{strings.Replace(base, `["секретный ответ"]`, `[]`, 1), "missing_evidence"},
		{`{}`, "schema_mismatch"},
		{`not JSON`, "invalid_json"},
	}
	for _, c := range cases {
		_, err := Parse([]byte(c.raw), "Исходный ответ")
		if err != ValidationError(c.reason) || strings.Contains(err.Error(), "секретный") {
			t.Fatalf("unexpected diagnostic: %v", err)
		}
	}
}
