package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

func emitEmailEvents(ctx context.Context, client *apiClient, saleID string, saleIndex int, cfg config) (int, error) {
	if !cfg.EmailEvents {
		return 0, nil
	}
	eventTypes := []string{"DELIVERED"}
	if saleIndex%2 == 0 {
		eventTypes = append(eventTypes, "OPENED")
	}
	if saleIndex%5 == 0 {
		eventTypes = append(eventTypes, "CLICKED")
	}
	if saleIndex%17 == 0 {
		eventTypes = []string{"BOUNCED"}
	}

	headers := map[string]string{"X-Webhook-Secret": cfg.EmailWebhookSecret}
	recorded := 0
	for offset, eventType := range eventTypes {
		payload := buildEmailEventPayload(saleID, eventType, saleIndex+offset, time.Now().UTC())
		for attempt := 0; attempt < cfg.IssuedTicketWaitAttempts; attempt++ {
			err := client.requestJSON(ctx, http.MethodPost, "/webhooks/email-events", payload, headers, []int{http.StatusAccepted}, nil)
			if err == nil {
				recorded++
				break
			}
			var httpErr apiError
			if !errors.As(err, &httpErr) || httpErr.status != http.StatusNotFound || attempt == cfg.IssuedTicketWaitAttempts-1 {
				return recorded, err
			}
			if !sleep(ctx, 500*time.Millisecond) {
				return recorded, ctx.Err()
			}
		}
	}
	return recorded, nil
}

func buildEmailEventPayload(saleID string, eventType string, index int, now time.Time) emailEventPayload {
	reason := ""
	if eventType == "BOUNCED" {
		reason = "demo generated bounce"
	}
	return emailEventPayload{
		SaleID:          saleID,
		EventType:       eventType,
		OccurredAt:      now.UTC().Format(time.RFC3339),
		ProviderEventID: fmt.Sprintf("demo-%s-%04d-%s", strings.ToLower(eventType), index, uuid.NewString()),
		Reason:          reason,
	}
}
