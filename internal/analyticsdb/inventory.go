package analyticsdb

import (
	"context"

	"github.com/varner/sales-event-project/internal/analytics"
)

// ReadTicketInventory reads current ticket availability for the sales event filter.
func ReadTicketInventory(ctx context.Context, db Queryer, filter Filter) ([]analytics.TicketInventory, error) {
	rows, err := db.Query(ctx, ticketInventoryQuery, filter.SalesEventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	inventory := make([]analytics.TicketInventory, 0)
	for rows.Next() {
		var item analytics.TicketInventory
		if err := rows.Scan(
			&item.SalesEventID,
			&item.TicketID,
			&item.TicketType,
			&item.AvailableQuantity,
		); err != nil {
			return nil, err
		}
		inventory = append(inventory, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return inventory, nil
}
