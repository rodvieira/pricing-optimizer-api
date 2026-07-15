package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/rodvieira/pricing-optimizer-api/test/otelrecorder"
)

func TestQueryTracer_SuccessfulQueryProducesAnUnsetStatusSpan(t *testing.T) {
	// Not t.Parallel(): mutates the process-global TracerProvider.
	recorder := otelrecorder.WithRecordingTracerProvider(t)
	tracer := NewQueryTracer()

	ctx := tracer.TraceQueryStart(context.Background(), nil, pgx.TraceQueryStartData{
		SQL: "SELECT id FROM generations WHERE id = $1",
	})
	tracer.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{})

	spans := recorder.Ended()
	require.Len(t, spans, 1)
	assert.Equal(t, "postgres.query", spans[0].Name())
	assert.Equal(t, sdktrace.Status{Code: codes.Unset}, spans[0].Status())
	otelrecorder.AssertHasAttribute(t, spans[0], "db.system.name", "postgresql")
	otelrecorder.AssertHasAttribute(t, spans[0], "db.query.text", "SELECT id FROM generations WHERE id = $1")
}

func TestQueryTracer_FailedQueryRecordsTheErrorAndSetsSpanStatusToError(t *testing.T) {
	// Not t.Parallel(): mutates the process-global TracerProvider.
	recorder := otelrecorder.WithRecordingTracerProvider(t)
	tracer := NewQueryTracer()
	wantErr := errors.New("connection reset")

	ctx := tracer.TraceQueryStart(context.Background(), nil, pgx.TraceQueryStartData{
		SQL: "UPDATE generations SET status = $1 WHERE id = $2",
	})
	tracer.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{Err: wantErr})

	spans := recorder.Ended()
	require.Len(t, spans, 1)
	assert.Equal(t, sdktrace.Status{Code: codes.Error, Description: "connection reset"}, spans[0].Status())
	require.Len(t, spans[0].Events(), 1, "RecordError must add an exception event")
}
