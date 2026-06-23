package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/require"
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
		AutoBan: &autoBan,
		Setting: &setting,
		Weight:  &weight,
		Group:   "default",
		Models:  "gpt-4o-mini",
		Status:  common.ChannelStatusEnabled,
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

func TestGetRandomSatisfiedChannelExcludeKeepsStrictPriorityWhenTierDegraded(t *testing.T) {
	const (
		group = "strict-priority-group"
		model = "strict-priority-model"
	)

	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	oldGroup2Model2Channels := group2model2channels
	oldChannelsIDM := channelsIDM
	t.Cleanup(func() {
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
		group2model2channels = oldGroup2Model2Channels
		channelsIDM = oldChannelsIDM
	})

	common.MemoryCacheEnabled = true
	highPriority := int64(100)
	lowPriority := int64(10)
	weight := uint(100)

	channelsIDM = map[int]*Channel{}
	group2model2channels = map[string]map[string][]int{
		group: {
			model: {},
		},
	}

	for id := 1; id <= 4; id++ {
		channelsIDM[id] = &Channel{
			Id:       id,
			Priority: &highPriority,
			Weight:   &weight,
		}
		group2model2channels[group][model] = append(group2model2channels[group][model], id)
	}
	for id := 101; id <= 110; id++ {
		channelsIDM[id] = &Channel{
			Id:       id,
			Priority: &lowPriority,
			Weight:   &weight,
		}
		group2model2channels[group][model] = append(group2model2channels[group][model], id)
	}

	exclude := map[int]bool{
		1: true,
		2: true,
		3: true,
	}

	for i := 0; i < 20; i++ {
		selected, err := GetRandomSatisfiedChannelExclude(group, model, exclude)
		if err != nil {
			t.Fatalf("select channel failed: %v", err)
		}
		if selected == nil {
			t.Fatal("expected a selected channel")
		}
		if selected.Id != 4 {
			t.Fatalf("expected remaining high-priority channel id=4, got id=%d", selected.Id)
		}
	}
}

func TestDBChannelSelectionLoadsExternalTimeoutConfig(t *testing.T) {
	truncateTables(t)

	const (
		group = "db-timeout-group"
		model = "db-timeout-model"
	)
	priority := int64(10)
	weight := uint(100)
	maxLatency := 5
	channel := &Channel{
		Name:                 "db-timeout-channel",
		Key:                  "sk-timeout",
		Group:                group,
		Models:               model,
		Status:               common.ChannelStatusEnabled,
		Priority:             &priority,
		Weight:               &weight,
		MaxFirstTokenLatency: &maxLatency,
	}
	require.NoError(t, channel.Insert())

	selected, err := GetChannel(group, model, 0, "")
	require.NoError(t, err)
	require.NotNil(t, selected)
	require.Equal(t, maxLatency, selected.GetMaxFirstTokenLatency())

	selectedExclude, err := GetChannelExclude(group, model, nil)
	require.NoError(t, err)
	require.NotNil(t, selectedExclude)
	require.Equal(t, maxLatency, selectedExclude.GetMaxFirstTokenLatency())
}

func TestFixAbilityLoadsExternalDynamicBreakerConfig(t *testing.T) {
	truncateTables(t)

	priority := int64(10)
	weight := uint(100)
	autoBan := 1
	channel := &Channel{
		Name:                  "db-breaker-channel",
		Key:                   "sk-breaker",
		Group:                 "db-breaker-group",
		Models:                "db-breaker-model",
		Status:                common.ChannelStatusAutoDisabled,
		Priority:              &priority,
		Weight:                &weight,
		AutoBan:               &autoBan,
		DynamicCircuitBreaker: true,
	}
	require.NoError(t, channel.Insert())
	require.NoError(t, DB.Model(&Ability{}).Where("channel_id = ?", channel.Id).Update("enabled", false).Error)

	success, failed, err := FixAbility()
	require.NoError(t, err)
	require.Equal(t, 1, success)
	require.Equal(t, 0, failed)

	var ability Ability
	require.NoError(t, DB.First(&ability, "channel_id = ?", channel.Id).Error)
	require.True(t, ability.Enabled)
}

func TestUpdateChannelStatusPreservesExternalConfigs(t *testing.T) {
	truncateTables(t)

	priority := int64(10)
	weight := uint(100)
	autoBan := 1
	maxLatency := 5
	interval := 15
	coeff := 2.5
	channel := &Channel{
		Name:                     "status-preserve-channel",
		Key:                      "sk-status",
		Group:                    "status-preserve-group",
		Models:                   "status-preserve-model",
		Status:                   common.ChannelStatusEnabled,
		Priority:                 &priority,
		Weight:                   &weight,
		AutoBan:                  &autoBan,
		DynamicCircuitBreaker:    true,
		ToleranceCoefficient:     &coeff,
		MaxFirstTokenLatency:     &maxLatency,
		ScheduledTestInterval:    &interval,
		MaxRetryAttempts:         3,
		TreatEmptyReplyAsFailure: true,
	}
	require.NoError(t, channel.Insert())

	require.True(t, UpdateChannelStatus(channel.Id, "", common.ChannelStatusAutoDisabled, "test status update"))

	reloaded, err := GetChannelById(channel.Id, true)
	require.NoError(t, err)
	require.True(t, reloaded.DynamicCircuitBreaker)
	require.NotNil(t, reloaded.ToleranceCoefficient)
	require.Equal(t, coeff, *reloaded.ToleranceCoefficient)
	require.Equal(t, maxLatency, reloaded.GetMaxFirstTokenLatency())
	require.Equal(t, interval, reloaded.GetScheduledTestInterval())
	require.Equal(t, 3, reloaded.GetMaxRetryAttempts())
	require.True(t, reloaded.TreatEmptyReplyAsFailure)
}
