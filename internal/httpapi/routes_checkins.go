package httpapi

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/varner/sales-event-project/internal/correlation"
	"github.com/varner/sales-event-project/internal/metrics"
	"github.com/varner/sales-event-project/internal/observability"
	"go.opentelemetry.io/otel/attribute"
)

func registerCheckInRoutes(router *gin.Engine, deps RouterDeps) {
	router.POST("/sales-events/:salesEventId/check-ins", requireRoles(deps.AuthStore, RoleCheckIn, RoleAdmin), func(c *gin.Context) {
		var req CreateCheckInRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		checkInReq := CheckInTicketRequest{
			SalesEventID: c.Param("salesEventId"),
			TicketCode:   req.TicketCode,
		}
		if err := validateCheckInRequest(&checkInReq); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		requestCtx, checkInSpan := observability.StartBusinessSpan(c.Request.Context(), "ticket.check_in",
			attribute.String("sales_event.id", checkInReq.SalesEventID),
		)
		result, err := deps.Store.CheckInTicket(requestCtx, checkInReq)
		if err == nil {
			checkInSpan.SetAttributes(append(observability.CorrelationAttributes(correlation.ContextWithMetadata(requestCtx, result.Metadata)),
				attribute.String("sale.id", result.SaleID),
				attribute.String("ticket.issued.id", result.IssuedTicketID),
			)...)
		}
		observability.EndSpan(checkInSpan, err)
		if err != nil {
			status := http.StatusBadRequest
			switch {
			case errors.Is(err, errSalesEventNotFound), errors.Is(err, errIssuedTicketNotFound):
				status = http.StatusNotFound
			case errors.Is(err, errTicketAlreadyCheckedIn), errors.Is(err, errTicketWrongEvent), errors.Is(err, errTicketCannotBeCheckedIn):
				status = http.StatusConflict
			default:
				var validationErr errValidation
				if !errors.As(err, &validationErr) {
					status = http.StatusInternalServerError
				}
			}
			c.JSON(status, gin.H{"error": err.Error()})
			return
		}

		correlation.WriteHTTPHeaders(c.Writer.Header(), result.Metadata)
		metrics.CheckInsCreatedTotal.Inc()
		logFields := append(correlation.LogFields(result.Metadata),
			"check_in_id", result.CheckInID,
			"issued_ticket_id", result.IssuedTicketID,
			"sales_event_id", result.SalesEventID,
			"sale_id", result.SaleID,
		)
		slog.Info("ticket checked in", logFields...)
		c.JSON(http.StatusCreated, result)
	})
}
