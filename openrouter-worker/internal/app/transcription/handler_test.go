package transcription

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"hr-assistant/openrouter-worker/internal/app/actions"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestHandlerTranscribesVoiceAnswersConcurrentlyAndRemovesFiles(t *testing.T) {
	var active atomic.Int32
	var maximum atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case strings.HasSuffix(request.URL.Path, "/getFile"):
			fileID := request.URL.Query().Get("file_id")
			writeJSON(t, writer, map[string]any{
				"ok": true, "result": map[string]any{"file_path": "voices/" + fileID + ".ogg", "file_size": 7},
			})
		case strings.Contains(request.URL.Path, "/file/bot"):
			_, _ = io.WriteString(writer, strings.TrimSuffix(path.Base(request.URL.Path), ".ogg"))
		case request.URL.Path == "/api/v1/audio/transcriptions":
			current := active.Add(1)
			defer active.Add(-1)
			for current > maximum.Load() && !maximum.CompareAndSwap(maximum.Load(), current) {
			}
			time.Sleep(80 * time.Millisecond)
			var payload struct {
				InputAudio struct {
					Data   string `json:"data"`
					Format string `json:"format"`
				} `json:"input_audio"`
			}
			require.NoError(t, json.NewDecoder(request.Body).Decode(&payload))
			audio, err := base64.StdEncoding.DecodeString(payload.InputAudio.Data)
			require.NoError(t, err)
			require.Equal(t, "ogg", payload.InputAudio.Format)
			writer.Header().Set("X-Generation-Id", "generation-"+string(audio))
			writeJSON(t, writer, map[string]any{
				"text": "text-" + string(audio), "model": "test-model",
				"usage": map[string]any{"total_tokens": 3, "cost": 0.001},
			})
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(server.Close)

	tempRoot := t.TempDir()
	httpClient := &http.Client{Timeout: 5 * time.Second}
	handler := NewHandler(
		NewTelegramClient(httpClient, server.URL, "TOKEN"),
		NewOpenRouterClient(httpClient, server.URL+"/api/v1", "KEY"),
		tempRoot,
	)
	var group sync.WaitGroup
	results := make(chan actions.Result, 3)
	for index := 1; index <= 3; index++ {
		group.Add(1)
		go func(fileID string) {
			defer group.Done()
			result, failure := handler.Execute(context.Background(), actions.Job{
				ID: uuid.New(), AnswerID: uuid.New(), TelegramFileID: fileID,
				MimeType: "audio/ogg", Model: "test-model", Language: "ru",
			})
			require.Nil(t, failure)
			results <- result
		}(fmt.Sprintf("voice-%d", index))
	}
	group.Wait()
	close(results)
	require.GreaterOrEqual(t, maximum.Load(), int32(2))
	texts := make(map[string]bool)
	for result := range results {
		texts[result.Text] = true
		require.NotEmpty(t, result.GenerationID)
	}
	require.Len(t, texts, 3)
	entries, err := os.ReadDir(tempRoot)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestHandlerClassifiesFailuresAndAlwaysRemovesAttemptDirectory(t *testing.T) {
	tests := []struct {
		name       string
		getStatus  int
		openStatus int
		permanent  bool
		code       string
	}{
		{name: "invalid Telegram file", getStatus: http.StatusBadRequest, permanent: true, code: "telegram_file_unavailable"},
		{name: "temporary OpenRouter failure", getStatus: http.StatusOK, openStatus: http.StatusInternalServerError, permanent: false, code: "openrouter_unavailable"},
		{name: "unsupported audio", getStatus: http.StatusOK, openStatus: http.StatusUnprocessableEntity, permanent: true, code: "unsupported_audio"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				switch {
				case strings.HasSuffix(request.URL.Path, "/getFile"):
					writer.WriteHeader(test.getStatus)
					writeJSON(t, writer, map[string]any{
						"ok":         test.getStatus == http.StatusOK,
						"error_code": test.getStatus,
						"result":     map[string]any{"file_path": "voice.ogg", "file_size": 5},
					})
				case strings.Contains(request.URL.Path, "/file/bot"):
					_, _ = io.WriteString(writer, "audio")
				case request.URL.Path == "/api/v1/audio/transcriptions":
					writer.WriteHeader(test.openStatus)
					writeJSON(t, writer, map[string]any{"error": "not logged by worker"})
				default:
					http.NotFound(writer, request)
				}
			}))
			defer server.Close()
			tempRoot := t.TempDir()
			handler := NewHandler(
				NewTelegramClient(server.Client(), server.URL, "TOKEN"),
				NewOpenRouterClient(server.Client(), server.URL+"/api/v1", "KEY"),
				tempRoot,
			)
			_, failure := handler.Execute(context.Background(), actions.Job{
				TelegramFileID: "voice-id", MimeType: "audio/ogg", Model: "test-model",
			})
			require.NotNil(t, failure)
			require.Equal(t, test.permanent, failure.Permanent)
			require.Equal(t, test.code, failure.Code)
			entries, err := os.ReadDir(tempRoot)
			require.NoError(t, err)
			require.Empty(t, entries)
		})
	}
}

func writeJSON(t *testing.T, writer http.ResponseWriter, value any) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	require.NoError(t, json.NewEncoder(writer).Encode(value))
}
