package main

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"time"
)

func buildPaymentPayload(amount int, approved bool) paymentPayload {
	status := "FAILED"
	if approved {
		status = "APPROVED"
	}
	return paymentPayload{
		Amount:   amount,
		Provider: "demo_generator",
		Status:   status,
	}
}

func shouldApprove(index int, rng *rand.Rand, failedPaymentRate float64) bool {
	if failedPaymentRate <= 0 {
		return true
	}
	if failedPaymentRate >= 1 {
		return false
	}
	if index == 1 {
		return true
	}
	return rng.Float64() >= failedPaymentRate
}

func processPayment(ctx context.Context, client *apiClient, saleID string, payload paymentPayload, cfg config) error {
	headers := map[string]string{"X-API-Key": cfg.PaymentAPIKey}
	var lastErr error
	for attempt := 0; attempt < cfg.PaymentWaitAttempts; attempt++ {
		err := client.requestJSON(ctx, http.MethodPost, "/sales/"+saleID+"/payments", payload, headers, []int{http.StatusAccepted}, nil)
		if err == nil {
			return nil
		}
		lastErr = err
		var httpErr apiError
		if !errors.As(err, &httpErr) || (httpErr.status != http.StatusNotFound && httpErr.status != http.StatusConflict) {
			return err
		}
		if !sleep(ctx, 500*time.Millisecond) {
			return ctx.Err()
		}
	}
	return fmt.Errorf("payment was not accepted for sale %s: %w", saleID, lastErr)
}
