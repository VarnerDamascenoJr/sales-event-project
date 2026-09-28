package analytics

import (
	"math"
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

type demandKey struct {
	SalesEventID string
	TicketID     string
	TicketType   string
	WindowSize   string
}

type demandSeries struct {
	key       demandKey
	duration  time.Duration
	byWindow  map[time.Time]int
	first     time.Time
	last      time.Time
	hasWindow bool
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

func buildDemandSeries(events []Event, specs []WindowSpec) map[demandKey]demandSeries {
	seriesByKey := make(map[demandKey]demandSeries)
	for _, event := range normalizeEvents(events) {
		if event.EventType != "sale.item.created" || event.TicketID == "" {
			continue
		}
		quantity := event.Quantity
		if quantity <= 0 {
			quantity = 1
		}
		for _, spec := range specs {
			start := event.OccurredAt.Truncate(spec.Duration)
			key := demandKey{
				SalesEventID: event.SalesEventID,
				TicketID:     event.TicketID,
				TicketType:   event.TicketType,
				WindowSize:   spec.Name,
			}
			series := seriesByKey[key]
			if series.byWindow == nil {
				series = demandSeries{
					key:      key,
					duration: spec.Duration,
					byWindow: make(map[time.Time]int),
					first:    start,
					last:     start,
				}
			}
			if !series.hasWindow || start.Before(series.first) {
				series.first = start
			}
			if !series.hasWindow || start.After(series.last) {
				series.last = start
			}
			series.hasWindow = true
			series.byWindow[start] += quantity
			seriesByKey[key] = series
		}
	}
	return seriesByKey
}

func buildDemandForecast(series demandSeries, options DemandForecastOptions) (DemandForecast, bool) {
	if denseDemandWindowCount(series) > options.MaxSeriesWindows {
		return DemandForecast{}, false
	}

	quantities, starts := denseDemandQuantities(series)
	if len(quantities) < 2 {
		return DemandForecast{}, false
	}

	testCount := demandTestWindowCount(len(quantities), options)
	if testCount <= 0 || testCount >= len(quantities) {
		return DemandForecast{}, false
	}
	trainCount := len(quantities) - testCount

	points := make([]DemandSeriesPoint, 0, len(quantities)+options.HorizonWindows)
	var sumAbsoluteError float64
	var sumSquaredError float64
	for i, quantity := range quantities {
		observed := quantity
		point := DemandSeriesPoint{
			WindowStart:      starts[i],
			WindowEnd:        starts[i].Add(series.duration),
			ObservedQuantity: &observed,
		}
		if i < trainCount {
			point.Split = "train"
			points = append(points, point)
			continue
		}

		predicted := movingAverage(quantities[:i], options.MovingAverageWindow)
		absoluteError := math.Abs(float64(quantity) - predicted)
		squaredError := absoluteError * absoluteError
		point.Split = "test"
		point.PredictedQuantity = &predicted
		point.AbsoluteError = &absoluteError
		point.SquaredError = &squaredError
		sumAbsoluteError += absoluteError
		sumSquaredError += squaredError
		points = append(points, point)
	}

	forecastQuantity := movingAverage(quantities, options.MovingAverageWindow)
	forecastStart := starts[len(starts)-1].Add(series.duration)
	for i := 0; i < options.HorizonWindows; i++ {
		predicted := forecastQuantity
		points = append(points, DemandSeriesPoint{
			WindowStart:       forecastStart,
			WindowEnd:         forecastStart.Add(series.duration),
			Split:             "forecast",
			PredictedQuantity: &predicted,
		})
		forecastStart = forecastStart.Add(series.duration)
	}

	return DemandForecast{
		SalesEventID:        series.key.SalesEventID,
		TicketID:            series.key.TicketID,
		TicketType:          series.key.TicketType,
		WindowSize:          series.key.WindowSize,
		Method:              DemandForecastMethodMovingAverage,
		MovingAverageWindow: options.MovingAverageWindow,
		TrainWindowCount:    trainCount,
		TestWindowCount:     testCount,
		ForecastWindowStart: starts[len(starts)-1].Add(series.duration),
		ForecastWindowEnd:   starts[len(starts)-1].Add(series.duration * time.Duration(options.HorizonWindows+1)),
		ForecastQuantity:    forecastQuantity,
		Metrics: DemandForecastMetrics{
			MAE:  sumAbsoluteError / float64(testCount),
			RMSE: math.Sqrt(sumSquaredError / float64(testCount)),
		},
		Series: points,
	}, true
}

func denseDemandQuantities(series demandSeries) ([]int, []time.Time) {
	if !series.hasWindow {
		return nil, nil
	}
	quantities := make([]int, 0)
	starts := make([]time.Time, 0)
	for current := series.first; !current.After(series.last); current = current.Add(series.duration) {
		quantities = append(quantities, series.byWindow[current])
		starts = append(starts, current)
	}
	return quantities, starts
}

func denseDemandWindowCount(series demandSeries) int {
	if !series.hasWindow || series.duration <= 0 {
		return 0
	}
	return int(series.last.Sub(series.first)/series.duration) + 1
}

func demandTestWindowCount(seriesLength int, options DemandForecastOptions) int {
	testCount := int(math.Ceil(float64(seriesLength) * options.TestFraction))
	if testCount < options.MinimumTestWindows {
		testCount = options.MinimumTestWindows
	}
	if testCount >= seriesLength {
		testCount = seriesLength - 1
	}
	return testCount
}

func movingAverage(values []int, window int) float64 {
	if len(values) == 0 {
		return 0
	}
	start := len(values) - window
	if start < 0 {
		start = 0
	}
	var total int
	for _, value := range values[start:] {
		total += value
	}
	return float64(total) / float64(len(values[start:]))
}
