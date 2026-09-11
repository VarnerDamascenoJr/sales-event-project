package messaging

import (
	"context"
	"testing"

	"github.com/varner/sales-event-project/internal/correlation"
)

func TestPublishingHeadersIncludesCorrelationMetadata(t *testing.T) {
	metadata := correlation.Metadata{
		RequestID:     "req-demo",
		CorrelationID: "corr-demo",
		TransactionID: "sale-demo",
	}
	ctx := correlation.ContextWithMetadata(context.Background(), metadata)

	headers := publishingHeaders(ctx)

	if headers[correlation.AMQPRequestIDHeader] != metadata.RequestID {
		t.Fatalf("expected request id header %q, got %q", metadata.RequestID, headers[correlation.AMQPRequestIDHeader])
	}
	if headers[correlation.AMQPCorrelationIDHeader] != metadata.CorrelationID {
		t.Fatalf("expected correlation id header %q, got %q", metadata.CorrelationID, headers[correlation.AMQPCorrelationIDHeader])
	}
	if headers[correlation.AMQPTransactionIDHeader] != metadata.TransactionID {
		t.Fatalf("expected transaction id header %q, got %q", metadata.TransactionID, headers[correlation.AMQPTransactionIDHeader])
	}
}

func TestPublishingHeadersOmitEmptyCorrelationValues(t *testing.T) {
	ctx := correlation.ContextWithMetadata(context.Background(), correlation.Metadata{
		CorrelationID: "corr-demo",
	})

	headers := publishingHeaders(ctx)

	if _, ok := headers[correlation.AMQPRequestIDHeader]; ok {
		t.Fatal("expected empty request id to be omitted")
	}
	if headers[correlation.AMQPCorrelationIDHeader] != "corr-demo" {
		t.Fatalf("expected correlation id header, got %q", headers[correlation.AMQPCorrelationIDHeader])
	}
	if _, ok := headers[correlation.AMQPTransactionIDHeader]; ok {
		t.Fatal("expected empty transaction id to be omitted")
	}
}
