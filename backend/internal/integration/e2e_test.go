package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type authResult struct {
	AccessToken string `json:"access_token"`
}

type vacancyResult struct {
	ID           string  `json:"id"`
	Status       string  `json:"status"`
	InterviewURL *string `json:"interview_url"`
}

type questionResult struct {
	ID         string `json:"id"`
	AnswerType string `json:"answer_type"`
	Position   int32  `json:"position"`
}

type questionListResult struct {
	Items []questionResult `json:"items"`
}

func TestBackendFlow(t *testing.T) {
	baseURL := os.Getenv("E2E_BASE_URL")
	if baseURL == "" {
		t.Skip("E2E_BASE_URL is not set")
	}
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	client := &http.Client{Jar: jar, Timeout: 10 * time.Second}
	email := "e2e-" + uuid.NewString() + "@example.com"

	var auth authResult
	status := doJSON(t, client, http.MethodPost, baseURL+"/api/v1/auth/register", "", map[string]any{
		"email":    email,
		"password": "strong-e2e-password",
	}, &auth)
	require.Equal(t, http.StatusCreated, status)
	require.NotEmpty(t, auth.AccessToken)
	status = doJSON(t, client, http.MethodPost, baseURL+"/api/v1/auth/register", "", map[string]any{
		"email": email, "password": "strong-e2e-password",
	}, nil)
	require.Equal(t, http.StatusConflict, status)
	status = doJSON(t, client, http.MethodPost, baseURL+"/api/v1/auth/login", "", map[string]any{
		"email": email, "password": "wrong-password",
	}, nil)
	require.Equal(t, http.StatusUnauthorized, status)
	status = doJSON(t, client, http.MethodPost, baseURL+"/api/v1/auth/login", "", map[string]any{
		"email": email, "password": "strong-e2e-password",
	}, &auth)
	require.Equal(t, http.StatusOK, status)

	status = doJSON(t, client, http.MethodPost, baseURL+"/api/v1/vacancies", auth.AccessToken, map[string]any{}, nil)
	require.Equal(t, http.StatusBadRequest, status)
	status = doJSON(t, client, http.MethodPost, baseURL+"/api/v1/vacancies", auth.AccessToken, map[string]any{
		"title": "Sales manager", "company_name": "Removed field",
		"interview_language": "ru", "welcome_message": "Welcome",
		"questions": []map[string]string{{"text": "Tell us about yourself", "answer_type": "voice"}},
	}, nil)
	require.Equal(t, http.StatusBadRequest, status)

	var vacancy vacancyResult
	status = doJSON(t, client, http.MethodPost, baseURL+"/api/v1/vacancies", auth.AccessToken, map[string]any{
		"title": "Sales manager", "interview_language": "ru", "welcome_message": "Welcome",
		"questions": []map[string]string{
			{"text": "Tell us about yourself", "answer_type": "text"},
			{"text": "Tell us about your sales experience", "answer_type": "voice"},
		},
	}, &vacancy)
	require.Equal(t, http.StatusCreated, status)
	require.Equal(t, "published", vacancy.Status)
	require.NotNil(t, vacancy.InterviewURL)
	firstURL := *vacancy.InterviewURL

	var ordered questionListResult
	status = doJSON(t, client, http.MethodGet, baseURL+"/api/v1/vacancies/"+vacancy.ID+"/questions", auth.AccessToken, nil, &ordered)
	require.Equal(t, http.StatusOK, status)
	first, second := ordered.Items[0], ordered.Items[1]
	require.Equal(t, "text", first.AnswerType)
	require.Equal(t, "voice", second.AnswerType)
	status = doJSON(t, client, http.MethodPut, baseURL+"/api/v1/vacancies/"+vacancy.ID+"/questions/order", auth.AccessToken,
		map[string]any{"question_ids": []string{second.ID, first.ID}}, &ordered)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, second.ID, ordered.Items[0].ID)
	require.EqualValues(t, 0, ordered.Items[0].Position)
	status = doJSON(t, client, http.MethodPut, baseURL+"/api/v1/vacancies/"+vacancy.ID+"/questions/order", auth.AccessToken,
		map[string]any{"question_ids": []string{first.ID}}, nil)
	require.Equal(t, http.StatusUnprocessableEntity, status)
	status = doJSON(t, client, http.MethodGet, baseURL+"/api/v1/vacancies/"+vacancy.ID+"/questions", auth.AccessToken, nil, &ordered)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, second.ID, ordered.Items[0].ID)

	var rotated vacancyResult
	status = doJSON(t, client, http.MethodPost, baseURL+"/api/v1/vacancies/"+vacancy.ID+"/link/rotate", auth.AccessToken, nil, &rotated)
	require.Equal(t, http.StatusOK, status)
	require.NotNil(t, rotated.InterviewURL)
	require.NotEqual(t, firstURL, *rotated.InterviewURL)

	secondJar, err := cookiejar.New(nil)
	require.NoError(t, err)
	secondClient := &http.Client{Jar: secondJar, Timeout: 10 * time.Second}
	var secondAuth authResult
	status = doJSON(t, secondClient, http.MethodPost, baseURL+"/api/v1/auth/register", "", map[string]any{
		"email":    "e2e-" + uuid.NewString() + "@example.com",
		"password": "strong-e2e-password",
	}, &secondAuth)
	require.Equal(t, http.StatusCreated, status)
	status = doJSON(t, secondClient, http.MethodGet, baseURL+"/api/v1/vacancies/"+vacancy.ID, secondAuth.AccessToken, nil, nil)
	require.Equal(t, http.StatusNotFound, status)

	var refreshed authResult
	status = doJSON(t, client, http.MethodPost, baseURL+"/api/v1/auth/refresh", "", nil, &refreshed)
	require.Equal(t, http.StatusOK, status)
	require.NotEmpty(t, refreshed.AccessToken)

	var archived vacancyResult
	status = doJSON(t, client, http.MethodPost, baseURL+"/api/v1/vacancies/"+vacancy.ID+"/archive", refreshed.AccessToken, nil, &archived)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, "archived", archived.Status)
	require.Nil(t, archived.InterviewURL)

	var restored vacancyResult
	status = doJSON(t, client, http.MethodPost, baseURL+"/api/v1/vacancies/"+vacancy.ID+"/restore", refreshed.AccessToken, nil, &restored)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, "published", restored.Status)
	require.NotNil(t, restored.InterviewURL)

	status = doJSON(t, client, http.MethodPost, baseURL+"/api/v1/auth/logout", "", nil, nil)
	require.Equal(t, http.StatusNoContent, status)
	status = doJSON(t, client, http.MethodGet, baseURL+"/api/v1/auth/me", refreshed.AccessToken, nil, nil)
	require.Equal(t, http.StatusUnauthorized, status)
	status = doJSON(t, client, http.MethodPost, baseURL+"/api/v1/auth/refresh", "", nil, nil)
	require.Equal(t, http.StatusUnauthorized, status)
}

func doJSON(t *testing.T, client *http.Client, method, url, accessToken string, body, destination any) int {
	t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, url, reader)
	require.NoError(t, err)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if accessToken != "" {
		request.Header.Set("Authorization", "Bearer "+accessToken)
	}
	response, err := client.Do(request)
	require.NoError(t, err)
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	if destination != nil && len(data) > 0 {
		require.NoError(t, json.Unmarshal(data, destination), fmt.Sprintf("response: %s", data))
	}
	return response.StatusCode
}
