// Package repository implements domain.GenerationRepo.
package repository

import (
	"context"
	"fmt"
	"sync"

	"github.com/rodvieira/pricing-optimizer-api/internal/domain"
)

// InMemoryGenerationRepo is a thread-safe, process-local domain.GenerationRepo.
// It exists to unblock Sprint 5 (HTTP + SSE) ahead of Sprint 6's real
// Postgres/sqlc-backed adapter, which will implement the same port — nothing
// outside cmd/api's wiring needs to change when that swap happens. Data does
// not survive a process restart.
type InMemoryGenerationRepo struct {
	mu   sync.RWMutex
	data map[string]domain.Generation
}

// NewInMemoryGenerationRepo creates an empty repository.
func NewInMemoryGenerationRepo() *InMemoryGenerationRepo {
	return &InMemoryGenerationRepo{data: make(map[string]domain.Generation)}
}

// Save implements domain.GenerationRepo. ctx is accepted to satisfy the port
// (a Postgres-backed implementation needs it for query cancellation); a
// map write cannot block, so there is nothing here for it to cancel.
func (r *InMemoryGenerationRepo) Save(_ context.Context, g domain.Generation) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.data[g.ID] = g
	return nil
}

// Get implements domain.GenerationRepo.
func (r *InMemoryGenerationRepo) Get(_ context.Context, id string) (*domain.Generation, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	g, ok := r.data[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", domain.ErrGenerationNotFound, id)
	}
	return &g, nil
}
