package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"
	"go.opentelemetry.io/otel/trace"
)

// QueryTracer is a pgx.QueryTracer that opens one span per query executed
// through the pool, mirroring the "one span per unit of work" shape the
// otelhttp instrumentation already uses at the HTTP layer (see
// httpapi.NewRouter). It carries no per-row or per-argument detail: sqlc
// always emits parameterized SQL ($1, $2, ...) rather than inlined values,
// so the query text itself is safe to attach as a span attribute.
type QueryTracer struct {
	tracer trace.Tracer
}

// NewQueryTracer builds the tracer cmd/api wires into
// pgxpool.Config.ConnConfig.Tracer.
func NewQueryTracer() *QueryTracer {
	return &QueryTracer{
		tracer: otel.Tracer("github.com/rodvieira/pricing-optimizer-api/internal/adapter/repository"),
	}
}

// TraceQueryStart implements pgx.QueryTracer.
func (t *QueryTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	ctx, _ = t.tracer.Start(ctx, "postgres.query", trace.WithAttributes(
		semconv.DBSystemNamePostgreSQL,
		semconv.DBQueryText(data.SQL),
	))
	return ctx
}

// TraceQueryEnd implements pgx.QueryTracer. It ends the span TraceQueryStart
// opened, found via the context TraceQueryStart returned (pgx guarantees
// that context flows unchanged into the matching TraceQueryEnd call).
func (t *QueryTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	span := trace.SpanFromContext(ctx)
	defer span.End()

	if data.Err != nil {
		span.RecordError(data.Err)
		span.SetStatus(codes.Error, data.Err.Error())
	}
}
