package worker

import (
	"strings"
	"testing"
	"time"
)

func TestEmailRetryBackoffUsesExponentialDelayWithCap(t *testing.T) {
	tests := []struct {
		name     string
		attempts int
		want     time.Duration
	}{
		{name: "first retry", attempts: 1, want: time.Minute},
		{name: "second retry", attempts: 2, want: 2 * time.Minute},
		{name: "third retry", attempts: 3, want: 4 * time.Minute},
		{name: "capped", attempts: 10, want: time.Hour},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := emailRetryBackoff(test.attempts); got != test.want {
				t.Fatalf("expected %s, got %s", test.want, got)
			}
		})
	}
}

func TestEmailRetryStateMovesToDeadLetterAtLimit(t *testing.T) {
	attempts, status, nextRetryAt := emailRetryState(maxEmailRetryAttempts)
	if attempts != maxEmailRetryAttempts {
		t.Fatalf("expected attempts %d, got %d", maxEmailRetryAttempts, attempts)
	}
	if status != emailStatusDeadLetter {
		t.Fatalf("expected status %q, got %q", emailStatusDeadLetter, status)
	}
	if time.Until(nextRetryAt) <= 0 {
		t.Fatal("expected next retry time in the future")
	}
}

func TestTrimEmailErrorLimitsStoredMessage(t *testing.T) {
	value := strings.Repeat("x", 2500)
	got := trimEmailError(value)
	if len(got) != 2000 {
		t.Fatalf("expected trimmed error length 2000, got %d", len(got))
	}
}
