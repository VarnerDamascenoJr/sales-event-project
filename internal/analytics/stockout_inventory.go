package analytics

import "sort"

func normalizeTicketInventory(inventory []TicketInventory) []TicketInventory {
	normalized := make([]TicketInventory, 0, len(inventory))
	for _, item := range inventory {
		if item.TicketID == "" {
			continue
		}
		normalized = append(normalized, item)
	}
	sort.SliceStable(normalized, func(i, j int) bool {
		left := normalized[i]
		right := normalized[j]
		if left.SalesEventID != right.SalesEventID {
			return left.SalesEventID < right.SalesEventID
		}
		if left.TicketType != right.TicketType {
			return left.TicketType < right.TicketType
		}
		return left.TicketID < right.TicketID
	})
	return normalized
}
