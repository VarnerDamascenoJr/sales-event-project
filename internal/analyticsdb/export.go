package analyticsdb

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/varner/sales-event-project/internal/analytics"
)

// Queryer provides the database reads needed by the exporter.
type Queryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

// Exporter builds analytics documents from operational database records.
type Exporter struct {
	DB Queryer
}

// NewExporter creates an exporter backed by db.
func NewExporter(db Queryer) Exporter {
	return Exporter{DB: db}
}

// ExportAnalytics reads events and inventory and builds an analytics document.
func (e Exporter) ExportAnalytics(ctx context.Context, filter Filter) (analytics.Document, error) {
	events, err := ReadEvents(ctx, e.DB, filter)
	if err != nil {
		return analytics.Document{}, err
	}

	inventory, err := ReadTicketInventory(ctx, e.DB, filter)
	if err != nil {
		return analytics.Document{}, err
	}

	return analytics.BuildDocumentWithInventory(time.Now(), SourceForFilter(filter), events, analytics.DefaultWindowSpecs(), inventory), nil
}
