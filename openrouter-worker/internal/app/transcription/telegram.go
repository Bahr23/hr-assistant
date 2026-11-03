package transcription

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"hr-assistant/openrouter-worker/internal/app/actions"
)

const MaxAudioBytes int64 = 20 * 1024 * 1024

type TelegramClient struct {
	httpClient *http.Client
	baseURL    string
	botToken   string
}

func NewTelegramClient(httpClient *http.Client, baseURL, botToken string) *TelegramClient {
	return &TelegramClient{
		httpClient: httpClient,
		baseURL:    strings.TrimRight(baseURL, "/"),
		botToken:   botToken,
	}
}

type telegramResponse struct {
	OK          bool   `json:"ok"`
	ErrorCode   int    `json:"error_code"`
	Description string `json:"description"`
	Result      struct {
		FilePath string `json:"file_path"`
		FileSize int64  `json:"file_size"`
	} `json:"result"`
	Parameters struct {
		RetryAfter int `json:"retry_after"`
	} `json:"parameters"`
}

func (c *TelegramClient) Download(
	ctx context.Context,
	fileID string,
	targetDirectory string,
	expectedSize int64,
) (string, *actions.Failure) {
	if expectedSize > MaxAudioBytes {
		return "", permanent("audio_too_large", "Telegram audio exceeds the 20 MB limit")
	}
	filePath, fileSize, failure := c.getFile(ctx, fileID)
	if failure != nil {
		return "", failure
	}
	if fileSize > MaxAudioBytes {
		return "", permanent("audio_too_large", "Telegram audio exceeds the 20 MB limit")
	}
	target := filepath.Join(targetDirectory, "audio")
	output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", transient("temp_file_create_failed", "could not create temporary audio file")
	}

	downloadURL := c.baseURL + "/file/bot" + c.botToken + "/" + strings.TrimLeft(filePath, "/")
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		_ = output.Close()
		return "", permanent("telegram_request_invalid", "could not build Telegram file request")
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		_ = output.Close()
		return "", transient("telegram_download_unavailable", "Telegram file download is temporarily unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_ = output.Close()
		if response.StatusCode == http.StatusNotFound || response.StatusCode == http.StatusBadRequest {
			return "", permanent("telegram_file_unavailable", "Telegram file no longer exists")
		}
		return "", transient("telegram_download_unavailable", "Telegram file download is temporarily unavailable")
	}
	written, copyErr := io.Copy(output, io.LimitReader(response.Body, MaxAudioBytes+1))
	closeErr := output.Close()
	if copyErr != nil || closeErr != nil {
		return "", transient("telegram_download_interrupted", "Telegram file download was interrupted")
	}
	if written > MaxAudioBytes {
		return "", permanent("audio_too_large", "Telegram audio exceeds the 20 MB limit")
	}
	if written == 0 {
		return "", permanent("empty_audio", "Telegram returned an empty audio file")
	}
	return target, nil
}

func (c *TelegramClient) getFile(
	ctx context.Context,
	fileID string,
) (string, int64, *actions.Failure) {
	values := url.Values{"file_id": []string{fileID}}
	requestURL := c.baseURL + "/bot" + c.botToken + "/getFile?" + values.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return "", 0, permanent("telegram_request_invalid", "could not build Telegram getFile request")
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return "", 0, transient("telegram_get_file_unavailable", "Telegram getFile is temporarily unavailable")
	}
	defer response.Body.Close()
	var body telegramResponse
	decoder := json.NewDecoder(io.LimitReader(response.Body, 1<<20))
	if err := decoder.Decode(&body); err != nil {
		return "", 0, transient("telegram_response_invalid", "Telegram returned an invalid response")
	}
	if response.StatusCode == http.StatusTooManyRequests || body.ErrorCode == http.StatusTooManyRequests {
		failure := transient("telegram_rate_limited", "Telegram rate limit was reached")
		if body.Parameters.RetryAfter > 0 {
			failure.RetryAfter = timeSeconds(body.Parameters.RetryAfter)
		}
		return "", 0, failure
	}
	if !body.OK || response.StatusCode < 200 || response.StatusCode >= 300 {
		code := response.StatusCode
		if body.ErrorCode != 0 {
			code = body.ErrorCode
		}
		if code == http.StatusBadRequest || code == http.StatusNotFound {
			return "", 0, permanent("telegram_file_unavailable", "Telegram file identifier is invalid or expired")
		}
		return "", 0, transient("telegram_get_file_unavailable", "Telegram getFile is temporarily unavailable")
	}
	if strings.TrimSpace(body.Result.FilePath) == "" {
		return "", 0, transient("telegram_response_invalid", "Telegram response did not contain a file path")
	}
	return body.Result.FilePath, body.Result.FileSize, nil
}

func audioFormat(mimeType, telegramFileID string) (string, *actions.Failure) {
	mime := strings.ToLower(strings.TrimSpace(strings.Split(mimeType, ";")[0]))
	formats := map[string]string{
		"audio/ogg": "ogg", "application/ogg": "ogg", "audio/opus": "ogg",
		"audio/mpeg": "mp3", "audio/mp3": "mp3", "audio/mp4": "m4a",
		"audio/x-m4a": "m4a", "audio/webm": "webm", "audio/wav": "wav",
		"audio/x-wav": "wav", "audio/flac": "flac", "audio/aac": "aac",
	}
	if format, ok := formats[mime]; ok {
		return format, nil
	}
	extension := strings.TrimPrefix(strings.ToLower(filepath.Ext(telegramFileID)), ".")
	if extension == "oga" || extension == "opus" {
		extension = "ogg"
	}
	for _, supported := range []string{"ogg", "mp3", "m4a", "webm", "wav", "flac", "aac"} {
		if extension == supported {
			return extension, nil
		}
	}
	if mime == "" {
		return "ogg", nil
	}
	return "", permanent("unsupported_audio", "audio format is not supported")
}

func permanent(code, message string) *actions.Failure {
	return &actions.Failure{Permanent: true, Code: code, Message: message}
}

func transient(code, message string) *actions.Failure {
	return &actions.Failure{Code: code, Message: message}
}

func timeSeconds(value int) time.Duration {
	return time.Duration(value) * time.Second
}
