package correlation

import (
	"testing"

	"github.com/rabbitmq/amqp091-go"
)

func TestFromAMQPHeadersExtractsCorrelationMetadata(t *testing.T) {
	metadata := FromAMQPHeaders(amqp091.Table{
		AMQPRequestIDHeader:     " req-demo ",
		AMQPCorrelationIDHeader: []byte("corr-demo"),
		AMQPTransactionIDHeader: "sale-demo",
	})

	if metadata.RequestID != "req-demo" {
		t.Fatalf("expected request id to be extracted, got %q", metadata.RequestID)
	}
	if metadata.CorrelationID != "corr-demo" {
		t.Fatalf("expected correlation id to be extracted, got %q", metadata.CorrelationID)
	}
	if metadata.TransactionID != "sale-demo" {
		t.Fatalf("expected transaction id to be extracted, got %q", metadata.TransactionID)
	}
}

func TestMergeKeepsPrimaryMetadataBeforeFallback(t *testing.T) {
	metadata := Merge(Metadata{
		RequestID:     "req-header",
		CorrelationID: "",
		TransactionID: "sale-header",
	}, Metadata{
		RequestID:     "req-event",
		CorrelationID: "corr-event",
		TransactionID: "sale-event",
	})

	if metadata.RequestID != "req-header" {
		t.Fatalf("expected primary request id, got %q", metadata.RequestID)
	}
	if metadata.CorrelationID != "corr-event" {
		t.Fatalf("expected fallback correlation id, got %q", metadata.CorrelationID)
	}
	if metadata.TransactionID != "sale-header" {
		t.Fatalf("expected primary transaction id, got %q", metadata.TransactionID)
	}
}
