package analytics

import (
	"testing"
	"time"
)

func TestBuildSurvivalAnalysesSeparatesObservedAndCensoredDurations(t *testing.T) {
	base := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	censorAt := base.Add(20 * time.Minute)
	events := []Event{
		{EventType: "sale.created", OccurredAt: base, SalesEventID: "event-1", SaleID: "sale-1", Status: "COMPLETED"},
		{EventType: "sale.item.created", OccurredAt: base, SalesEventID: "event-1", SaleID: "sale-1", TicketID: "ticket-general", TicketType: "General", Status: "COMPLETED"},
		{EventType: "payment.processed", OccurredAt: base.Add(2 * time.Minute), SalesEventID: "event-1", SaleID: "sale-1", Status: "APPROVED", Provider: "card"},
		{EventType: "email.sent", OccurredAt: base.Add(4 * time.Minute), SalesEventID: "event-1", SaleID: "sale-1", Status: "SENT"},
		{EventType: "checkin.completed", OccurredAt: base.Add(12 * time.Minute), SalesEventID: "event-1", SaleID: "sale-1", TicketID: "ticket-general", TicketType: "General", Status: "COMPLETED"},

		{EventType: "sale.created", OccurredAt: base.Add(time.Minute), SalesEventID: "event-1", SaleID: "sale-2", Status: "PENDING_PAYMENT"},
		{EventType: "sale.item.created", OccurredAt: base.Add(time.Minute), SalesEventID: "event-1", SaleID: "sale-2", TicketID: "ticket-general", TicketType: "General", Status: "PENDING_PAYMENT"},
	}

	analyses := BuildSurvivalAnalyses(events, censorAt, []SurvivalIntervalSpec{
		{StartSeconds: 0, EndSeconds: 300},
		{StartSeconds: 300, EndSeconds: 900},
		{StartSeconds: 900, EndSeconds: 0},
	})

	payment := findSurvivalAnalysis(t, analyses, "time_to_payment")
	if payment.ObservationCount != 2 || payment.EventCount != 1 || payment.CensoredCount != 1 {
		t.Fatalf("unexpected payment counts: %+v", payment)
	}
	if payment.Percentiles["p50"] != 120 {
		t.Fatalf("unexpected payment median: %+v", payment.Percentiles)
	}
	if payment.HazardTable[0].AtRisk != 2 || payment.HazardTable[0].Events != 1 {
		t.Fatalf("unexpected first payment hazard bucket: %+v", payment.HazardTable[0])
	}
	if payment.HazardTable[2].Censored != 1 {
		t.Fatalf("expected one censored payment observation in open-ended bucket, got %+v", payment.HazardTable[2])
	}

	email := findSurvivalAnalysis(t, analyses, "time_to_email_sent")
	if email.Percentiles["p50"] != 240 {
		t.Fatalf("unexpected email median: %+v", email.Percentiles)
	}

	checkIn := findSurvivalAnalysis(t, analyses, "time_to_check_in")
	if checkIn.UnitOfAnalysis != "sale_ticket" {
		t.Fatalf("unexpected check-in unit: %s", checkIn.UnitOfAnalysis)
	}
	if checkIn.Percentiles["p50"] != 720 {
		t.Fatalf("unexpected check-in median: %+v", checkIn.Percentiles)
	}
}

func TestBuildSurvivalAnalysesReturnsNoPercentilesWithoutObservedEvents(t *testing.T) {
	base := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	analyses := BuildSurvivalAnalyses([]Event{
		{EventType: "sale.created", OccurredAt: base, SalesEventID: "event-1", SaleID: "sale-1", Status: "PENDING_PAYMENT"},
		{EventType: "sale.item.created", OccurredAt: base, SalesEventID: "event-1", SaleID: "sale-1", TicketID: "ticket-general", TicketType: "General", Status: "PENDING_PAYMENT"},
	}, base.Add(10*time.Minute), []SurvivalIntervalSpec{{StartSeconds: 0, EndSeconds: 0}})

	payment := findSurvivalAnalysis(t, analyses, "time_to_payment")
	if payment.EventCount != 0 || payment.CensoredCount != 1 {
		t.Fatalf("unexpected censored-only counts: %+v", payment)
	}
	if payment.Percentiles != nil {
		t.Fatalf("expected no percentiles without observed events, got %+v", payment.Percentiles)
	}
}

func findSurvivalAnalysis(t *testing.T, analyses []SurvivalAnalysis, eventName string) SurvivalAnalysis {
	t.Helper()
	for _, analysis := range analyses {
		if analysis.EventName == eventName {
			return analysis
		}
	}
	t.Fatalf("survival analysis %s not found", eventName)
	return SurvivalAnalysis{}
}
