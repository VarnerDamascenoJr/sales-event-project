package analytics

import (
	"math"
	"strings"
	"time"
)

func buildSalePriorRecords(events []Event) map[string]*salePriorRecord {
	records := make(map[string]*salePriorRecord)
	for _, event := range events {
		if event.SaleID == "" {
			continue
		}
		record := records[event.SaleID]
		if record == nil {
			record = &salePriorRecord{}
			records[event.SaleID] = record
		}
		switch event.EventType {
		case "sale.created":
			if record.CreatedAt.IsZero() || event.OccurredAt.Before(record.CreatedAt) {
				record.CreatedAt = event.OccurredAt
			}
			if strings.EqualFold(event.Status, "COMPLETED") {
				record.Completed = true
			}
		case "payment.processed":
			if strings.EqualFold(event.Status, "APPROVED") {
				record.Completed = true
			}
		case "sale.item.created":
			record.Demand += eventQuantity(event)
		}
	}
	return records
}

func simulationPriorPeriod(records map[string]*salePriorRecord) SimulationPriorPeriod {
	var start time.Time
	var end time.Time
	for _, record := range records {
		if record.CreatedAt.IsZero() {
			continue
		}
		if start.IsZero() || record.CreatedAt.Before(start) {
			start = record.CreatedAt
		}
		if end.IsZero() || record.CreatedAt.After(end) {
			end = record.CreatedAt
		}
	}
	return SimulationPriorPeriod{
		StartedAt: formatPriorTime(start),
		EndedAt:   formatPriorTime(end),
	}
}

func simulationPriorSampleSize(records map[string]*salePriorRecord, saleIDs []string) (SimulationPriorSampleSize, []int) {
	sampleSize := SimulationPriorSampleSize{SaleCount: len(records)}
	completedDemand := make([]int, 0)
	for _, saleID := range saleIDs {
		record := records[saleID]
		sampleSize.TotalItemQuantity += record.Demand
		if !record.Completed {
			continue
		}
		sampleSize.CompletedSaleCount++
		sampleSize.CompletedItemQuantity += record.Demand
		if record.Demand > 0 {
			sampleSize.CompletedDemandCount++
			completedDemand = append(completedDemand, record.Demand)
		}
	}
	sampleSize.CancelledSaleCount = sampleSize.SaleCount - sampleSize.CompletedSaleCount
	return sampleSize, completedDemand
}

func simulationPriorEstimates(sampleSize SimulationPriorSampleSize, completedDemand []int) SimulationPriorEstimates {
	demandMean := meanFloat64(completedDemand)
	demandStdDev := populationStdDev(completedDemand, demandMean)
	demandCoefficientOfVariation := 0.0
	if demandMean > 0 {
		demandCoefficientOfVariation = demandStdDev / demandMean
	}
	modalDemand := intMode(completedDemand)
	deviationProbability := 0.0
	if len(completedDemand) > 0 {
		deviations := 0
		for _, demand := range completedDemand {
			if demand != modalDemand {
				deviations++
			}
		}
		deviationProbability = float64(deviations) / float64(len(completedDemand))
	}

	conversionProbability := 0.0
	cancellationProbability := 0.0
	if sampleSize.SaleCount > 0 {
		conversionProbability = float64(sampleSize.CompletedSaleCount) / float64(sampleSize.SaleCount)
		cancellationProbability = float64(sampleSize.CancelledSaleCount) / float64(sampleSize.SaleCount)
	}

	return SimulationPriorEstimates{
		ConversionProbability:        round4(conversionProbability),
		CancellationProbability:      round4(cancellationProbability),
		DemandMean:                   round4(demandMean),
		DemandStdDev:                 round4(demandStdDev),
		DemandCoefficientOfVariation: round4(demandCoefficientOfVariation),
		ModalDemand:                  modalDemand,
		DemandDeviationProbability:   round4(deviationProbability),
	}
}

func simulationPriorUncertainty(estimates SimulationPriorEstimates) SimulationPriorUncertainty {
	return SimulationPriorUncertainty{
		CancellationProbability:    estimates.CancellationProbability,
		DemandVariationProbability: estimates.DemandDeviationProbability,
		DemandVariationRate:        estimates.DemandCoefficientOfVariation,
	}
}

func eventQuantity(event Event) int {
	if event.Quantity <= 0 {
		return 1
	}
	return event.Quantity
}

func formatPriorTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func meanFloat64(values []int) float64 {
	if len(values) == 0 {
		return 0
	}
	total := 0
	for _, value := range values {
		total += value
	}
	return float64(total) / float64(len(values))
}

func populationStdDev(values []int, mean float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var sumSquaredDiffs float64
	for _, value := range values {
		diff := float64(value) - mean
		sumSquaredDiffs += diff * diff
	}
	return math.Sqrt(sumSquaredDiffs / float64(len(values)))
}

func intMode(values []int) int {
	if len(values) == 0 {
		return 0
	}
	counts := make(map[int]int)
	for _, value := range values {
		counts[value]++
	}
	mode := values[0]
	for value, count := range counts {
		if count > counts[mode] || (count == counts[mode] && value < mode) {
			mode = value
		}
	}
	return mode
}

func round4(value float64) float64 {
	if !isFinite(value) {
		return 0
	}
	return math.Round(value*10000) / 10000
}
