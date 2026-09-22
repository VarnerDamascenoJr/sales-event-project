package analytics

import (
	"testing"
	"time"
)

func TestBuildFunnelSegmentsComputesConditionalConversions(t *testing.T) {
	base := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	events := []Event{
		{EventType: "sale.created", OccurredAt: base, SalesEventID: "event-1", SaleID: "sale-1", Status: "COMPLETED"},
		{EventType: "sale.item.created", OccurredAt: base, SalesEventID: "event-1", SaleID: "sale-1", TicketID: "ticket-general", TicketType: "General", Status: "COMPLETED", Quantity: 1},
		{EventType: "payment.processed", OccurredAt: base.Add(time.Minute), SalesEventID: "event-1", SaleID: "sale-1", Status: "APPROVED", Provider: "card"},
		{EventType: "ticket.issued", OccurredAt: base.Add(2 * time.Minute), SalesEventID: "event-1", SaleID: "sale-1", TicketID: "ticket-general", TicketType: "General", Status: "ISSUED", Quantity: 1},
		{EventType: "checkin.completed", OccurredAt: base.Add(3 * time.Minute), SalesEventID: "event-1", SaleID: "sale-1", TicketID: "ticket-general", TicketType: "General", Status: "COMPLETED", Quantity: 1},

		{EventType: "sale.created", OccurredAt: base.Add(30 * time.Second), SalesEventID: "event-1", SaleID: "sale-2", Status: "PENDING_PAYMENT"},
		{EventType: "sale.item.created", OccurredAt: base.Add(30 * time.Second), SalesEventID: "event-1", SaleID: "sale-2", TicketID: "ticket-general", TicketType: "General", Status: "PENDING_PAYMENT", Quantity: 1},
	}

	segments := BuildFunnelSegments(events, []WindowSpec{{Name: "5m", Duration: 5 * time.Minute}}, DefaultFunnelOptions())
	if len(segments) != 2 {
		t.Fatalf("expected paid and unknown provider segments, got %d", len(segments))
	}

	paidSegment := findFunnelSegment(t, segments, "card")
	if paidSegment.SalesEventID != "event-1" || paidSegment.TicketID != "ticket-general" || paidSegment.TicketType != "General" {
		t.Fatalf("unexpected paid segment dimensions: %+v", paidSegment)
	}
	if !paidSegment.WindowStart.Equal(base) || !paidSegment.WindowEnd.Equal(base.Add(5*time.Minute)) {
		t.Fatalf("unexpected paid segment window: %+v", paidSegment)
	}
	assertFunnelStep(t, paidSegment.Steps[0], FunnelStageAccepted, FunnelStagePending, 1, 1, 1)
	assertFunnelStep(t, paidSegment.Steps[1], FunnelStagePending, FunnelStagePaid, 1, 1, 1)
	assertFunnelStep(t, paidSegment.Steps[2], FunnelStagePaid, FunnelStageTicketIssued, 1, 1, 1)
	assertFunnelStep(t, paidSegment.Steps[3], FunnelStageTicketIssued, FunnelStageCheckIn, 1, 1, 1)
	if paidSegment.Steps[0].ConfidenceInterval.Method != "wilson" {
		t.Fatalf("unexpected confidence method: %s", paidSegment.Steps[0].ConfidenceInterval.Method)
	}
	if paidSegment.Steps[0].Bayesian.PosteriorAlpha != 2 || paidSegment.Steps[0].Bayesian.PosteriorBeta != 1 {
		t.Fatalf("unexpected beta posterior: %+v", paidSegment.Steps[0].Bayesian)
	}

	unknownSegment := findFunnelSegment(t, segments, "unknown")
	assertFunnelStep(t, unknownSegment.Steps[1], FunnelStagePending, FunnelStagePaid, 1, 0, 0)
}

func TestBuildFunnelSegmentsHandlesZeroDenominatorSteps(t *testing.T) {
	base := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	segments := BuildFunnelSegments([]Event{
		{EventType: "sale.created", OccurredAt: base, SalesEventID: "event-1", SaleID: "sale-1", Status: "FAILED"},
		{EventType: "sale.item.created", OccurredAt: base, SalesEventID: "event-1", SaleID: "sale-1", TicketID: "ticket-general", TicketType: "General", Status: "FAILED", Quantity: 1},
	}, []WindowSpec{{Name: "5m", Duration: 5 * time.Minute}}, DefaultFunnelOptions())

	if len(segments) != 1 {
		t.Fatalf("expected one segment, got %d", len(segments))
	}
	paidStep := segments[0].Steps[1]
	if paidStep.Trials != 0 || paidStep.Successes != 0 {
		t.Fatalf("expected zero denominator paid step, got %+v", paidStep)
	}
	if paidStep.ConfidenceInterval.Lower != 0 || paidStep.ConfidenceInterval.Upper != 0 {
		t.Fatalf("expected empty confidence interval for zero denominator, got %+v", paidStep.ConfidenceInterval)
	}
	if paidStep.Bayesian.PosteriorMean != 0.5 {
		t.Fatalf("expected Beta(1,1) posterior mean for no data, got %f", paidStep.Bayesian.PosteriorMean)
	}
}

func findFunnelSegment(t *testing.T, segments []FunnelSegment, provider string) FunnelSegment {
	t.Helper()
	for _, segment := range segments {
		if segment.Provider == provider {
			return segment
		}
	}
	t.Fatalf("segment with provider %s not found", provider)
	return FunnelSegment{}
}

func assertFunnelStep(t *testing.T, step FunnelStep, from string, to string, trials int, successes int, probability float64) {
	t.Helper()
	if step.From != from || step.To != to {
		t.Fatalf("unexpected funnel step: %+v", step)
	}
	if step.Trials != trials || step.Successes != successes {
		t.Fatalf("unexpected counts for %s -> %s: %+v", from, to, step)
	}
	if step.ConversionProbability != probability {
		t.Fatalf("unexpected probability for %s -> %s: %f", from, to, step.ConversionProbability)
	}
}
