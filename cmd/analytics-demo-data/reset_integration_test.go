//go:build integration

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestDemoInventoryResetIsAtomicAndScoped(t *testing.T) {
	if os.Getenv("SALES_DEMO_RESET_INTEGRATION") != "1" || !strings.HasPrefix(os.Getenv("COMPOSE_PROJECT_NAME"), "demo-reset-test-") {
		t.Skip("requires an explicitly enabled disposable demo-reset-test-* Compose project")
	}
	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(filepath.Clean(filepath.Join(packageDir, "../.."))); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(packageDir); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	otherEvent := uuid.NewString()
	otherTicket := uuid.NewString()
	variables := []string{"event_id=" + defaultSalesEventID, "general_id=" + defaultGeneralTicketID, "vip_id=" + defaultVIPTicketID, "other_event=" + otherEvent, "other_ticket=" + otherTicket}
	query := func(sql string) string {
		t.Helper()
		result, err := runPSQL(ctx, sql, variables...)
		if err != nil {
			t.Fatalf("database setup or verification: %v", err)
		}
		return strings.TrimSpace(result)
	}
	original := query(`SELECT string_agg(id::text || ':' || available_quantity::text, ',' ORDER BY id)
FROM tickets WHERE sales_event_id = :'event_id'::uuid AND id IN (:'general_id'::uuid, :'vip_id'::uuid);`)
	if original == "" || !strings.Contains(original, ",") {
		t.Fatal("both seeded demo tickets are required")
	}
	originalStocks := strings.Split(original, ",")
	generalStock := strings.SplitN(originalStocks[0], ":", 2)[1]
	vipStock := strings.SplitN(originalStocks[1], ":", 2)[1]
	query(`INSERT INTO sales_events (id, name, status, starts_at) VALUES (:'other_event'::uuid, 'reset test', 'PUBLISHED', now());
INSERT INTO tickets (id, sales_event_id, name, price, available_quantity) VALUES (:'other_ticket'::uuid, :'other_event'::uuid, 'unrelated', 100, 17);`)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, err := runPSQL(cleanupCtx, `UPDATE tickets SET sales_event_id = :'event_id'::uuid, available_quantity = CASE WHEN id = :'general_id'::uuid THEN :'old_general'::integer ELSE :'old_vip'::integer END WHERE id IN (:'general_id'::uuid, :'vip_id'::uuid);
DELETE FROM tickets WHERE id = :'other_ticket'::uuid;
DELETE FROM sales_events WHERE id = :'other_event'::uuid;`, append(variables, "old_general="+generalStock, "old_vip="+vipStock)...)
		if err != nil {
			t.Errorf("restore disposable test data: %v", err)
		}
	})

	query(`UPDATE tickets SET available_quantity = 1 WHERE id IN (:'general_id'::uuid, :'vip_id'::uuid);`)
	cfg := defaultConfig()
	for run := 0; run < 2; run++ {
		if err := resetDemoInventory(ctx, cfg); err != nil {
			t.Fatalf("reset run %d: %v", run+1, err)
		}
	}
	quantities := query(`SELECT (SELECT available_quantity FROM tickets WHERE id = :'general_id'::uuid)::text || ',' ||
    (SELECT available_quantity FROM tickets WHERE id = :'vip_id'::uuid)::text || ',' ||
    (SELECT available_quantity FROM tickets WHERE id = :'other_ticket'::uuid)::text;`)
	if quantities != "1000,500,17" {
		t.Fatalf("unexpected stock after repeated resets: %s", quantities)
	}

	query(`UPDATE tickets SET available_quantity = 37 WHERE id = :'general_id'::uuid;
UPDATE tickets SET sales_event_id = :'other_event'::uuid WHERE id = :'vip_id'::uuid;`)
	if err := resetDemoInventory(ctx, cfg); err == nil {
		t.Fatal("reset succeeded with a ticket outside the seeded event")
	}
	if got := query(`SELECT available_quantity FROM tickets WHERE id = :'general_id'::uuid;`); got != "37" {
		t.Fatalf("failed reset modified the other demo ticket: %s", got)
	}
	if got := query(`SELECT available_quantity FROM tickets WHERE id = :'other_ticket'::uuid;`); got != "17" {
		t.Fatalf("reset modified unrelated inventory: %s", got)
	}

	runID := "test%_" + uuid.NewString()[:8]
	currentCustomer := "demo-" + runID + "-0001"
	otherCustomer := "demo-" + runID + "-other-0001"
	currentSale, otherSale := uuid.NewString(), uuid.NewString()
	currentIssued, otherIssued := uuid.NewString(), uuid.NewString()
	variables = append(variables, "current_customer="+currentCustomer, "other_customer="+otherCustomer,
		"current_sale="+currentSale, "other_sale="+otherSale, "current_issued="+currentIssued, "other_issued="+otherIssued)
	query(`INSERT INTO customers (id, email, name) VALUES
    (:'current_customer', :'current_customer' || '@test.local', 'demo test'),
    (:'other_customer', :'other_customer' || '@test.local', 'demo test');
INSERT INTO sales (id, sales_event_id, customer_id, status) VALUES
    (:'current_sale'::uuid, :'event_id'::uuid, :'current_customer', 'COMPLETED'),
    (:'other_sale'::uuid, :'event_id'::uuid, :'other_customer', 'COMPLETED');
INSERT INTO issued_tickets (id, sale_id, ticket_id, customer_id, sequence, qr_code_payload) VALUES
    (:'current_issued'::uuid, :'current_sale'::uuid, :'general_id'::uuid, :'current_customer', 1, :'current_issued'),
    (:'other_issued'::uuid, :'other_sale'::uuid, :'general_id'::uuid, :'other_customer', 1, :'other_issued');`)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, err := runPSQL(cleanupCtx, `DELETE FROM sales WHERE id IN (:'current_sale'::uuid, :'other_sale'::uuid);
DELETE FROM customers WHERE id IN (:'current_customer', :'other_customer');`, variables...)
		if err != nil {
			t.Errorf("remove disposable run data: %v", err)
		}
	})
	cfg.RunID = runID
	ids, err := fetchUncheckedIssuedTicketIDs(ctx, cfg, 10)
	if err != nil {
		t.Fatalf("fetch tickets for current run: %v", err)
	}
	if len(ids) != 1 || ids[0] != currentIssued {
		t.Fatalf("tickets for current run = %v, want only %s", ids, currentIssued)
	}
}
