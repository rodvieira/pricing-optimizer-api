package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rodvieira/pricing-optimizer-api/internal/adapter/repository/sqlcgen"
	"github.com/rodvieira/pricing-optimizer-api/internal/domain"
)

// PostgresGenerationRepo implements domain.GenerationRepo against a real
// Postgres database, via sqlc-generated queries (db/queries/generations.sql
// against the schema in db/migrations). SiteProfile and Variations are
// stored as JSONB columns; this adapter marshals/unmarshals them, since
// sqlc maps jsonb to a plain []byte rather than a structured Go type.
type PostgresGenerationRepo struct {
	q *sqlcgen.Queries
}

// NewPostgresGenerationRepo creates the repo bound to pool. pool's lifecycle
// (Close) is cmd/api's responsibility, not this adapter's — the same
// division main.go already keeps for every other adapter.
func NewPostgresGenerationRepo(pool *pgxpool.Pool) *PostgresGenerationRepo {
	return &PostgresGenerationRepo{q: sqlcgen.New(pool)}
}

// Save implements domain.GenerationRepo.
func (r *PostgresGenerationRepo) Save(ctx context.Context, g domain.Generation) error {
	id, err := parsePgUUID(g.ID)
	if err != nil {
		return fmt.Errorf("save generation: %w", err)
	}
	siteProfile, err := json.Marshal(g.SiteProfile)
	if err != nil {
		return fmt.Errorf("save generation: marshal site profile: %w", err)
	}
	variations, err := json.Marshal(g.Variations)
	if err != nil {
		return fmt.Errorf("save generation: marshal variations: %w", err)
	}

	err = r.q.UpsertGeneration(ctx, sqlcgen.UpsertGenerationParams{
		ID:          id,
		SourceUrl:   g.SourceURL,
		SiteProfile: siteProfile,
		Status:      string(g.Status),
		Variations:  variations,
		CreatedAt:   pgtype.Timestamptz{Time: g.CreatedAt, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("save generation: %w", err)
	}
	return nil
}

// Get implements domain.GenerationRepo.
func (r *PostgresGenerationRepo) Get(ctx context.Context, id string) (*domain.Generation, error) {
	pgID, err := parsePgUUID(id)
	if err != nil {
		return nil, fmt.Errorf("get generation: %w", err)
	}

	row, err := r.q.GetGeneration(ctx, pgID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: %s", domain.ErrGenerationNotFound, id)
		}
		return nil, fmt.Errorf("get generation: %w", err)
	}

	gen, err := rowToGeneration(row)
	if err != nil {
		return nil, fmt.Errorf("get generation: %w", err)
	}
	return gen, nil
}

// rowToGeneration maps one sqlcgen.Generation row to the domain type,
// unmarshaling its two JSONB columns.
func rowToGeneration(row sqlcgen.Generation) (*domain.Generation, error) {
	var siteProfile domain.SiteProfile
	if len(row.SiteProfile) > 0 {
		if err := json.Unmarshal(row.SiteProfile, &siteProfile); err != nil {
			return nil, fmt.Errorf("unmarshal site profile: %w", err)
		}
	}

	variations := []domain.Variation{}
	if len(row.Variations) > 0 {
		if err := json.Unmarshal(row.Variations, &variations); err != nil {
			return nil, fmt.Errorf("unmarshal variations: %w", err)
		}
	}

	return &domain.Generation{
		ID:          uuid.UUID(row.ID.Bytes).String(),
		SourceURL:   row.SourceUrl,
		SiteProfile: siteProfile,
		Status:      domain.GenerationStatus(row.Status),
		Variations:  variations,
		CreatedAt:   row.CreatedAt.Time,
	}, nil
}

// parsePgUUID parses id (always adapter-stamped via uuid.NewString(), per
// domain.Variation's own doc comment) into the pgtype.UUID form the
// generated queries need.
func parsePgUUID(id string) (pgtype.UUID, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid id %q: %w", id, err)
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}, nil
}
