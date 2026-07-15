package telemetry

import (
	"context"
	"fmt"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

// slogHandler wraps another slog.Handler, adding trace_id/span_id
// attributes from ctx's active span to every record — the real
// OpenTelemetry-backed replacement for the request-id stand-in
// httpapi.writeProblem used until this package existed. Only records
// logged through a context-aware call (ErrorContext, InfoContext, ...)
// carry a span; slog.Error/Info et al. use context.Background() under the
// hood and so never see a trace_id, exactly as before this package existed.
type slogHandler struct {
	next slog.Handler
}

// NewSlogHandler wraps next with the trace_id/span_id behavior described above.
func NewSlogHandler(next slog.Handler) slog.Handler {
	return &slogHandler{next: next}
}

func (h *slogHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *slogHandler) Handle(ctx context.Context, record slog.Record) error {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		record.AddAttrs(
			slog.String("trace_id", sc.TraceID().String()),
			slog.String("span_id", sc.SpanID().String()),
		)
	}
	if err := h.next.Handle(ctx, record); err != nil {
		return fmt.Errorf("telemetry: handle log record: %w", err)
	}
	return nil
}

func (h *slogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &slogHandler{next: h.next.WithAttrs(attrs)}
}

func (h *slogHandler) WithGroup(name string) slog.Handler {
	return &slogHandler{next: h.next.WithGroup(name)}
}
