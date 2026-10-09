package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	cfg, err := parseConfig(os.Args[1:], time.Now().UTC())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	result := newGenerator(cfg, os.Stderr).run(ctx)
	payload, err := json.MarshalIndent(map[string]any{
		"config":  redactConfig(cfg),
		"summary": result,
	}, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, "marshal summary:", err)
		os.Exit(1)
	}
	fmt.Println(string(payload))
	if result.Errors > 0 {
		os.Exit(1)
	}
}
