package telemetry_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"

	"github.com/rodvieira/pricing-optimizer-api/internal/telemetry"
)

func TestInit_NoEndpointIsANoop(t *testing.T) {
	// Not t.Parallel(): Init mutates the process-global TracerProvider via
	// otel.SetTracerProvider, so these two cases can't run concurrently
	// with each other or with other tests that read/set it.
	shutdown, err := telemetry.Init(context.Background(), telemetry.Config{
		ServiceName: "pricing-optimizer-api", ServiceVersion: "test",
	})
	require.NoError(t, err)
	require.NotNil(t, shutdown)

	tracer := otel.Tracer("test")
	_, span := tracer.Start(context.Background(), "test-span")
	defer span.End()

	assert.False(t, span.SpanContext().IsValid(), "a noop tracer must never produce a real span context")
	assert.NoError(t, shutdown(context.Background()))
}

func TestInit_WithEndpointInstallsARealTracerProvider(t *testing.T) {
	// Constructing the OTLP/HTTP exporter and TracerProvider doesn't dial
	// the collector (that only happens on export), so this doesn't need a
	// real endpoint to be listening.
	shutdown, err := telemetry.Init(context.Background(), telemetry.Config{
		ServiceName: "pricing-optimizer-api", ServiceVersion: "test", Endpoint: "localhost:4318",
	})
	require.NoError(t, err)
	require.NotNil(t, shutdown)
	t.Cleanup(func() { assert.NoError(t, shutdown(context.Background())) })

	tracer := otel.Tracer("test")
	_, span := tracer.Start(context.Background(), "test-span")
	defer span.End()

	assert.True(t, span.SpanContext().IsValid(), "a real TracerProvider must produce a valid span context")
}
