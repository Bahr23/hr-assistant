package transcription

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"hr-assistant/openrouter-worker/internal/app/actions"
)

type Handler struct {
	telegram   *TelegramClient
	openRouter *OpenRouterClient
	tempRoot   string
}

func NewHandler(telegram *TelegramClient, openRouter *OpenRouterClient, tempRoot string) *Handler {
	return &Handler{telegram: telegram, openRouter: openRouter, tempRoot: tempRoot}
}

func (h *Handler) Type() string { return actions.VoiceTranscription }

func (h *Handler) Execute(ctx context.Context, job actions.Job) (actions.Result, *actions.Failure) {
	format, failure := audioFormat(job.MimeType, job.TelegramFileID)
	if failure != nil {
		return actions.Result{}, failure
	}
	tempDirectory, err := os.MkdirTemp(h.tempRoot, "attempt-")
	if err != nil {
		return actions.Result{}, transient("temp_directory_create_failed", "could not create temporary directory")
	}
	if err := os.Chmod(tempDirectory, 0o700); err != nil {
		_ = os.RemoveAll(tempDirectory)
		return actions.Result{}, transient("temp_directory_permissions_failed", "could not secure temporary directory")
	}
	defer os.RemoveAll(tempDirectory)

	path, failure := h.telegram.Download(ctx, job.TelegramFileID, tempDirectory, job.FileSize)
	if failure != nil {
		return actions.Result{}, failure
	}
	return h.openRouter.Transcribe(ctx, path, format, job.Model, job.Language)
}

func PrepareTempRoot(path string) error {
	clean := filepath.Clean(strings.TrimSpace(path))
	if clean == "." || clean == "/" || clean == "" {
		return &os.PathError{Op: "validate", Path: path, Err: os.ErrInvalid}
	}
	if err := os.MkdirAll(clean, 0o700); err != nil {
		return err
	}
	entries, err := os.ReadDir(clean)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(clean, entry.Name())); err != nil {
			return err
		}
	}
	return os.Chmod(clean, 0o700)
}
