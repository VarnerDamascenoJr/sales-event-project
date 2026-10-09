package main

import (
	"context"
	"net/http"
	"time"
)

func performCheckIns(ctx context.Context, client *apiClient, cfg config) (int, error) {
	headers := map[string]string{"X-API-Key": cfg.CheckInAPIKey}
	checkedIn := 0
	for attempt := 0; attempt < cfg.IssuedTicketWaitAttempts; attempt++ {
		issuedTicketIDs, err := fetchUncheckedIssuedTicketIDs(ctx, cfg, cfg.CheckIns-checkedIn)
		if err != nil {
			return checkedIn, err
		}
		for _, issuedTicketID := range issuedTicketIDs {
			payload := map[string]string{"ticketCode": "issued_ticket:" + issuedTicketID}
			if err := client.requestJSON(ctx, http.MethodPost, "/sales-events/"+cfg.SalesEventID+"/check-ins", payload, headers, []int{http.StatusCreated}, nil); err != nil {
				return checkedIn, err
			}
			checkedIn++
			if checkedIn >= cfg.CheckIns {
				return checkedIn, nil
			}
		}
		if checkedIn >= cfg.CheckIns {
			return checkedIn, nil
		}
		if !sleep(ctx, 500*time.Millisecond) {
			return checkedIn, ctx.Err()
		}
	}
	return checkedIn, nil
}
