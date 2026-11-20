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
	// Starting the recurrence at exp(-demand) loses precision near underflow.
	if expectedDemand > 700 {
		return poissonLargeDemandProbability(expectedDemand, availableQuantity)
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

func poissonLargeDemandProbability(demand float64, availableQuantity int) float64 {
	stock := float64(availableQuantity)
	if demand > 1e10 {
		// A continuity-corrected normal approximation bounds work for enormous
		// inputs, where summing the exact Poisson tail can require millions of terms.
		z := (stock + 0.5 - demand) / math.Sqrt(demand)
		return clampProbability(0.5 * math.Erfc(z/math.Sqrt2))
	}

	// Start at the boundary, in log space, then sum the shorter tail away
	// from the mode. This avoids losing the entire distribution to underflow.
	if stock < demand {
		term := poissonMass(demand, stock)
		cdf := term
		for k := stock; k > 0; k-- {
			term *= k / demand
			next := cdf + term
			if next == cdf {
				break
			}
			cdf = next
		}
		return clampProbability(1 - cdf)
	}

	k := stock + 1
	term := poissonMass(demand, k)
	tail := term
	for term > 0 {
		k++
		term *= demand / k
		next := tail + term
		if next == tail {
			break
		}
		tail = next
	}
	return clampProbability(tail)
}

func poissonMass(demand, k float64) float64 {
	logFactorial, _ := math.Lgamma(k + 1)
	return math.Exp(k*math.Log(demand) - demand - logFactorial)
}
