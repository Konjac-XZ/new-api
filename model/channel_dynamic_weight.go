package model

import "math"

const (
	dynamicWeightHPBase               = 10.0
	dynamicWeightMinTolerance         = 0.1
	dynamicWeightMaxTolerance         = 10.0
	dynamicWeightRateConfidenceReqs   = 30.0
	dynamicWeightFailurePenaltyWeight = 0.4
	dynamicWeightTimeoutPenaltyWeight = 0.6
	dynamicWeightConfidenceFloor      = 0.1
)

type DynamicWeightMetrics struct {
	Enabled              bool
	HPRatio              float64
	FailureRate          float64
	TimeoutRate          float64
	RateSampleConfidence float64
	RatePenaltyFactor    float64
	ConfidenceMultiplier float64
}

func clampFloat64(v, minV, maxV float64) float64 {
	if v < minV {
		return minV
	}
	if v > maxV {
		return maxV
	}
	return v
}

func (channel *Channel) GetDynamicWeightMetrics() DynamicWeightMetrics {
	metrics := DynamicWeightMetrics{
		Enabled:              false,
		HPRatio:              1.0,
		RatePenaltyFactor:    1.0,
		ConfidenceMultiplier: 1.0,
	}

	if channel == nil || !channel.IsDynamicCircuitBreakerEnabled() {
		return metrics
	}

	metrics.Enabled = true
	setting := channel.GetSetting()
	toleranceCoefficient := 1.0
	if setting.ToleranceCoefficient != nil {
		toleranceCoefficient = clampFloat64(*setting.ToleranceCoefficient, dynamicWeightMinTolerance, dynamicWeightMaxTolerance)
	}

	maxHP := dynamicWeightHPBase * toleranceCoefficient
	hp := channel.BreakerHP
	if hp < 0 {
		hp = maxHP
	}
	hp = clampFloat64(hp, 0, maxHP)
	if maxHP > 0 {
		metrics.HPRatio = hp / maxHP
	}

	if channel.BreakerRecentRequests > 0 {
		requests := channel.BreakerRecentRequests
		metrics.FailureRate = clampFloat64(channel.BreakerRecentFailures/requests, 0, 1)
		metrics.TimeoutRate = clampFloat64(channel.BreakerRecentTimeouts/requests, 0, 1)
		metrics.RateSampleConfidence = clampFloat64(requests/dynamicWeightRateConfidenceReqs, 0, 1)
	}

	ratePenaltyBase := dynamicWeightFailurePenaltyWeight*metrics.FailureRate + dynamicWeightTimeoutPenaltyWeight*metrics.TimeoutRate
	ratePenalty := 1.0 - metrics.RateSampleConfidence*ratePenaltyBase
	metrics.RatePenaltyFactor = clampFloat64(ratePenalty, 0, 1)

	metrics.ConfidenceMultiplier = clampFloat64(
		metrics.HPRatio*metrics.RatePenaltyFactor,
		dynamicWeightConfidenceFloor,
		1.0,
	)
	return metrics
}

func (channel *Channel) GetEffectiveRoutingWeight(baseWeight int) int {
	if baseWeight <= 0 {
		return 0
	}
	metrics := channel.GetDynamicWeightMetrics()
	effectiveWeight := int(math.Round(float64(baseWeight) * metrics.ConfidenceMultiplier))
	if effectiveWeight < 1 {
		return 1
	}
	return effectiveWeight
}
