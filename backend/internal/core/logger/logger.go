package logger

import (
	"io"
	"log/slog"
)

func New(output io.Writer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(output, nil))
}
