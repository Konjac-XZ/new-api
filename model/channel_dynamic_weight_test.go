package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

func newChannelWithDynamicWeightForTest(dynamicEnabled bool) *Channel {
	autoBan := 1
	if !dynamicEnabled {
		autoBan = 0
	}
	weight := uint(100)
	settingBytes, _ := common.Marshal(dto.ChannelSettings{DynamicCircuitBreaker: dynamicEnabled})
	setting := string(settingBytes)
	return &Channel{
		AutoBan:  &autoBan,
		Setting:  &setting,
		Weight:   &weight,
		Group:    "default",
		Models:   "gpt-4o-mini",
		Status:   common.ChannelStatusEnabled,
	}
}

func TestDynamicWeightMetricsDisabledDefaults(t *testing.T) {
	ch := newChannelWithDynamicWeightForTest(false)
	metrics := ch.GetDynamicWeightMetrics()
	if metrics.Enabled {
		t.Fatal("expected metrics disabled when dynamic breaker is off")
	}
	if metrics.ConfidenceMultiplier != 1.0 {
		t.Fatalf("expected confidence multiplier 1.0, got %f", metrics.ConfidenceMultiplier)
	}
	if ch.GetEffectiveRoutingWeight(80) != 80 {
		t.Fatalf("expected effective weight to equal base weight when disabled")
	}
}

func TestDynamicWeightMetricsHPAndRatePenalty(t *testing.T) {
	chHealthy := newChannelWithDynamicWeightForTest(true)
	chHealthy.BreakerHP = 10
	chHealthy.BreakerRecentRequests = 100
	chHealthy.BreakerRecentFailures = 0
	chHealthy.BreakerRecentTimeouts = 0

	chDegraded := newChannelWithDynamicWeightForTest(true)
	chDegraded.BreakerHP = 3
	chDegraded.BreakerRecentRequests = 100
	chDegraded.BreakerRecentFailures = 40
	chDegraded.BreakerRecentTimeouts = 20

	healthy := chHealthy.GetDynamicWeightMetrics()
	degraded := chDegraded.GetDynamicWeightMetrics()

	if degraded.ConfidenceMultiplier >= healthy.ConfidenceMultiplier {
		t.Fatalf("expected degraded multiplier lower than healthy, got degraded=%f healthy=%f", degraded.ConfidenceMultiplier, healthy.ConfidenceMultiplier)
	}
	if degraded.RatePenaltyFactor >= 1.0 {
		t.Fatalf("expected degraded rate penalty factor below 1.0, got %f", degraded.RatePenaltyFactor)
	}
}

func TestDynamicWeightMetricsFloorAndEffectiveWeight(t *testing.T) {
	ch := newChannelWithDynamicWeightForTest(true)
	ch.BreakerHP = 0
	ch.BreakerRecentRequests = 100
	ch.BreakerRecentFailures = 100
	ch.BreakerRecentTimeouts = 100

	metrics := ch.GetDynamicWeightMetrics()
	if metrics.ConfidenceMultiplier < 0.1 {
		t.Fatalf("expected confidence floor >= 0.1, got %f", metrics.ConfidenceMultiplier)
	}
	if metrics.ConfidenceMultiplier > 1.0 {
		t.Fatalf("expected confidence <= 1.0, got %f", metrics.ConfidenceMultiplier)
	}
	if got := ch.GetEffectiveRoutingWeight(1); got < 1 {
		t.Fatalf("expected positive base weight to keep minimum effective weight 1, got %d", got)
	}
	if got := ch.GetEffectiveRoutingWeight(0); got != 0 {
		t.Fatalf("expected zero base weight to remain zero, got %d", got)
	}
}
