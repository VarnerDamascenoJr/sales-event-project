package httpapi

import (
	"github.com/gin-gonic/gin"
	"github.com/varner/sales-event-project/internal/correlation"
	"github.com/varner/sales-event-project/internal/events"
)

func correlationMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		metadata := correlation.FromHTTPHeader(c.Request.Header)
		correlation.WriteHTTPHeaders(c.Request.Header, metadata)
		c.Request = c.Request.WithContext(correlation.ContextWithMetadata(c.Request.Context(), metadata))
		correlation.WriteHTTPHeaders(c.Writer.Header(), metadata)
		c.Next()
	}
}

func requestCorrelationMetadata(c *gin.Context, transactionID string) correlation.Metadata {
	metadata, _ := correlation.FromContext(c.Request.Context())
	return correlation.WithTransactionID(metadata, transactionID)
}

func eventCorrelationMetadata(metadata correlation.Metadata) events.CorrelationMetadata {
	return events.CorrelationMetadata{
		RequestID:     metadata.RequestID,
		CorrelationID: metadata.CorrelationID,
		TransactionID: metadata.TransactionID,
	}
}
