package analytics

import "math"

func buildFunnelSteps(counts funnelCounts, options FunnelOptions) []FunnelStep {
	return []FunnelStep{
		newFunnelStep(FunnelStageAccepted, FunnelStagePending, counts.accepted, counts.pending, options),
		newFunnelStep(FunnelStagePending, FunnelStagePaid, counts.pending, counts.paid, options),
		newFunnelStep(FunnelStagePaid, FunnelStageTicketIssued, counts.paid, counts.ticketIssued, options),
		newFunnelStep(FunnelStageTicketIssued, FunnelStageCheckIn, counts.ticketIssued, counts.checkedIn, options),
	}
}

func newFunnelStep(from string, to string, trials int, successes int, options FunnelOptions) FunnelStep {
	return FunnelStep{
		From:                  from,
		To:                    to,
		Trials:                trials,
		Successes:             successes,
		ConversionProbability: proportion(successes, trials),
		ConfidenceInterval:    wilsonInterval(successes, trials, options),
		Bayesian:              betaPosterior(successes, trials, options),
	}
}

func wilsonInterval(successes int, trials int, options FunnelOptions) ConfidenceInterval {
	interval := ConfidenceInterval{
		Method: "wilson",
		Level:  options.ConfidenceLevel,
	}
	if trials == 0 {
		return interval
	}
	n := float64(trials)
	p := float64(successes) / n
	z := options.WilsonZ
	z2 := z * z
	denominator := 1 + z2/n
	center := (p + z2/(2*n)) / denominator
	margin := z * math.Sqrt((p*(1-p)+z2/(4*n))/n) / denominator
	interval.Lower = clampProbability(center - margin)
	interval.Upper = clampProbability(center + margin)
	return interval
}

func betaPosterior(successes int, trials int, options FunnelOptions) BetaBinomial {
	failures := trials - successes
	if failures < 0 {
		failures = 0
	}
	posteriorAlpha := options.PriorAlpha + float64(successes)
	posteriorBeta := options.PriorBeta + float64(failures)
	return BetaBinomial{
		PriorAlpha:     options.PriorAlpha,
		PriorBeta:      options.PriorBeta,
		PosteriorAlpha: posteriorAlpha,
		PosteriorBeta:  posteriorBeta,
		PosteriorMean:  clampProbability(posteriorAlpha / (posteriorAlpha + posteriorBeta)),
	}
}

func proportion(successes int, trials int) float64 {
	if trials == 0 {
		return 0
	}
	return clampProbability(float64(successes) / float64(trials))
}
