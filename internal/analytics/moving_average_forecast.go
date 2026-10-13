package analytics

import (
	"math"
	"time"
)

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
