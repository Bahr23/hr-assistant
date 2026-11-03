package service

import (
	"context"
	"testing"

	"hr-assistant/backend/internal/core/domain"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type recordingStore struct {
	query ListQuery
	page  Page
}

func (s *recordingStore) List(_ context.Context, _ uuid.UUID, query ListQuery) (Page, error) {
	s.query = query
	return s.page, nil
}

func (s *recordingStore) Get(_ context.Context, _, _ uuid.UUID) (Interview, error) {
	return Interview{}, nil
}

func TestListNormalizesSearchAndDefaultsPagination(t *testing.T) {
	store := &recordingStore{page: Page{Items: []Summary{}}}
	module := New(store)

	_, err := module.List(context.Background(), uuid.New(), ListQuery{Search: "  Ivan  "})

	require.NoError(t, err)
	require.Equal(t, "Ivan", store.query.Search)
	require.Equal(t, int32(20), store.query.Limit)
	require.Equal(t, int32(0), store.query.Offset)
}

func TestListRejectsInvalidPagination(t *testing.T) {
	module := New(&recordingStore{})

	_, err := module.List(context.Background(), uuid.New(), ListQuery{Limit: 101})

	var fault *domain.Fault
	require.ErrorAs(t, err, &fault)
	require.Equal(t, domain.FaultValidation, fault.Kind)
	require.Equal(t, "pagination", fault.Field)
}

func TestListRejectsLongSearch(t *testing.T) {
	module := New(&recordingStore{})
	search := make([]rune, 101)
	for index := range search {
		search[index] = 'я'
	}

	_, err := module.List(context.Background(), uuid.New(), ListQuery{Search: string(search)})

	var fault *domain.Fault
	require.ErrorAs(t, err, &fault)
	require.Equal(t, "search", fault.Field)
}

func (s *recordingStore) RetryEvaluation(_ context.Context, _, _, _ uuid.UUID, version int) (int, error) {
	return version + 1, nil
}

func (s *recordingStore) RebuildReport(_ context.Context, _, _ uuid.UUID, version int) (int, error) {
	return version + 1, nil
}
