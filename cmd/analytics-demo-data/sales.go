package main

import (
	"context"
	"fmt"
	"net/http"
)

func buildSalePayload(index int, cfg config) salePayload {
	ticketID, unitPrice := ticketForSale(index, cfg)
	quantity := 1
	if index%7 == 0 {
		quantity = 2
	}
	customerID := fmt.Sprintf("demo-%s-%04d", cfg.RunID, index)
	return salePayload{
		SalesEventID:  cfg.SalesEventID,
		CustomerID:    customerID,
		CustomerName:  fmt.Sprintf("Demo Buyer %04d", index),
		CustomerEmail: customerID + "@example.com",
		Items: []saleItem{{
			TicketID:  ticketID,
			Quantity:  quantity,
			UnitPrice: unitPrice,
		}},
	}
}

func ticketForSale(index int, cfg config) (string, int) {
	if index%4 == 0 {
		return cfg.VIPTicketID, cfg.VIPPrice
	}
	return cfg.GeneralTicketID, cfg.GeneralPrice
}

func saleAmount(payload salePayload) int {
	total := 0
	for _, item := range payload.Items {
		total += item.Quantity * item.UnitPrice
	}
	return total
}

func expectedTicketCount(payload salePayload) int {
	total := 0
	for _, item := range payload.Items {
		total += item.Quantity
	}
	return total
}

func createSale(ctx context.Context, client *apiClient, payload salePayload) (string, error) {
	var response struct {
		SaleID string `json:"saleId"`
	}
	if err := client.requestJSON(ctx, http.MethodPost, "/sales", payload, nil, []int{http.StatusAccepted}, &response); err != nil {
		return "", err
	}
	return response.SaleID, nil
}
