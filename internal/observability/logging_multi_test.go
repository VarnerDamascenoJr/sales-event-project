package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"reflect"
	"testing"
	"time"
)

func TestMultiHandlerEnabled(t *testing.T) {
	tests := []struct {
		name   string
		levels []slog.Level
		want   bool
	}{
		{name: "no handlers"},
		{name: "all disabled", levels: []slog.Level{slog.LevelWarn, slog.LevelError}},
		{name: "first enabled", levels: []slog.Level{slog.LevelInfo, slog.LevelError}, want: true},
		{name: "last enabled", levels: []slog.Level{slog.LevelError, slog.LevelDebug}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := multiHandler{}
			for _, level := range tt.levels {
				handler.handlers = append(handler.handlers, &multiTestHandler{minLevel: level})
			}
			if got := handler.Enabled(context.Background(), slog.LevelInfo); got != tt.want {
				t.Fatalf("expected Enabled=%t, got %t", tt.want, got)
			}
		})
	}
}

func TestMultiHandlerHandleForwardsRecordToEnabledHandlers(t *testing.T) {
	first := &multiTestHandler{minLevel: slog.LevelDebug}
	disabled := &multiTestHandler{minLevel: slog.LevelError}
	last := &multiTestHandler{minLevel: slog.LevelInfo}
	handler := multiHandler{handlers: []slog.Handler{first, disabled, last}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	record := slog.NewRecord(time.Unix(1, 0), slog.LevelInfo, "sale accepted", 42)
	record.AddAttrs(slog.String("sale_id", "sale-123"))

	if err := handler.Handle(ctx, record); err != nil {
		t.Fatalf("handle log record: %v", err)
	}
	if len(disabled.records) != 0 {
		t.Fatal("disabled handler received a log record")
	}
	for _, target := range []*multiTestHandler{first, last} {
		if len(target.records) != 1 {
			t.Fatalf("expected one log record, got %d", len(target.records))
		}
		if target.ctx != ctx {
			t.Fatal("handler received a different context")
		}
		got := target.records[0]
		if got.Message != record.Message || got.Level != record.Level || got.Time != record.Time || got.PC != record.PC {
			t.Fatalf("log record fields changed: %+v", got)
		}
		if got.NumAttrs() != 1 {
			t.Fatalf("expected one record attribute, got %d", got.NumAttrs())
		}
		got.Attrs(func(attr slog.Attr) bool {
			if attr.Key != "sale_id" || attr.Value.String() != "sale-123" {
				t.Fatalf("unexpected record attribute: %v", attr)
			}
			return true
		})
	}
}

func TestMultiHandlerHandleJoinsErrorsAndContinuesDelivery(t *testing.T) {
	firstErr := errors.New("first handler failed")
	lastErr := errors.New("last handler failed")
	disabledErr := errors.New("disabled handler failed")
	first := &multiTestHandler{err: firstErr}
	last := &multiTestHandler{err: lastErr}
	disabled := &multiTestHandler{minLevel: slog.LevelError, err: disabledErr}
	success := &multiTestHandler{}
	handler := multiHandler{handlers: []slog.Handler{first, disabled, last, success}}

	err := handler.Handle(context.Background(), slog.NewRecord(time.Time{}, slog.LevelInfo, "sale accepted", 0))
	if !errors.Is(err, firstErr) || !errors.Is(err, lastErr) {
		t.Fatalf("expected both handler errors, got %v", err)
	}
	if errors.Is(err, disabledErr) {
		t.Fatalf("disabled handler error was included: %v", err)
	}
	for _, target := range []*multiTestHandler{first, last, success} {
		if len(target.records) != 1 {
			t.Fatalf("expected delivery to continue after errors, got %d records", len(target.records))
		}
	}
	if len(disabled.records) != 0 {
		t.Fatal("disabled handler received a log record")
	}
}

func TestMultiHandlerWithAttrsAndGroups(t *testing.T) {
	tests := []struct {
		name   string
		derive func(slog.Handler) slog.Handler
		want   map[string]any
	}{
		{
			name: "attributes",
			derive: func(h slog.Handler) slog.Handler {
				return h.WithAttrs([]slog.Attr{slog.String("service", "sales")})
			},
			want: map[string]any{"service": "sales", "sale_id": "sale-123"},
		},
		{
			name: "group",
			derive: func(h slog.Handler) slog.Handler {
				return h.WithGroup("sale")
			},
			want: map[string]any{"sale": map[string]any{"sale_id": "sale-123"}},
		},
		{
			name: "empty group",
			derive: func(h slog.Handler) slog.Handler {
				return h.WithGroup("")
			},
			want: map[string]any{"sale_id": "sale-123"},
		},
		{
			name: "attributes inside group",
			derive: func(h slog.Handler) slog.Handler {
				return h.WithGroup("sale").WithAttrs([]slog.Attr{slog.String("service", "sales")})
			},
			want: map[string]any{"sale": map[string]any{"service": "sales", "sale_id": "sale-123"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var outputs [2]bytes.Buffer
			handler := multiHandler{handlers: []slog.Handler{
				slog.NewJSONHandler(&outputs[0], nil),
				slog.NewJSONHandler(&outputs[1], nil),
			}}
			slog.New(tt.derive(handler)).Info("derived", "sale_id", "sale-123")
			slog.New(handler).Info("original", "sale_id", "sale-456")

			for i := range outputs {
				decoder := json.NewDecoder(&outputs[i])
				for _, expected := range []struct {
					message string
					attrs   map[string]any
				}{
					{message: "derived", attrs: tt.want},
					{message: "original", attrs: map[string]any{"sale_id": "sale-456"}},
				} {
					var got map[string]any
					if err := decoder.Decode(&got); err != nil {
						t.Fatalf("decode handler %d output: %v", i, err)
					}
					if got["msg"] != expected.message || got["level"] != "INFO" {
						t.Fatalf("unexpected handler %d record: %v", i, got)
					}
					delete(got, "time")
					delete(got, "level")
					delete(got, "msg")
					if !reflect.DeepEqual(got, expected.attrs) {
						t.Fatalf("handler %d attributes: expected %v, got %v", i, expected.attrs, got)
					}
				}
			}
		})
	}
}

func TestMultiHandlerWithoutHandlers(t *testing.T) {
	handler := multiHandler{}
	ctx := context.Background()
	record := slog.NewRecord(time.Time{}, slog.LevelInfo, "sale accepted", 0)
	for _, target := range []slog.Handler{
		handler,
		handler.WithAttrs([]slog.Attr{slog.String("service", "sales")}),
		handler.WithGroup("sale"),
	} {
		if target.Enabled(ctx, record.Level) {
			t.Fatal("handler without destinations is enabled")
		}
		if err := target.Handle(ctx, record); err != nil {
			t.Fatalf("handler without destinations returned an error: %v", err)
		}
	}
}

type multiTestHandler struct {
	minLevel slog.Level
	err      error
	records  []slog.Record
	ctx      context.Context
}

func (h *multiTestHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.minLevel
}

func (h *multiTestHandler) Handle(ctx context.Context, record slog.Record) error {
	h.ctx = ctx
	h.records = append(h.records, record.Clone())
	return h.err
}

func (h *multiTestHandler) WithAttrs([]slog.Attr) slog.Handler {
	return h
}

func (h *multiTestHandler) WithGroup(string) slog.Handler {
	return h
}
