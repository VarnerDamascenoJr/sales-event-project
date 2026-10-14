package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"log/slog"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/varner/sales-event-project/internal/correlation"
)

func TestConfigureLoggerWritesJSONAtInfoLevel(t *testing.T) {
	tests := []struct {
		name     string
		handlers []slog.Handler
	}{
		{name: "without telemetry"},
		{name: "nil telemetry handler", handlers: []slog.Handler{nil}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout := captureLoggerStdout(t)
			ConfigureLogger("sales-event-api", "test", tt.handlers...)

			slog.Debug("filtered debug record")
			slog.Info("sale accepted", "sale_id", "sale-123")

			records := readConfiguredLogRecords(t, stdout)
			if len(records) != 1 {
				t.Fatalf("expected only the info record on stdout, got %d records", len(records))
			}
			assertConfiguredLogRecord(t, records[0], map[string]any{
				"level":       "INFO",
				"msg":         "sale accepted",
				"service":     "sales-event-api",
				"environment": "test",
				"sale_id":     "sale-123",
			})
		})
	}
}

func TestConfigureLoggerAddsTelemetryAndProtectsSensitiveAttrs(t *testing.T) {
	stdout := captureLoggerStdout(t)
	var telemetryOutput bytes.Buffer
	telemetry := slog.NewJSONHandler(&telemetryOutput, &slog.HandlerOptions{Level: slog.LevelDebug})
	ConfigureLogger("sales-event-worker", "production", telemetry)
	ctx := correlation.ContextWithMetadata(context.Background(), correlation.Metadata{
		RequestID:     "req-123",
		CorrelationID: "corr-456",
		TransactionID: "tx-789",
	})

	slog.Debug("telemetry debug record")
	slog.Default().With("api_key", "bound-secret").InfoContext(ctx, "sale accepted",
		"sale_id", "sale-123",
		"email", "buyer@example.com",
		slog.Group("payload",
			slog.String("customer_email", "customer@example.com"),
			slog.String("customer_id", "customer-123"),
		),
	)

	stdoutRecords := readConfiguredLogRecords(t, stdout)
	if len(stdoutRecords) != 1 {
		t.Fatalf("expected only the info record on stdout, got %d records", len(stdoutRecords))
	}
	telemetryRecords := readConfiguredLogRecords(t, &telemetryOutput)
	if len(telemetryRecords) != 2 {
		t.Fatalf("expected debug and info records in telemetry, got %d records", len(telemetryRecords))
	}
	assertConfiguredLogRecord(t, telemetryRecords[0], map[string]any{
		"level":       "DEBUG",
		"msg":         "telemetry debug record",
		"service":     "sales-event-worker",
		"environment": "production",
	})
	want := map[string]any{
		"level":          "INFO",
		"msg":            "sale accepted",
		"service":        "sales-event-worker",
		"environment":    "production",
		"sale_id":        "sale-123",
		"api_key":        "[redacted]",
		"email":          "[redacted]",
		"request_id":     "req-123",
		"correlation_id": "corr-456",
		"transaction_id": "tx-789",
		"payload": map[string]any{
			"customer_email": "[redacted]",
			"customer_id":    "customer-123",
		},
	}
	assertConfiguredLogRecord(t, stdoutRecords[0], want)
	assertConfiguredLogRecord(t, telemetryRecords[1], want)
}

func captureLoggerStdout(t *testing.T) *os.File {
	t.Helper()
	originalStdout := os.Stdout
	originalLogger := slog.Default()
	originalLogWriter := log.Writer()
	originalLogFlags := log.Flags()
	stdout, err := os.CreateTemp(t.TempDir(), "stdout-*.json")
	if err != nil {
		t.Fatalf("create stdout capture: %v", err)
	}
	os.Stdout = stdout
	t.Cleanup(func() {
		os.Stdout = originalStdout
		slog.SetDefault(originalLogger)
		log.SetOutput(originalLogWriter)
		log.SetFlags(originalLogFlags)
		if err := stdout.Close(); err != nil {
			t.Errorf("close stdout capture: %v", err)
		}
	})
	return stdout
}

func readConfiguredLogRecords(t *testing.T, reader io.Reader) []map[string]any {
	t.Helper()
	if seeker, ok := reader.(io.Seeker); ok {
		if _, err := seeker.Seek(0, io.SeekStart); err != nil {
			t.Fatalf("rewind log output: %v", err)
		}
	}
	decoder := json.NewDecoder(reader)
	var records []map[string]any
	for {
		var record map[string]any
		err := decoder.Decode(&record)
		if errors.Is(err, io.EOF) {
			return records
		}
		if err != nil {
			t.Fatalf("decode log output: %v", err)
		}
		records = append(records, record)
	}
}

func assertConfiguredLogRecord(t *testing.T, got map[string]any, want map[string]any) {
	t.Helper()
	timestamp, ok := got["time"].(string)
	if !ok {
		t.Fatalf("expected a log timestamp, got %v", got["time"])
	}
	if _, err := time.Parse(time.RFC3339Nano, timestamp); err != nil {
		t.Fatalf("invalid log timestamp %q: %v", timestamp, err)
	}
	delete(got, "time")
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected log record %v, got %v", want, got)
	}
}
