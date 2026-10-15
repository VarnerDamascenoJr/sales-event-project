package analyticsdb

import (
	"fmt"
	"strings"
	"time"

	"github.com/varner/sales-event-project/internal/analytics"
)

const (
	// DefaultLimit is used when the event reader receives a non-positive limit.
	DefaultLimit = 2000
	// MaxLimit is the maximum limit accepted by the analytics HTTP endpoint.
	MaxLimit = 10000
)

// Filter selects the event history and inventory included in an export.
type Filter struct {
	SalesEventID string
	Start        string
	End          string
	Limit        int
}

// ValidateTimestampBound accepts an empty bound or a timestamp in RFC3339 format.
func ValidateTimestampBound(value string, name string) error {
	if value == "" {
		return nil
	}
	if _, err := time.Parse(time.RFC3339, value); err != nil {
		return fmt.Errorf("%s must be RFC3339: %w", name, err)
	}
	return nil
}

// SourceForFilter describes the filters applied to an analytics export.
func SourceForFilter(filter Filter) analytics.Source {
	source := analytics.Source{}
	filters := make([]string, 0, 2)
	if filter.SalesEventID != "" {
		source.SalesEventID = filter.SalesEventID
		filters = append(filters, "sales_event_id")
	}
	if filter.Start != "" || filter.End != "" {
		filters = append(filters, "time_range")
	}
	source.Filter = strings.Join(filters, ",")
	return source
}
