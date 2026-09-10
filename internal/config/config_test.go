package config

import "testing"

func TestLoadUsesDedicatedEmailRetryMetricsDefaults(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("EMAIL_RETRY_METRICS_HOST", "")
	t.Setenv("EMAIL_RETRY_METRICS_PORT", "")

	cfg := Load()

	if cfg.EmailRetryMetricsHost != "0.0.0.0" {
		t.Fatalf("expected email retry metrics host default 0.0.0.0, got %q", cfg.EmailRetryMetricsHost)
	}
	if cfg.EmailRetryMetricsPort != "9092" {
		t.Fatalf("expected email retry metrics port default 9092, got %q", cfg.EmailRetryMetricsPort)
	}
}

func TestLoadAcceptsDedicatedEmailRetryMetricsConfig(t *testing.T) {
	t.Setenv("EMAIL_RETRY_METRICS_HOST", "127.0.0.1")
	t.Setenv("EMAIL_RETRY_METRICS_PORT", "9192")

	cfg := Load()

	if cfg.EmailRetryMetricsHost != "127.0.0.1" {
		t.Fatalf("expected configured email retry metrics host, got %q", cfg.EmailRetryMetricsHost)
	}
	if cfg.EmailRetryMetricsPort != "9192" {
		t.Fatalf("expected configured email retry metrics port, got %q", cfg.EmailRetryMetricsPort)
	}
}
