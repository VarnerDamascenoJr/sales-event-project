package outbox

import (
	"strings"
	"testing"
	"time"
)

func TestNextBackoffUsesExponentialDelayWithCap(t *testing.T) {
	tests := []struct {
		name     string
		attempts int
		want     time.Duration
	}{
		{name: "first attempt", attempts: 1, want: time.Minute},
		{name: "second attempt", attempts: 2, want: 2 * time.Minute},
		{name: "third attempt", attempts: 3, want: 4 * time.Minute},
		{name: "capped", attempts: 10, want: time.Hour},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := nextBackoff(test.attempts); got != test.want {
				t.Fatalf("expected %s, got %s", test.want, got)
			}
		})
	}
}

func TestTrimErrorLimitsStoredMessage(t *testing.T) {
	value := strings.Repeat("x", 1200)

	got := trimError(value)

	if len(got) != 1000 {
		t.Fatalf("expected trimmed error length 1000, got %d", len(got))
	}
}

func TestRoutingKeyForEventType(t *testing.T) {
	routingKey, ok := routingKeyForEventType("SALE_COMPLETED")
	if !ok || routingKey != "sale.completed" {
		t.Fatalf("unexpected routing key: %q ok=%v", routingKey, ok)
	}

	if _, ok := routingKeyForEventType("UNKNOWN"); ok {
		t.Fatal("expected unknown event type to have no routing key")
	}
}
