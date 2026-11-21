//go:build integration

package integration_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/varner/sales-event-project/internal/analytics"
)

func TestAnalyticsExportUsesSeededAPIKeyRoles(t *testing.T) {
	tests := []struct {
		name       string
		apiKey     string
		wantRole   string
		wantStatus int
	}{
		{name: "support", apiKey: "dev-support-key", wantRole: "SUPPORT", wantStatus: http.StatusOK},
		{name: "admin", apiKey: "dev-admin-key", wantRole: "ADMIN", wantStatus: http.StatusOK},
		{name: "check-in", apiKey: "dev-check-in-key", wantRole: "CHECK_IN", wantStatus: http.StatusForbidden},
		{name: "payment provider", apiKey: "dev-payment-provider-key", wantRole: "PAYMENT_PROVIDER", wantStatus: http.StatusForbidden},
		{name: "invalid", apiKey: "invalid-integration-test-key", wantStatus: http.StatusUnauthorized},
		{name: "missing", wantStatus: http.StatusUnauthorized},
	}
	client := &http.Client{Timeout: 15 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db := connectDB(t, ctx)
	defer db.Close()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantRole != "" {
				keyHash := sha256.Sum256([]byte(tt.apiKey))
				var role string
				err := db.QueryRow(ctx, `SELECT role FROM api_keys WHERE key_hash = $1 AND active = TRUE AND revoked_at IS NULL`, fmt.Sprintf("%x", keyHash)).Scan(&role)
				if err != nil {
					t.Fatalf("read seeded API key role: %v", err)
				}
				if role != tt.wantRole {
					t.Fatalf("seeded API key role = %q, want %q", role, tt.wantRole)
				}
			}
			req, err := http.NewRequest(http.MethodGet, apiBaseURL()+"/analytics/export?salesEventId="+salesEventID+"&limit=10", nil)
			if err != nil {
				t.Fatalf("create analytics export request: %v", err)
			}
			if tt.apiKey != "" {
				req.Header.Set("X-API-Key", tt.apiKey)
			}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("request analytics export: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tt.wantStatus {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}
			if tt.wantStatus != http.StatusOK {
				return
			}
			var document analytics.Document
			if err := json.NewDecoder(resp.Body).Decode(&document); err != nil {
				t.Fatalf("decode analytics export: %v", err)
			}
			if document.SchemaVersion != analytics.SchemaVersion || document.Source.SalesEventID != salesEventID {
				t.Fatalf("unexpected analytics export schema or event: %q, %q", document.SchemaVersion, document.Source.SalesEventID)
			}
		})
	}
}
