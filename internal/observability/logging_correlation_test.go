package observability

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/varner/sales-event-project/internal/correlation"
)

func TestCorrelationHandlerAddsLogAttrsFromContext(t *testing.T) {
	capture := &captureHandler{}
	handler := correlationHandler{handler: capture}
	ctx := correlation.ContextWithMetadata(context.Background(), correlation.Metadata{
		RequestID:     "req-123",
		CorrelationID: "corr-456",
		TransactionID: "tx-789",
	})

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "sale accepted", 0)
	if err := handler.Handle(ctx, record); err != nil {
		t.Fatalf("handle log record: %v", err)
	}

	assertCapturedAttr(t, capture.attrs, "request_id", "req-123")
	assertCapturedAttr(t, capture.attrs, "correlation_id", "corr-456")
	assertCapturedAttr(t, capture.attrs, "transaction_id", "tx-789")
}

type captureHandler struct {
	attrs map[string]any
}

func (h *captureHandler) Enabled(context.Context, slog.Level) bool {
	return true
}

func (h *captureHandler) Handle(_ context.Context, record slog.Record) error {
	h.attrs = map[string]any{}
	record.Attrs(func(attr slog.Attr) bool {
		h.attrs[attr.Key] = attr.Value.Any()
		return true
	})
	return nil
}

func (h *captureHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := &captureHandler{attrs: map[string]any{}}
	for _, attr := range attrs {
		next.attrs[attr.Key] = attr.Value.Any()
	}
	return next
}

func (h *captureHandler) WithGroup(string) slog.Handler {
	return h
}

func assertCapturedAttr(t *testing.T, attrs map[string]any, name string, want string) {
	t.Helper()
	if got := attrs[name]; got != want {
		t.Fatalf("expected log attr %s=%q, got %q", name, want, got)
	}
}
