package domain

type FaultKind string

const (
	FaultNotFound        FaultKind = "not_found"
	FaultValidation      FaultKind = "validation"
	FaultConflict        FaultKind = "conflict"
	FaultUnauthenticated FaultKind = "unauthenticated"
)

type Fault struct {
	Kind    FaultKind
	Code    string
	Field   string
	Message string
}

func (f *Fault) Error() string {
	if f.Message != "" {
		return f.Message
	}
	return f.Code
}

func NewFault(kind FaultKind, code, message string) error {
	return &Fault{Kind: kind, Code: code, Message: message}
}

func ValidationFault(code, field, message string) error {
	return &Fault{Kind: FaultValidation, Code: code, Field: field, Message: message}
}
