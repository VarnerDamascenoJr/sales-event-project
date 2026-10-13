package analytics

import "testing"

func TestBuildFunnelStepsHandlesZeroDenominatorSteps(t *testing.T) {
	steps := buildFunnelSteps(funnelCounts{accepted: 1}, DefaultFunnelOptions())

	paidStep := steps[1]
	if paidStep.From != FunnelStagePending || paidStep.To != FunnelStagePaid {
		t.Fatalf("unexpected paid step: %+v", paidStep)
	}
	if paidStep.Trials != 0 || paidStep.Successes != 0 {
		t.Fatalf("expected zero denominator paid step, got %+v", paidStep)
	}
	if paidStep.ConfidenceInterval.Lower != 0 || paidStep.ConfidenceInterval.Upper != 0 {
		t.Fatalf("expected empty confidence interval for zero denominator, got %+v", paidStep.ConfidenceInterval)
	}
	if paidStep.Bayesian.PosteriorMean != 0.5 {
		t.Fatalf("expected Beta(1,1) posterior mean for no data, got %f", paidStep.Bayesian.PosteriorMean)
	}
}

func TestNewFunnelStepClampsInvalidCounts(t *testing.T) {
	step := newFunnelStep(FunnelStageAccepted, FunnelStagePending, 1, 2, DefaultFunnelOptions())

	if step.ConversionProbability != 1 {
		t.Fatalf("expected conversion probability to be clamped, got %f", step.ConversionProbability)
	}
	if step.Bayesian.PosteriorBeta != 1 {
		t.Fatalf("expected negative failures to be clamped to zero, got %+v", step.Bayesian)
	}
}
