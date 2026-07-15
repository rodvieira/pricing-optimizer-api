-- name: UpsertGeneration :exec
-- Creates a generation, or overwrites every column but created_at if one
-- already exists at this id — matching domain.GenerationRepo.Save's own
-- "creates or overwrites" contract. created_at is deliberately excluded
-- from the update so a later terminal-state save (e.g. streaming ->
-- completed) doesn't reset when the record was first created.
INSERT INTO generations (id, source_url, site_profile, status, variations, created_at)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (id) DO UPDATE SET
    source_url   = EXCLUDED.source_url,
    site_profile = EXCLUDED.site_profile,
    status       = EXCLUDED.status,
    variations   = EXCLUDED.variations;

-- name: GetGeneration :one
SELECT id, source_url, site_profile, status, variations, created_at
FROM generations
WHERE id = $1;
