package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/varner/sales-event-project/internal/analyticsdb"
	"github.com/varner/sales-event-project/internal/config"
	"github.com/varner/sales-event-project/internal/database"
)

func main() {
	var salesEventID string
	var outputPath string
	var start string
	var end string
	var pretty bool
	var limit int

	flag.StringVar(&salesEventID, "sales-event-id", "", "optional sales event UUID filter")
	flag.StringVar(&outputPath, "output", "", "optional output path; stdout is used when empty")
	flag.StringVar(&start, "start", "", "optional inclusive RFC3339 lower timestamp bound")
	flag.StringVar(&end, "end", "", "optional exclusive RFC3339 upper timestamp bound")
	flag.BoolVar(&pretty, "pretty", true, "render indented JSON")
	flag.IntVar(&limit, "limit", 2000, "maximum number of analytic events to export")
	flag.Parse()

	if limit <= 0 {
		slog.Error("limit must be greater than zero")
		os.Exit(2)
	}
	if err := analyticsdb.ValidateTimestampBound(start, "start"); err != nil {
		slog.Error("invalid timestamp bound", "error", err)
		os.Exit(2)
	}
	if err := analyticsdb.ValidateTimestampBound(end, "end"); err != nil {
		slog.Error("invalid timestamp bound", "error", err)
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cfg := config.Load()
	db, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("connect postgres failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	filter := analyticsdb.Filter{
		SalesEventID: salesEventID,
		Start:        start,
		End:          end,
		Limit:        limit,
	}
	document, err := analyticsdb.NewExporter(db).ExportAnalytics(ctx, filter)
	if err != nil {
		slog.Error("export analytics failed", "error", err)
		os.Exit(1)
	}

	payload, err := marshalJSON(document, pretty)
	if err != nil {
		slog.Error("marshal analytics export failed", "error", err)
		os.Exit(1)
	}

	if outputPath == "" {
		fmt.Println(string(payload))
		return
	}

	if err := os.WriteFile(outputPath, append(payload, '\n'), 0o600); err != nil {
		slog.Error("write analytics export failed", "path", outputPath, "error", err)
		os.Exit(1)
	}
}

func marshalJSON(value any, pretty bool) ([]byte, error) {
	if pretty {
		return json.MarshalIndent(value, "", "  ")
	}
	return json.Marshal(value)
}
