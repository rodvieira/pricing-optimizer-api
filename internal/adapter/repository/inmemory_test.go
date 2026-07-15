package repository

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rodvieira/pricing-optimizer-api/internal/domain"
)

func fixtureGeneration(id string) domain.Generation {
	return domain.Generation{
		ID:        id,
		SourceURL: "https://example.com",
		SiteProfile: domain.SiteProfile{
			URL:   "https://example.com",
			Title: "Acme Analytics",
		},
		Status:    domain.GenerationStatusCompleted,
		CreatedAt: time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC),
	}
}

func TestInMemoryGenerationRepo_SaveAndGet(t *testing.T) {
	t.Parallel()

	repo := NewInMemoryGenerationRepo()
	g := fixtureGeneration("gen-1")

	err := repo.Save(context.Background(), g)
	require.NoError(t, err)

	got, err := repo.Get(context.Background(), "gen-1")
	require.NoError(t, err)
	assert.Equal(t, g, *got)
}

func TestInMemoryGenerationRepo_Get_NotFound(t *testing.T) {
	t.Parallel()

	repo := NewInMemoryGenerationRepo()

	_, err := repo.Get(context.Background(), "does-not-exist")

	require.ErrorIs(t, err, domain.ErrGenerationNotFound)
}

func TestInMemoryGenerationRepo_Save_OverwritesExisting(t *testing.T) {
	t.Parallel()

	repo := NewInMemoryGenerationRepo()
	ctx := context.Background()

	first := fixtureGeneration("gen-1")
	first.Status = domain.GenerationStatusStreaming
	require.NoError(t, repo.Save(ctx, first))

	second := fixtureGeneration("gen-1")
	second.Status = domain.GenerationStatusCompleted
	require.NoError(t, repo.Save(ctx, second))

	got, err := repo.Get(ctx, "gen-1")
	require.NoError(t, err)
	assert.Equal(t, domain.GenerationStatusCompleted, got.Status)
}

func TestInMemoryGenerationRepo_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	repo := NewInMemoryGenerationRepo()
	ctx := context.Background()
	const n = 50

	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			g := fixtureGeneration("gen-" + strconv.Itoa(i))
			require.NoError(t, repo.Save(ctx, g))
			_, err := repo.Get(ctx, g.ID)
			require.NoError(t, err)
		}(i)
	}
	wg.Wait()
}
