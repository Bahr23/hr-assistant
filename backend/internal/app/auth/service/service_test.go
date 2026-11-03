package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"hr-assistant/backend/internal/core/domain"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type memorySessionStore struct {
	mu       sync.Mutex
	users    map[uuid.UUID]domain.User
	byEmail  map[string]uuid.UUID
	sessions map[uuid.UUID]domain.Session
}

func newMemorySessionStore() *memorySessionStore {
	return &memorySessionStore{
		users: map[uuid.UUID]domain.User{}, byEmail: map[string]uuid.UUID{}, sessions: map[uuid.UUID]domain.Session{},
	}
}

func (s *memorySessionStore) Register(_ context.Context, user domain.User, session domain.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.byEmail[user.Email]; exists {
		return domain.NewFault(domain.FaultConflict, "email_already_registered", "email is already registered")
	}
	s.users[user.ID] = user
	s.byEmail[user.Email] = user.ID
	s.sessions[session.ID] = session
	return nil
}

func (s *memorySessionStore) FindHRByEmail(_ context.Context, email string) (domain.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.byEmail[email]
	if !ok {
		return domain.User{}, domain.NewFault(domain.FaultNotFound, "hr_not_found", "HR not found")
	}
	return s.users[id], nil
}

func (s *memorySessionStore) Start(_ context.Context, session domain.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[session.ID] = session
	return nil
}

func (s *memorySessionStore) RotateOrRevoke(_ context.Context, attempt RotationAttempt) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[attempt.SessionID]
	if !ok || session.UserID != attempt.HRID || session.RevokedAt != nil || !session.ExpiresAt.After(attempt.Now) {
		return unauthenticated()
	}
	if session.RefreshTokenHash != attempt.PresentedHash {
		revokedAt := attempt.Now
		session.RevokedAt = &revokedAt
		s.sessions[session.ID] = session
		return unauthenticated()
	}
	session.RefreshTokenHash = attempt.NextHash
	session.ExpiresAt = attempt.NextExpiresAt
	s.sessions[session.ID] = session
	return nil
}

func (s *memorySessionStore) Authenticate(_ context.Context, sessionID, userID uuid.UUID, now time.Time) (domain.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[sessionID]
	if !ok || session.UserID != userID || session.RevokedAt != nil || !session.ExpiresAt.After(now) {
		return domain.User{}, unauthenticated()
	}
	return s.users[userID], nil
}

func (s *memorySessionStore) Revoke(_ context.Context, sessionID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[sessionID]
	if !ok || session.RevokedAt != nil {
		return nil
	}
	now := time.Now().UTC()
	session.RevokedAt = &now
	s.sessions[sessionID] = session
	return nil
}

func newTestService(t *testing.T) *Service {
	t.Helper()
	module, err := New(newMemorySessionStore(), Config{
		Secret: "test-secret-with-at-least-thirty-two-characters", AccessTTL: time.Minute, RefreshTTL: time.Hour,
	})
	require.NoError(t, err)
	return module
}

func registerHR(t *testing.T, module *Service) Grant {
	t.Helper()
	grant, err := module.Open(context.Background(), RegisterHR{
		Email: "hr@example.com", Password: "strong-password",
	})
	require.NoError(t, err)
	return grant
}

func TestLogoutImmediatelyInvalidatesAccessToken(t *testing.T) {
	module := newTestService(t)
	grant := registerHR(t, module)
	_, err := module.Authenticate(context.Background(), grant.AccessToken)
	require.NoError(t, err)

	require.NoError(t, module.Close(context.Background(), grant.RefreshToken))
	_, err = module.Authenticate(context.Background(), grant.AccessToken)

	var fault *domain.Fault
	require.ErrorAs(t, err, &fault)
	require.Equal(t, domain.FaultUnauthenticated, fault.Kind)
}

func TestRefreshReuseRevokesWholeSession(t *testing.T) {
	module := newTestService(t)
	initial := registerHR(t, module)
	rotated, err := module.Open(context.Background(), Refresh{Token: initial.RefreshToken})
	require.NoError(t, err)

	_, err = module.Open(context.Background(), Refresh{Token: initial.RefreshToken})
	require.Error(t, err)
	_, err = module.Authenticate(context.Background(), rotated.AccessToken)
	require.Error(t, err)
	_, err = module.Open(context.Background(), Refresh{Token: rotated.RefreshToken})
	require.Error(t, err)
}

func TestConcurrentRefreshCannotLeaveTwoValidCredentials(t *testing.T) {
	module := newTestService(t)
	initial := registerHR(t, module)

	start := make(chan struct{})
	results := make(chan Grant, 2)
	errors := make(chan error, 2)
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			grant, err := module.Open(context.Background(), Refresh{Token: initial.RefreshToken})
			results <- grant
			errors <- err
		}()
	}
	close(start)
	wait.Wait()
	close(results)
	close(errors)

	successes := 0
	for err := range errors {
		if err == nil {
			successes++
		}
	}
	require.LessOrEqual(t, successes, 1)
	for grant := range results {
		if grant.AccessToken != "" {
			_, err := module.Authenticate(context.Background(), grant.AccessToken)
			require.Error(t, err)
		}
	}
}
