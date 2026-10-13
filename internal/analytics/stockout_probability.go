package analytics

import "math"

func sumInts(values []int) int {
	var total int
	for _, value := range values {
		total += value
	}
	return total
}

func meanInts(values []int) float64 {
	if len(values) == 0 {
		return 0
	}
	return float64(sumInts(values)) / float64(len(values))
}

func sampleVarianceInts(values []int, mean float64) float64 {
	if len(values) < 2 {
		return 0
	}
	var sumSquaredDiffs float64
	for _, value := range values {
		diff := float64(value) - mean
		sumSquaredDiffs += diff * diff
	}
	return sumSquaredDiffs / float64(len(values)-1)
}

func poissonStockoutProbability(expectedDemand float64, availableQuantity int) float64 {
	if availableQuantity < 0 {
		return 1
	}
	if math.IsNaN(expectedDemand) || expectedDemand <= 0 {
		return 0
	}
	if math.IsInf(expectedDemand, 1) {
		return 1
	}
	if math.IsInf(expectedDemand, -1) {
		return 0
	}

	term := math.Exp(-expectedDemand)
	if math.IsNaN(term) {
		return 0
	}
	cdf := term
	for k := 1; k <= availableQuantity; k++ {
		term *= expectedDemand / float64(k)
		if math.IsNaN(term) || math.IsInf(term, 0) {
			return 1
		}
		cdf += term
		if math.IsNaN(cdf) || math.IsInf(cdf, 0) {
			return 1
		}
		if term == 0 {
			break
		}
	}
	return clampProbability(1 - cdf)
}
