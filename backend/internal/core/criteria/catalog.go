package criteria

import (
	_ "embed"
	"encoding/json"
	"github.com/google/uuid"
)

//go:embed templates.v1.json
var catalogJSON []byte

type Template struct {
	ID              uuid.UUID `json:"id"`
	Title           string    `json:"title"`
	EvaluationGuide string    `json:"evaluationGuide"`
}
type Catalog struct {
	Version string     `json:"version"`
	Items   []Template `json:"items"`
}

func Templates() Catalog {
	var c Catalog
	if err := json.Unmarshal(catalogJSON, &c); err != nil {
		panic(err)
	}
	return c
}
func Exists(id uuid.UUID) bool {
	for _, t := range Templates().Items {
		if t.ID == id {
			return true
		}
	}
	return false
}
