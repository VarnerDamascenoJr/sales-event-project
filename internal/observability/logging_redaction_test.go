package observability

import (
	"log/slog"
	"testing"
)

func TestRedactAttrRedactsSensitiveValue(t *testing.T) {
	attr := redactAttr(slog.String("email", "customer@example.com"))

	if got := attr.Value.String(); got != "[redacted]" {
		t.Fatalf("expected sensitive attr to be redacted, got %q", got)
	}
}

func TestRedactAttrRedactsSensitiveValuesInsideGroup(t *testing.T) {
	attr := redactAttr(slog.Group("payload",
		slog.String("customer_email", "customer@example.com"),
		slog.String("sale_id", "sale-123"),
	))

	values := groupAttrValues(attr)
	if got := values["customer_email"]; got != "[redacted]" {
		t.Fatalf("expected nested sensitive attr to be redacted, got %q", got)
	}
	if got := values["sale_id"]; got != "sale-123" {
		t.Fatalf("expected non-sensitive nested attr to be preserved, got %q", got)
	}
}

func groupAttrValues(attr slog.Attr) map[string]string {
	values := map[string]string{}
	for _, groupAttr := range attr.Value.Group() {
		values[groupAttr.Key] = groupAttr.Value.String()
	}
	return values
}
