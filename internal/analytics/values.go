package analytics

import "math"

func clampProbability(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, -1) {
		return 0
	}
	if math.IsInf(value, 1) {
		return 1
	}
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func isFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func finiteOrZero(value float64) float64 {
	if !isFinite(value) {
		return 0
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
