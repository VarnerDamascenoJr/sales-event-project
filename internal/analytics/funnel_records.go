package analytics

import (
	"sort"
	"strings"
)

func buildFunnelRecords(events []Event) []funnelRecord {
	recordsByKey := make(map[funnelKey]*funnelRecord)
	saleDefaults := make(map[string]*funnelRecord)

	for _, event := range normalizeEvents(events) {
		if event.SaleID == "" {
			continue
		}
		if event.EventType == "sale.created" {
			record := saleDefaults[event.SaleID]
			if record == nil {
				record = &funnelRecord{SaleID: event.SaleID}
				saleDefaults[event.SaleID] = record
			}
			record.SalesEventID = event.SalesEventID
			record.AcceptedAt = event.OccurredAt
			record.Accepted = true
			record.Pending = saleReachedPending(event.Status)
			continue
		}
		if event.EventType == "payment.processed" {
			applyToSaleRecords(recordsByKey, event.SaleID, func(record *funnelRecord) {
				record.Provider = event.Provider
				if strings.EqualFold(event.Status, "APPROVED") {
					record.Paid = true
				}
			})
			if saleDefaults[event.SaleID] == nil {
				saleDefaults[event.SaleID] = &funnelRecord{SaleID: event.SaleID}
			}
			saleDefaults[event.SaleID].Provider = event.Provider
			if strings.EqualFold(event.Status, "APPROVED") {
				saleDefaults[event.SaleID].Paid = true
			}
			continue
		}

		if event.TicketID == "" {
			continue
		}
		key := funnelKey{SalesEventID: event.SalesEventID, SaleID: event.SaleID, TicketID: event.TicketID}
		record := recordsByKey[key]
		if record == nil {
			defaultRecord := saleDefaults[event.SaleID]
			record = &funnelRecord{
				SalesEventID: event.SalesEventID,
				SaleID:       event.SaleID,
				TicketID:     event.TicketID,
				TicketType:   event.TicketType,
			}
			if defaultRecord != nil {
				record.SalesEventID = firstNonEmpty(record.SalesEventID, defaultRecord.SalesEventID)
				record.AcceptedAt = defaultRecord.AcceptedAt
				record.Accepted = defaultRecord.Accepted
				record.Pending = defaultRecord.Pending
				record.Paid = defaultRecord.Paid
				record.Provider = defaultRecord.Provider
			}
			recordsByKey[key] = record
		}
		record.TicketType = firstNonEmpty(record.TicketType, event.TicketType)
		record.SalesEventID = firstNonEmpty(record.SalesEventID, event.SalesEventID)

		switch event.EventType {
		case "sale.item.created":
			record.Accepted = true
			record.Pending = record.Pending || saleReachedPending(event.Status)
			if record.AcceptedAt.IsZero() || event.OccurredAt.Before(record.AcceptedAt) {
				record.AcceptedAt = event.OccurredAt
			}
		case "ticket.issued":
			record.TicketIssued = true
		case "checkin.completed":
			record.CheckedIn = true
		}
	}

	records := make([]funnelRecord, 0, len(recordsByKey))
	for _, record := range recordsByKey {
		if record.AcceptedAt.IsZero() {
			continue
		}
		record.Provider = providerOrUnknown(record.Provider)
		records = append(records, *record)
	}
	sort.Slice(records, func(i, j int) bool {
		left := records[i]
		right := records[j]
		if !left.AcceptedAt.Equal(right.AcceptedAt) {
			return left.AcceptedAt.Before(right.AcceptedAt)
		}
		if left.SaleID != right.SaleID {
			return left.SaleID < right.SaleID
		}
		return left.TicketID < right.TicketID
	})
	return records
}

func applyToSaleRecords(records map[funnelKey]*funnelRecord, saleID string, apply func(*funnelRecord)) {
	for key, record := range records {
		if key.SaleID == saleID {
			apply(record)
		}
	}
}

func saleReachedPending(status string) bool {
	switch strings.ToUpper(status) {
	case "PENDING_PAYMENT", "COMPLETED":
		return true
	default:
		return false
	}
}

func providerOrUnknown(provider string) string {
	if provider == "" {
		return "unknown"
	}
	return provider
}
