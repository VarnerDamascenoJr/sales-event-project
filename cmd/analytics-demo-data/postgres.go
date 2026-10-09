package main

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

func resetDemoInventory(ctx context.Context, cfg config) error {
	sql := "UPDATE tickets SET available_quantity = CASE " +
		"WHEN id = '" + cfg.GeneralTicketID + "' THEN " + strconv.Itoa(cfg.GeneralStock) + " " +
		"WHEN id = '" + cfg.VIPTicketID + "' THEN " + strconv.Itoa(cfg.VIPStock) + " " +
		"ELSE available_quantity END " +
		"WHERE id IN ('" + cfg.GeneralTicketID + "', '" + cfg.VIPTicketID + "');"
	_, err := runPSQL(ctx, sql)
	return err
}

func fetchUncheckedIssuedTicketIDs(ctx context.Context, cfg config, limit int) ([]string, error) {
	if limit <= 0 {
		return nil, nil
	}
	sql := "SELECT it.id FROM issued_tickets it " +
		"JOIN sales s ON s.id = it.sale_id " +
		"LEFT JOIN ticket_check_ins tci ON tci.issued_ticket_id = it.id " +
		"WHERE s.sales_event_id = '" + cfg.SalesEventID + "' " +
		"AND tci.id IS NULL " +
		"ORDER BY it.created_at ASC " +
		"LIMIT " + strconv.Itoa(limit) + ";"
	output, err := runPSQL(ctx, sql)
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

func runPSQL(ctx context.Context, sql string) (string, error) {
	command := exec.CommandContext(
		ctx,
		"docker", "compose", "exec", "-T", "postgres",
		"psql", "-U", "sales", "-d", "sales_event", "-Atq", "-c", sql,
	)
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("run psql: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}
