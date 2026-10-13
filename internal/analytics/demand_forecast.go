package analytics

import (
	"sort"
	"time"
)

const DemandForecastMethodMovingAverage = "moving_average"

type DemandForecastOptions struct {
	MovingAverageWindow int
	TestFraction        float64
	MinimumTestWindows  int
	HorizonWindows      int
	MaxSeriesWindows    int
}

func DefaultDemandForecastOptions() DemandForecastOptions {
	return DemandForecastOptions{
		MovingAverageWindow: 3,
		TestFraction:        0.25,
		MinimumTestWindows:  1,
		HorizonWindows:      1,
		MaxSeriesWindows:    500,
	}
}

type DemandForecast struct {
	SalesEventID        string                `json:"salesEventId,omitempty"`
	TicketID            string                `json:"ticketId,omitempty"`
	TicketType          string                `json:"ticketType,omitempty"`
	WindowSize          string                `json:"windowSize"`
	Method              string                `json:"method"`
	MovingAverageWindow int                   `json:"movingAverageWindow"`
	TrainWindowCount    int                   `json:"trainWindowCount"`
	TestWindowCount     int                   `json:"testWindowCount"`
	ForecastWindowStart time.Time             `json:"forecastWindowStart"`
	ForecastWindowEnd   time.Time             `json:"forecastWindowEnd"`
	ForecastQuantity    float64               `json:"forecastQuantity"`
	Metrics             DemandForecastMetrics `json:"metrics"`
	Series              []DemandSeriesPoint   `json:"series"`
}

type DemandForecastMetrics struct {
	MAE  float64 `json:"mae"`
	RMSE float64 `json:"rmse"`
}

type DemandSeriesPoint struct {
	WindowStart       time.Time `json:"windowStart"`
	WindowEnd         time.Time `json:"windowEnd"`
	Split             string    `json:"split"`
	ObservedQuantity  *int      `json:"observedQuantity,omitempty"`
	PredictedQuantity *float64  `json:"predictedQuantity,omitempty"`
	AbsoluteError     *float64  `json:"absoluteError,omitempty"`
	SquaredError      *float64  `json:"squaredError,omitempty"`
}

func BuildDemandForecasts(events []Event, specs []WindowSpec, options DemandForecastOptions) []DemandForecast {
	normalizedSpecs := normalizeWindowSpecs(specs)
	normalizedOptions := normalizeDemandForecastOptions(options)
	seriesByKey := buildDemandSeries(events, normalizedSpecs)
	if len(seriesByKey) == 0 {
		return nil
	}

	keys := make([]demandKey, 0, len(seriesByKey))
	for key := range seriesByKey {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		left := keys[i]
		right := keys[j]
		if left.WindowSize != right.WindowSize {
			return left.WindowSize < right.WindowSize
		}
		if left.SalesEventID != right.SalesEventID {
			return left.SalesEventID < right.SalesEventID
		}
		if left.TicketType != right.TicketType {
			return left.TicketType < right.TicketType
		}
		return left.TicketID < right.TicketID
	})

	forecasts := make([]DemandForecast, 0, len(keys))
	for _, key := range keys {
		forecast, ok := buildDemandForecast(seriesByKey[key], normalizedOptions)
		if ok {
			forecasts = append(forecasts, forecast)
		}
	}
	return forecasts
}

func normalizeDemandForecastOptions(options DemandForecastOptions) DemandForecastOptions {
	defaults := DefaultDemandForecastOptions()
	if options.MovingAverageWindow <= 0 {
		options.MovingAverageWindow = defaults.MovingAverageWindow
	}
	if options.TestFraction <= 0 || options.TestFraction >= 1 {
		options.TestFraction = defaults.TestFraction
	}
	if options.MinimumTestWindows <= 0 {
		options.MinimumTestWindows = defaults.MinimumTestWindows
	}
	if options.HorizonWindows <= 0 {
		options.HorizonWindows = defaults.HorizonWindows
	}
	if options.MaxSeriesWindows <= 0 {
		options.MaxSeriesWindows = defaults.MaxSeriesWindows
	}
	return options
}
