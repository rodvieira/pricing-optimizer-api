package repository_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/rodvieira/pricing-optimizer-api/internal/adapter/repository"
	"github.com/rodvieira/pricing-optimizer-api/internal/domain"
)

// migrationsDir is db/migrations relative to this test file, so applying
// migrations here exercises the exact same SQL `make migrate-up` runs in
// development, rather than a hand-rolled duplicate of the schema.
const migrationsDir = "../../../db/migrations"

// newTestPostgres starts a real Postgres container, applies every migration
// in migrationsDir via goose, and returns a pool connected to it: this
// package's integration tests exercise the actual schema and SQL the
// generated queries run against, matching the project's own "real
// dependency over mocks" testing philosophy (already applied to the
// chromedp scraper and the Redis rate limiter).
func newTestPostgres(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping Postgres container test in short mode: spins up a real container")
	}

	ctx := context.Background()
	container, err := tcpostgres.Run(ctx, "postgres:15",
		tcpostgres.WithDatabase("pricing"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		tcpostgres.BasicWaitStrategies(),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, container.Terminate(context.Background()))
	})

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	db, err := sql.Open("pgx", connStr)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, goose.SetDialect("postgres"))
	require.NoError(t, goose.Up(db, migrationsDir))

	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool
}

func fixturePostgresGeneration() domain.Generation {
	return domain.Generation{
		ID:        "b6f1c6b2-6b8a-4b1a-9f1a-1c2c3c4c5c6c",
		SourceURL: "https://example.com",
		SiteProfile: domain.SiteProfile{
			URL:        "https://example.com",
			Title:      "Acme Analytics",
			SourceType: domain.SourceTypeStatic,
			AnalyzedAt: time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC),
		},
		Status: domain.GenerationStatusCompleted,
		Variations: []domain.Variation{
			{
				ID:       "a1a1a1a1-1111-1111-1111-111111111111",
				Strategy: domain.StrategyAnchor,
				Headline: "Simple, transparent pricing",
				Tiers: []domain.PricingTier{
					{
						Name:     "Pro",
						Price:    domain.Price{AmountMinorUnits: 2900, Currency: "USD", Interval: domain.IntervalMonthly},
						Features: []string{"Feature A"},
					},
				},
			},
		},
		CreatedAt: time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC),
	}
}

func TestPostgresGenerationRepo_SaveAndGet(t *testing.T) {
	pool := newTestPostgres(t)
	repo := repository.NewPostgresGenerationRepo(pool)
	ctx := context.Background()

	want := fixturePostgresGeneration()
	require.NoError(t, repo.Save(ctx, want))

	got, err := repo.Get(ctx, want.ID)
	require.NoError(t, err)
	assert.Equal(t, want.ID, got.ID)
	assert.Equal(t, want.SourceURL, got.SourceURL)
	assert.Equal(t, want.Status, got.Status)
	assert.Equal(t, want.SiteProfile, got.SiteProfile)
	assert.Equal(t, want.Variations, got.Variations)
	assert.WithinDuration(t, want.CreatedAt, got.CreatedAt, time.Second)
}

func TestPostgresGenerationRepo_Save_OverwritesButKeepsCreatedAt(t *testing.T) {
	pool := newTestPostgres(t)
	repo := repository.NewPostgresGenerationRepo(pool)
	ctx := context.Background()

	original := fixturePostgresGeneration()
	original.Status = domain.GenerationStatusStreaming
	original.Variations = nil
	require.NoError(t, repo.Save(ctx, original))

	updated := original
	updated.Status = domain.GenerationStatusCompleted
	updated.Variations = fixturePostgresGeneration().Variations
	// A later save should never be able to move created_at, even if the
	// caller's in-memory copy somehow drifted.
	updated.CreatedAt = original.CreatedAt.Add(time.Hour)
	require.NoError(t, repo.Save(ctx, updated))

	got, err := repo.Get(ctx, original.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.GenerationStatusCompleted, got.Status)
	assert.Equal(t, updated.Variations, got.Variations)
	assert.WithinDuration(t, original.CreatedAt, got.CreatedAt, time.Second,
		"created_at must survive an overwrite unchanged")
}

func TestPostgresGenerationRepo_Get_NotFound(t *testing.T) {
	pool := newTestPostgres(t)
	repo := repository.NewPostgresGenerationRepo(pool)

	_, err := repo.Get(context.Background(), "00000000-0000-0000-0000-000000000000")
	require.ErrorIs(t, err, domain.ErrGenerationNotFound)
}

func TestPostgresGenerationRepo_Get_InvalidID(t *testing.T) {
	pool := newTestPostgres(t)
	repo := repository.NewPostgresGenerationRepo(pool)

	_, err := repo.Get(context.Background(), "not-a-uuid")
	require.Error(t, err)
	assert.NotErrorIs(t, err, domain.ErrGenerationNotFound,
		"a malformed id is a different failure than a well-formed id that doesn't exist")
}
