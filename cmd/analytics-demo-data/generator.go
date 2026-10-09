package main

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
)

type generator struct {
	cfg    config
	client *apiClient
	rng    *rand.Rand
	stderr io.Writer
}

func newGenerator(cfg config, stderr io.Writer) *generator {
	return &generator{
		cfg: cfg,
		client: &apiClient{
			baseURL:        cfg.APIBaseURL,
			httpClient:     &http.Client{Timeout: cfg.RequestTimeout},
			rateLimitSleep: cfg.RateLimitSleep,
		},
		rng:    rand.New(rand.NewSource(cfg.Seed)),
		stderr: stderr,
	}
}

func (g *generator) run(ctx context.Context) summary {
	result := summary{SalesRequested: g.cfg.Sales}

	if g.cfg.ResetDemoInventory {
		if err := resetDemoInventory(ctx, g.cfg); err != nil {
			result.Errors++
			fmt.Fprintf(g.stderr, "reset demo inventory: %v\n", err)
			result.RateLimitRetries = g.client.rateLimitRetryCount
			return result
		}
	}

	for index := 1; index <= g.cfg.Sales; index++ {
		payload := buildSalePayload(index, g.cfg)
		saleID, err := createSale(ctx, g.client, payload)
		if err != nil {
			result.Errors++
			fmt.Fprintf(g.stderr, "sale %d: %v\n", index, err)
			continue
		}
		result.SalesCreated++

		approved := shouldApprove(index, g.rng, g.cfg.FailedPaymentRate)
		if err := processPayment(ctx, g.client, saleID, buildPaymentPayload(saleAmount(payload), approved), g.cfg); err != nil {
			result.Errors++
			fmt.Fprintf(g.stderr, "sale %d: %v\n", index, err)
			continue
		}
		if approved {
			result.PaymentsApproved++
			result.ExpectedIssuedTickets += expectedTicketCount(payload)
			count, err := emitEmailEvents(ctx, g.client, saleID, index, g.cfg)
			if err != nil {
				result.Errors++
				fmt.Fprintf(g.stderr, "sale %d: %v\n", index, err)
				continue
			}
			result.EmailEvents += count
		} else {
			result.PaymentsFailed++
		}
	}

	if g.cfg.CheckIns > 0 {
		count, err := performCheckIns(ctx, g.client, g.cfg)
		if err != nil {
			result.Errors++
			fmt.Fprintf(g.stderr, "check-ins: %v\n", err)
		}
		result.CheckIns = count
	}

	result.RateLimitRetries = g.client.rateLimitRetryCount
	return result
}
