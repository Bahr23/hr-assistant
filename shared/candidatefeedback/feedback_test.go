package candidatefeedback

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestParseEnforcesOneTelegramMessage(t *testing.T) {
	for _, paragraphs := range [][]string{{"Спасибо за интервью."}, {"Сильная сторона.", "Что раскрыть подробнее.", "Совет."}, {strings.Repeat("я", 600), strings.Repeat("😀", 600), "Совет."}} {
		raw, _ := json.Marshal(Result{paragraphs})
		result, err := Parse(raw)
		if err != nil || result.Text() != strings.Join(paragraphs, "\n\n") {
			t.Fatalf("valid paragraphs rejected: %v", err)
		}
	}
	for name, paragraphs := range map[string][]string{
		"empty": {}, "four": {"a", "b", "c", "d"}, "blank": {"   "},
		"too long": {strings.Repeat("я", 601)}, "hidden paragraph": {"Первый\n\nВторой"},
		"CR": {"Первый\rВторой"}, "Unicode paragraph": {"Первый\u2029Второй"},
		"Unicode line": {"Первый\u2028Второй"}, "next line": {"Первый\u0085Второй"},
		"control": {"Первый\x00Второй"},
	} {
		t.Run(name, func(t *testing.T) {
			raw, _ := json.Marshal(Result{paragraphs})
			if _, err := Parse(raw); err == nil {
				t.Fatal("invalid paragraphs accepted")
			}
		})
	}
	for _, raw := range []string{`{}`, `null`, `{"paragraphs":null}`, `{"paragraphs":["ok"],"score":10}`, `{"paragraphs":["ok"]}{}`, `{"paragraphs":[3]}`} {
		if _, err := Parse([]byte(raw)); err == nil {
			t.Fatalf("invalid JSON accepted: %s", raw)
		}
	}
}

func TestSchemaMatchesParagraphBoundaries(t *testing.T) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(Schema))
	if err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("feedback.json", doc); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile("feedback.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"Первый\nВторой", "Первый\rВторой", "Первый\u2029Второй", "Первый\u2028Второй", "Первый\u0085Второй"} {
		if err := schema.Validate(map[string]any{"paragraphs": []any{value}}); err == nil {
			t.Fatalf("schema accepted line separator %q", value)
		}
	}
	if err := schema.Validate(map[string]any{"paragraphs": []any{"Совет."}}); err != nil {
		t.Fatal(err)
	}
}
