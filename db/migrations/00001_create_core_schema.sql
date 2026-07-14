-- +goose Up
-- generations holds one analyze+generate run. SiteProfile and the produced
-- variations are stored as JSONB for the MVP; they can be normalized later.
CREATE TABLE generations (
    id           UUID PRIMARY KEY,
    source_url   TEXT NOT NULL,
    site_profile JSONB,
    status       TEXT NOT NULL,
    variations   JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_generations_created_at ON generations (created_at DESC);

-- llm_calls records every provider call for cost and latency analysis.
CREATE TABLE llm_calls (
    id                UUID PRIMARY KEY,
    generation_id     UUID REFERENCES generations (id) ON DELETE SET NULL,
    provider          TEXT NOT NULL,
    model             TEXT NOT NULL,
    strategy          TEXT,
    prompt_tokens     INTEGER NOT NULL DEFAULT 0,
    completion_tokens INTEGER NOT NULL DEFAULT 0,
    latency_ms        INTEGER NOT NULL DEFAULT 0,
    cost_usd          NUMERIC(12, 6) NOT NULL DEFAULT 0,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_llm_calls_generation_id ON llm_calls (generation_id);
CREATE INDEX idx_llm_calls_created_at ON llm_calls (created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS llm_calls;
DROP TABLE IF EXISTS generations;
