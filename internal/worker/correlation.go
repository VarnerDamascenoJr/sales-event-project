package worker

import (
	"context"

	"github.com/varner/sales-event-project/internal/correlation"
	"github.com/varner/sales-event-project/internal/events"
)

func contextWithEventMetadata(ctx context.Context, metadata events.CorrelationMetadata) context.Context {
	current, _ := correlation.FromContext(ctx)
	merged := correlation.Merge(current, correlation.Metadata{
		RequestID:     metadata.RequestID,
		CorrelationID: metadata.CorrelationID,
		TransactionID: metadata.TransactionID,
	})
	if merged == (correlation.Metadata{}) {
		return ctx
	}
	return correlation.ContextWithMetadata(ctx, merged)
}
