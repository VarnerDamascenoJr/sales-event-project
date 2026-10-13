package analytics

import (
	"sort"
	"time"
)

const SimulationPriorsSchemaVersion = "optiflow-sales-priors.v1"

func BuildSimulationPriors(generatedAt time.Time, source Source, events []Event, funnels []FunnelSegment, survivalAnalyses []SurvivalAnalysis, demandForecasts []DemandForecast, stockoutRisks []StockoutRisk) *SimulationPriors {
	records := buildSalePriorRecords(events)
	if len(records) == 0 {
		return nil
	}

	saleIDs := make([]string, 0, len(records))
	for saleID := range records {
		saleIDs = append(saleIDs, saleID)
	}
	sort.Strings(saleIDs)

	period := simulationPriorPeriod(records)
	sampleSize, completedDemand := simulationPriorSampleSize(records, saleIDs)
	estimates := simulationPriorEstimates(sampleSize, completedDemand)
	return &SimulationPriors{
		SchemaVersion: SimulationPriorsSchemaVersion,
		GeneratedAt:   generatedAt.UTC(),
		Source: SimulationPriorSource{
			Service:       firstNonEmpty(source.Service, "sales-event-project"),
			SchemaVersion: SchemaVersion,
			SalesEventID:  source.SalesEventID,
			Filter:        source.Filter,
		},
		Period:            period,
		SampleSize:        sampleSize,
		Estimates:         estimates,
		Uncertainty:       simulationPriorUncertainty(estimates),
		ConversionStages:  aggregateConversionStagePriors(funnels),
		Demand:            demandPriors(demandForecasts),
		OperationalTiming: operationalTimingPriors(survivalAnalyses),
		StockoutRisks:     stockoutRiskPriors(stockoutRisks),
	}
}
