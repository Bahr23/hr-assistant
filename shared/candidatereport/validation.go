package candidatereport

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
)

// ValidationError carries only a developer-owned code and a schema field path.
// Candidate text, criterion names and provider responses never enter diagnostics.
type ValidationError struct {
	code string
	path string
}

func (e ValidationError) Error() string          { return "invalid candidate report: " + e.code + ":" + e.path }
func (e ValidationError) ValidationCode() string { return e.code }
func (e ValidationError) ValidationPath() string { return e.path }

func invalidResult(code, path string) error {
	if !ValidResultPath(path) {
		path = "$"
	}
	return ValidationError{code: code, path: path}
}

var pathSegment = regexp.MustCompile(`^([a-z_]+)(?:\[(0|[1-9][0-9]{0,5})\])?$`)
var pathIndex = regexp.MustCompile(`^(0|[1-9][0-9]{0,5})$`)
var pathSchema = func() map[string]any {
	var schema map[string]any
	if err := json.Unmarshal(Schema, &schema); err != nil {
		panic(err)
	}
	return schema
}()

// ValidResultPath also validates persisted diagnostics before they are used in
// a retry prompt. Only paths through the checked-in schema are accepted.
func ValidResultPath(path string) bool {
	if path == "$" {
		return true
	}
	if len(path) == 0 || len(path) > 160 {
		return false
	}
	schema := pathSchema
	for _, segment := range strings.Split(path, ".") {
		parts := pathSegment.FindStringSubmatch(segment)
		if parts == nil {
			return false
		}
		properties, _ := schema["properties"].(map[string]any)
		schema, _ = properties[parts[1]].(map[string]any)
		if schema == nil {
			return false
		}
		if parts[2] != "" {
			schema, _ = schema["items"].(map[string]any)
			if schema == nil {
				return false
			}
		}
	}
	return true
}

func schemaErrorPath(err error) string {
	var validation *jsonschema.ValidationError
	if !errors.As(err, &validation) {
		return "$"
	}
	for len(validation.Causes) > 0 {
		validation = validation.Causes[0]
	}
	location := append([]string(nil), validation.InstanceLocation...)
	if required, ok := validation.ErrorKind.(*kind.Required); ok && len(required.Missing) > 0 {
		location = append(location, required.Missing[0])
	}
	path := ""
	for _, part := range location {
		if pathIndex.MatchString(part) {
			path += "[" + part + "]"
		} else {
			if path != "" {
				path += "."
			}
			path += part
		}
	}
	if !ValidResultPath(path) {
		return "$"
	}
	return path
}
