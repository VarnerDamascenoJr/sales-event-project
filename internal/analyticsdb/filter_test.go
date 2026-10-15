package analyticsdb

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/varner/sales-event-project/internal/analytics"
)

func TestValidateTimestampBound(t *testing.T) {
	for _, tt := range []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "empty"},
		{name: "UTC", value: "2026-10-14T10:00:00Z"},
		{name: "offset", value: "2026-10-14T10:00:00-03:00"},
		{name: "fractional seconds", value: "2026-10-14T10:00:00.123Z"},
		{name: "date only", value: "2026-10-14", wantErr: true},
		{name: "invalid date", value: "2026-02-30T10:00:00Z", wantErr: true},
		{name: "malformed", value: "invalid", wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTimestampBound(tt.value, "start")
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("expected a valid timestamp bound, got %v", err)
				}
				return
			}
			var parseErr *time.ParseError
			if !errors.As(err, &parseErr) || !strings.HasPrefix(err.Error(), "start must be RFC3339: ") {
				t.Fatalf("expected a named, wrapped timestamp parse error, got %v", err)
			}
		})
	}
}

func TestSourceForFilter(t *testing.T) {
	for _, tt := range []struct {
		name   string
		filter Filter
		want   analytics.Source
	}{
		{name: "no filters"},
		{name: "limit only", filter: Filter{Limit: 10}},
		{name: "sales event", filter: Filter{SalesEventID: "event-123"}, want: analytics.Source{SalesEventID: "event-123", Filter: "sales_event_id"}},
		{name: "start", filter: Filter{Start: "2026-10-14T10:00:00Z"}, want: analytics.Source{Filter: "time_range"}},
		{name: "end", filter: Filter{End: "2026-10-15T10:00:00Z"}, want: analytics.Source{Filter: "time_range"}},
		{name: "both bounds", filter: Filter{Start: "start", End: "end"}, want: analytics.Source{Filter: "time_range"}},
		{name: "combined", filter: Filter{SalesEventID: "event-123", Start: "start", End: "end", Limit: 10}, want: analytics.Source{SalesEventID: "event-123", Filter: "sales_event_id,time_range"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := SourceForFilter(tt.filter); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("expected source %+v, got %+v", tt.want, got)
			}
		})
	}
}
