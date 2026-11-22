package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

func resetDemoInventory(ctx context.Context, cfg config) error {
	if err := validateDemoInventoryReset(cfg); err != nil {
		return err
	}
	const sql = `
WITH selected AS MATERIALIZED (
    SELECT id FROM tickets
    WHERE sales_event_id = :'event_id'::uuid
      AND id IN (:'general_id'::uuid, :'vip_id'::uuid)
    ORDER BY id FOR UPDATE
), updated AS (
    UPDATE tickets SET available_quantity = CASE
        WHEN id = :'general_id'::uuid THEN :'general_stock'::integer
        WHEN id = :'vip_id'::uuid THEN :'vip_stock'::integer
        ELSE available_quantity END
    WHERE id IN (SELECT id FROM selected) AND (SELECT count(*) FROM selected) = 2
    RETURNING id
)
SELECT count(*) FROM updated;`
	output, err := runPSQL(ctx, sql,
		"event_id="+defaultSalesEventID,
		"general_id="+defaultGeneralTicketID,
		"vip_id="+defaultVIPTicketID,
		"general_stock="+strconv.Itoa(cfg.GeneralStock),
		"vip_stock="+strconv.Itoa(cfg.VIPStock),
	)
	if err != nil {
		return err
	}
	if strings.TrimSpace(output) != "2" {
		return errors.New("reset demo inventory: expected both seeded tickets in the demo event")
	}
	return nil
}

func fetchUncheckedIssuedTicketIDs(ctx context.Context, cfg config, limit int) ([]string, error) {
	if limit <= 0 {
		return nil, nil
	}
	const sql = `SELECT it.id FROM issued_tickets it
JOIN sales s ON s.id = it.sale_id
LEFT JOIN ticket_check_ins tci ON tci.issued_ticket_id = it.id
WHERE s.sales_event_id = :'event_id'::uuid AND tci.id IS NULL
  AND left(s.customer_id, length(:'customer_prefix')) = :'customer_prefix'
  AND substring(s.customer_id FROM length(:'customer_prefix') + 1) ~ '^[0-9]+$'
ORDER BY it.created_at ASC LIMIT :'limit'::integer;`
	output, err := runPSQL(ctx, sql,
		"event_id="+cfg.SalesEventID,
		"customer_prefix=demo-"+cfg.RunID+"-",
		"limit="+strconv.Itoa(limit),
	)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(output), "\n")
	ids := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			ids = append(ids, line)
		}
	}
	return ids, nil
}

func runPSQL(ctx context.Context, sql string, variables ...string) (string, error) {
	args := []string{"compose", "exec", "-T", "postgres", "psql", "-X", "-U", "sales", "-d", "sales_event", "-Atq", "-v", "ON_ERROR_STOP=1"}
	for _, variable := range variables {
		args = append(args, "-v", variable)
	}
	command := exec.CommandContext(ctx, "docker", args...)
	command.Stdin = strings.NewReader(sql)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("run psql: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(string(output)), nil
}
