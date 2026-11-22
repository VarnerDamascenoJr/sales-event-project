package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestResetDemoInventoryRequiresSeededIDs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{name: "seeded IDs", args: []string{"--reset-demo-inventory"}},
		{name: "custom event", args: []string{"--reset-demo-inventory", "--sales-event-id", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"}, wantErr: true},
		{name: "custom general ticket", args: []string{"--reset-demo-inventory", "--general-ticket-id", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"}, wantErr: true},
		{name: "custom VIP ticket", args: []string{"--reset-demo-inventory", "--vip-ticket-id", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"}, wantErr: true},
		{name: "custom IDs without reset", args: []string{"--sales-event-id", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"}},
		{name: "stock exceeds database range", args: []string{"--reset-demo-inventory", "--general-stock", "2147483648"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseConfig(tt.args, time.Now())
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseConfig error = %v, want error %v", err, tt.wantErr)
			}
		})
	}
}

func TestResetDemoInventoryRejectsUnseededIDsBeforeRunningDocker(t *testing.T) {
	cfg := defaultConfig()
	cfg.GeneralTicketID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	t.Setenv("PATH", t.TempDir())
	err := resetDemoInventory(context.Background(), cfg)
	if err == nil || !strings.Contains(err.Error(), "only supports the seeded demo event and ticket IDs") {
		t.Fatalf("reset error = %v, want seeded-ID validation error", err)
	}
}

func TestResetDemoInventoryIgnoresPSQLNoticesOnSuccess(t *testing.T) {
	binDir := t.TempDir()
	docker := filepath.Join(binDir, "docker")
	if err := os.WriteFile(docker, []byte("#!/bin/sh\nprintf 'NOTICE: test notice\\n' >&2\nprintf '2\\n'\n"), 0o700); err != nil {
		t.Fatalf("create fake docker: %v", err)
	}
	t.Setenv("PATH", binDir)
	if err := resetDemoInventory(context.Background(), defaultConfig()); err != nil {
		t.Fatalf("reset with successful psql notice: %v", err)
	}
}
