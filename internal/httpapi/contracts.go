package httpapi

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/varner/sales-event-project/internal/analytics"
	"github.com/varner/sales-event-project/internal/analyticsdb"
)

type RouterDeps struct {
	Broker               EventPublisher
	DB                   *pgxpool.Pool
	Store                SalesStore
	AuthStore            AuthStore
	AnalyticsExporter    AnalyticsExporter
	WebhookSecret        string
	PaymentWebhookSecret string
	MetricsProtected     bool
	PublicRateLimiter    *RateLimiter
}

type EventPublisher interface {
	PublishJSON(ctx context.Context, routingKey string, value any) error
}

type SalesStore interface {
	SalesEventExists(ctx context.Context, salesEventID string) (bool, error)
	GetTicketForEvent(ctx context.Context, ticketID string, salesEventID string) (TicketReadModel, error)
	ListSales(ctx context.Context, filter listSalesFilter) (ListSalesResponse, error)
	GetSale(ctx context.Context, salesEventID string, saleID string) (SaleDetailDTO, error)
	CreatePaymentIntent(ctx context.Context, req CreatePaymentIntentStoreRequest) (PaymentIntentDTO, error)
	ProcessPayment(ctx context.Context, req ProcessPaymentRequest) (ProcessPaymentResult, error)
	ProcessPaymentWebhook(ctx context.Context, req ProcessPaymentWebhookRequest) (ProcessPaymentResult, error)
	CheckInTicket(ctx context.Context, req CheckInTicketRequest) (CheckInTicketResult, error)
	RecordEmailEvent(ctx context.Context, req RecordEmailEventRequest) (RecordEmailEventResult, error)
}

type AnalyticsExporter interface {
	ExportAnalytics(context.Context, analyticsdb.Filter) (analytics.Document, error)
}
