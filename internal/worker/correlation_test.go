package worker

import (
	"context"
	"testing"

	"github.com/varner/sales-event-project/internal/correlation"
	"github.com/varner/sales-event-project/internal/events"
)

func TestContextWithEventMetadataUsesAMQPMetadataBeforePayloadMetadata(t *testing.T) {
	ctx := correlation.ContextWithMetadata(context.Background(), correlation.Metadata{
		RequestID:     "req-header",
		CorrelationID: "corr-header",
	})

	ctx = contextWithEventMetadata(ctx, events.CorrelationMetadata{
		RequestID:     "req-payload",
		CorrelationID: "corr-payload",
		TransactionID: "sale-payload",
	})

	metadata, ok := correlation.FromContext(ctx)
	if !ok {
		t.Fatal("expected metadata in context")
	}
	if metadata.RequestID != "req-header" {
		t.Fatalf("expected request id from AMQP headers, got %q", metadata.RequestID)
	}
	if metadata.CorrelationID != "corr-header" {
		t.Fatalf("expected correlation id from AMQP headers, got %q", metadata.CorrelationID)
	}
	if metadata.TransactionID != "sale-payload" {
		t.Fatalf("expected transaction id fallback from payload, got %q", metadata.TransactionID)
	}
}

func TestContextWithEventMetadataFallsBackToPayloadMetadata(t *testing.T) {
	ctx := contextWithEventMetadata(context.Background(), events.CorrelationMetadata{
		RequestID:     "req-payload",
		CorrelationID: "corr-payload",
		TransactionID: "sale-payload",
	})

	metadata, ok := correlation.FromContext(ctx)
	if !ok {
		t.Fatal("expected metadata in context")
	}
	if metadata.RequestID != "req-payload" || metadata.CorrelationID != "corr-payload" || metadata.TransactionID != "sale-payload" {
		t.Fatalf("unexpected metadata: %+v", metadata)
	}
}
