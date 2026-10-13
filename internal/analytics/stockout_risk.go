package analytics

import "time"

func DefaultStockoutRiskOptions() StockoutRiskOptions {
	return StockoutRiskOptions{
		WindowSize:             "5m",
		HorizonWindows:         12,
		MinimumObservedWindows: 2,
		MaxSeriesWindows:       500,
	}
}

func BuildStockoutRisks(events []Event, specs []WindowSpec, inventory []TicketInventory, generatedAt time.Time, options StockoutRiskOptions) []StockoutRisk {
	if len(inventory) == 0 {
		return nil
	}

	normalizedSpecs := normalizeWindowSpecs(specs)
	normalizedOptions := normalizeStockoutRiskOptions(options)
	spec, ok := stockoutWindowSpec(normalizedSpecs, normalizedOptions.WindowSize)
	if !ok {
		return nil
	}

	seriesByKey := buildDemandSeries(events, []WindowSpec{spec})
	normalizedInventory := normalizeTicketInventory(inventory)
	risks := make([]StockoutRisk, 0, len(normalizedInventory))
	for _, item := range normalizedInventory {
		key := demandKey{
			SalesEventID: item.SalesEventID,
			TicketID:     item.TicketID,
			TicketType:   item.TicketType,
			WindowSize:   spec.Name,
		}
		risks = append(risks, buildStockoutRisk(item, seriesByKey[key], generatedAt.UTC(), spec, normalizedOptions))
	}
	return risks
}

func normalizeStockoutRiskOptions(options StockoutRiskOptions) StockoutRiskOptions {
	defaults := DefaultStockoutRiskOptions()
	if options.WindowSize == "" {
		options.WindowSize = defaults.WindowSize
	}
	if options.HorizonWindows <= 0 {
		options.HorizonWindows = defaults.HorizonWindows
	}
	if options.MinimumObservedWindows <= 0 {
		options.MinimumObservedWindows = defaults.MinimumObservedWindows
	}
	if options.MaxSeriesWindows <= 0 {
		options.MaxSeriesWindows = defaults.MaxSeriesWindows
	}
	return options
}

func stockoutWindowSpec(specs []WindowSpec, windowSize string) (WindowSpec, bool) {
	for _, spec := range specs {
		if spec.Name == windowSize {
			return spec, true
		}
	}
	return WindowSpec{}, false
}
