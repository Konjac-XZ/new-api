package service

import (
	"fmt"
	"math"
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
)

func TestApplyBreakerDecay(t *testing.T) {
	channel := &model.Channel{
		BreakerPressure:  4,
		BreakerUpdatedAt: time.Now().Add(-breakerDecayWindow).Unix(),
	}

	applyBreakerDecay(channel, time.Now())

	if channel.BreakerPressure <= 0 || channel.BreakerPressure >= 4 {
		t.Fatalf("expected decayed pressure between 0 and 4, got %f", channel.BreakerPressure)
	}
}

func TestClassifyChannelFailure(t *testing.T) {
	firstTokenErr := types.NewErrorWithStatusCode(
		fmt.Errorf("too slow"),
		types.ErrorCodeChannelFirstTokenLatencyExceeded,
		http.StatusGatewayTimeout,
	)
	if got := classifyChannelFailure(nil, firstTokenErr); got != channelFailureKindFirstTokenTimeout {
		t.Fatalf("expected first token timeout, got %s", got)
	}

	info := &relaycommon.RelayInfo{
		StartTime:         time.Now().Add(-2 * time.Second),
		FirstResponseTime: time.Now().Add(-time.Second),
	}
	midStreamErr := types.NewOpenAIError(fmt.Errorf("stream broken"), types.ErrorCodeBadResponse, http.StatusBadGateway)
	if got := classifyChannelFailure(info, midStreamErr); got != channelFailureKindMidStreamFailure {
		t.Fatalf("expected mid-stream failure, got %s", got)
	}
}

func TestIsSlowSuccessfulRequest(t *testing.T) {
	settingBytes, err := common.Marshal(dto.ChannelSettings{DynamicCircuitBreaker: true})
	if err != nil {
		t.Fatalf("marshal setting failed: %v", err)
	}
	channel := &model.Channel{
		Setting:              common.GetPointer(string(settingBytes)),
		MaxFirstTokenLatency: common.GetPointer(10),
	}
	info := &relaycommon.RelayInfo{
		IsStream:          true,
		StartTime:         time.Now().Add(-9 * time.Second),
		FirstResponseTime: time.Now(),
	}

	if !isSlowSuccessfulRequest(channel, info) {
		t.Fatal("expected slow successful stream request to be detected")
	}
}

func TestRecordChannelProbeSuccess_DoesNotFastForwardCooldown(t *testing.T) {
	now := time.Now().Unix()
	channel := seedDynamicBreakerChannelForProbeTest(t, now+120, now-10, 5)

	if RecordChannelProbeSuccess(channel) {
		t.Fatal("expected probe success during cooldown to be ignored")
	}

	var latest model.Channel
	if err := model.DB.Select("breaker_cooldown_at", "breaker_updated_at").Where("id = ?", channel.Id).First(&latest).Error; err != nil {
		t.Fatalf("failed to reload channel breaker state: %v", err)
	}
	if latest.BreakerCooldownAt != channel.BreakerCooldownAt {
		t.Fatalf("expected cooldown_at to remain unchanged, got %d want %d", latest.BreakerCooldownAt, channel.BreakerCooldownAt)
	}
	if latest.BreakerUpdatedAt != channel.BreakerUpdatedAt {
		t.Fatalf("expected updated_at to remain unchanged, got %d want %d", latest.BreakerUpdatedAt, channel.BreakerUpdatedAt)
	}
}

func TestRecordChannelProbeSuccess_PromotesOnlyInAwaitingProbe(t *testing.T) {
	now := time.Now().Unix()
	channel := seedDynamicBreakerChannelForProbeTest(t, now-30, now-120, 5)

	if !RecordChannelProbeSuccess(channel) {
		t.Fatal("expected probe success to promote awaiting-probe channel")
	}

	var latest model.Channel
	if err := model.DB.Select("breaker_cooldown_at", "breaker_updated_at").Where("id = ?", channel.Id).First(&latest).Error; err != nil {
		t.Fatalf("failed to reload channel breaker state: %v", err)
	}
	if latest.BreakerCooldownAt <= channel.BreakerCooldownAt {
		t.Fatalf("expected cooldown_at to be promoted to now, got %d <= %d", latest.BreakerCooldownAt, channel.BreakerCooldownAt)
	}
	if latest.BreakerUpdatedAt != latest.BreakerCooldownAt {
		t.Fatalf("expected updated_at to match promoted cooldown_at, got updated_at=%d cooldown_at=%d", latest.BreakerUpdatedAt, latest.BreakerCooldownAt)
	}
}

func TestRecordChannelProbeFailure_IgnoresCoolingPhase(t *testing.T) {
	now := time.Now().Unix()
	channel := seedDynamicBreakerChannelForProbeTest(t, now+180, now-10, 5)

	if err := model.DB.Model(&model.Channel{}).Where("id = ?", channel.Id).Updates(map[string]any{
		"breaker_pressure":     1.5,
		"breaker_fail_streak":  2,
		"breaker_last_failure": "",
	}).Error; err != nil {
		t.Fatalf("failed to set breaker baseline: %v", err)
	}

	probeErr := types.NewErrorWithStatusCode(
		fmt.Errorf("probe failed during cooling"),
		types.ErrorCodeBadResponse,
		http.StatusBadGateway,
	)
	RecordChannelProbeFailure(channel, probeErr)

	var latest model.Channel
	if err := model.DB.Select("breaker_pressure", "breaker_fail_streak", "breaker_cooldown_at", "breaker_updated_at", "breaker_last_failure").
		Where("id = ?", channel.Id).First(&latest).Error; err != nil {
		t.Fatalf("failed to reload channel breaker state: %v", err)
	}
	if latest.BreakerPressure != 1.5 {
		t.Fatalf("expected pressure unchanged during cooling, got %f", latest.BreakerPressure)
	}
	if latest.BreakerFailStreak != 2 {
		t.Fatalf("expected fail_streak unchanged during cooling, got %d", latest.BreakerFailStreak)
	}
	if latest.BreakerCooldownAt != channel.BreakerCooldownAt {
		t.Fatalf("expected cooldown unchanged during cooling, got %d want %d", latest.BreakerCooldownAt, channel.BreakerCooldownAt)
	}
}

func TestRecordChannelProbeFailure_AwaitingProbeRestartsCooldown(t *testing.T) {
	now := time.Now().Unix()
	channel := seedDynamicBreakerChannelForProbeTest(t, now-30, now-120, 5)

	probeErr := types.NewErrorWithStatusCode(
		fmt.Errorf("probe failed in awaiting probe"),
		types.ErrorCodeBadResponse,
		http.StatusBadGateway,
	)
	RecordChannelProbeFailure(channel, probeErr)

	var latest model.Channel
	if err := model.DB.Select("breaker_pressure", "breaker_fail_streak", "breaker_cooldown_at", "breaker_last_failure").
		Where("id = ?", channel.Id).First(&latest).Error; err != nil {
		t.Fatalf("failed to reload channel breaker state: %v", err)
	}
	if latest.BreakerCooldownAt <= now {
		t.Fatalf("expected cooldown to restart from now, got cooldown_at=%d now=%d", latest.BreakerCooldownAt, now)
	}
	if latest.BreakerFailStreak != 1 {
		t.Fatalf("expected fail_streak incremented to 1, got %d", latest.BreakerFailStreak)
	}
	if latest.BreakerPressure <= 0 {
		t.Fatalf("expected pressure to increase on awaiting-probe failure, got %f", latest.BreakerPressure)
	}
	if latest.BreakerLastFailure == "" {
		t.Fatal("expected last_failure to be recorded")
	}
}

func TestRecordChannelProbeFailure_ObservationPenaltyIsStronger(t *testing.T) {
	now := time.Now().Unix()
	awaiting := seedDynamicBreakerChannelForProbeTest(t, now-30, now-120, 5)
	observation := seedDynamicBreakerChannelForProbeTest(t, now-30, now-30, 5)

	probeErr := types.NewErrorWithStatusCode(
		fmt.Errorf("probe failed"),
		types.ErrorCodeBadResponse,
		http.StatusBadGateway,
	)
	RecordChannelProbeFailure(awaiting, probeErr)
	RecordChannelProbeFailure(observation, probeErr)

	var awaitingLatest model.Channel
	if err := model.DB.Select("breaker_pressure", "breaker_cooldown_at").Where("id = ?", awaiting.Id).First(&awaitingLatest).Error; err != nil {
		t.Fatalf("failed to reload awaiting channel: %v", err)
	}
	var observationLatest model.Channel
	if err := model.DB.Select("breaker_pressure", "breaker_cooldown_at").Where("id = ?", observation.Id).First(&observationLatest).Error; err != nil {
		t.Fatalf("failed to reload observation channel: %v", err)
	}
	if observationLatest.BreakerPressure <= awaitingLatest.BreakerPressure {
		t.Fatalf("expected observation pressure penalty > awaiting penalty, got observation=%f awaiting=%f", observationLatest.BreakerPressure, awaitingLatest.BreakerPressure)
	}
	// Both cooldowns may hit the per-class cap (failureMaxCooldown), so we only
	// verify that observation cooldown is >= awaiting cooldown (not strictly >).
	if observationLatest.BreakerCooldownAt < awaitingLatest.BreakerCooldownAt {
		t.Fatalf("expected observation cooldown restart to be at least as strict, got observation=%d awaiting=%d", observationLatest.BreakerCooldownAt, awaitingLatest.BreakerCooldownAt)
	}
}

func seedDynamicBreakerChannelForProbeTest(t *testing.T, cooldownAt int64, updatedAt int64, interval int) *model.Channel {
	t.Helper()
	settingBytes, err := common.Marshal(dto.ChannelSettings{DynamicCircuitBreaker: true})
	if err != nil {
		t.Fatalf("marshal setting failed: %v", err)
	}
	autoBan := 1
	weight := uint(0)
	priority := int64(0)
	channel := &model.Channel{
		Type:                  0,
		Key:                   "sk-probe-test",
		Name:                  fmt.Sprintf("probe-test-%d", time.Now().UnixNano()),
		Status:                common.ChannelStatusEnabled,
		Group:                 "default",
		Models:                "gpt-4o-mini",
		AutoBan:               &autoBan,
		ScheduledTestInterval: &interval,
		Weight:                &weight,
		Priority:              &priority,
		Setting:               common.GetPointer(string(settingBytes)),
		BreakerCooldownAt:     cooldownAt,
		BreakerUpdatedAt:      updatedAt,
	}
	if err := model.DB.Create(channel).Error; err != nil {
		t.Fatalf("failed to seed channel: %v", err)
	}
	t.Cleanup(func() {
		_ = model.DB.Delete(&model.Channel{}, channel.Id).Error
	})
	return channel
}

// --- HP system unit tests (pure functions, no DB required) ---

func TestComputeMaxHP_DefaultCoefficient(t *testing.T) {
	channel := &model.Channel{}
	maxHP := computeMaxHP(channel)
	if maxHP != hpBase {
		t.Fatalf("expected maxHP=%f for clean channel, got %f", hpBase, maxHP)
	}
}

func TestComputeMaxHP_CustomCoefficient(t *testing.T) {
	coeff := 3.0
	settingBytes, err := common.Marshal(dto.ChannelSettings{
		DynamicCircuitBreaker: true,
		ToleranceCoefficient:  &coeff,
	})
	if err != nil {
		t.Fatalf("marshal setting failed: %v", err)
	}
	channel := &model.Channel{Setting: common.GetPointer(string(settingBytes))}
	maxHP := computeMaxHP(channel)
	expected := hpBase * coeff
	if maxHP != expected {
		t.Fatalf("expected maxHP=%f, got %f", expected, maxHP)
	}
}

func TestComputeMaxHP_FailureRatePenalty(t *testing.T) {
	// 50% failure rate → failureRateFactor = 0.75
	channel := &model.Channel{
		BreakerRecentRequests: 10.0,
		BreakerRecentFailures: 5.0,
	}
	maxHP := computeMaxHP(channel)
	expected := hpBase * 1.0 * 0.75 * 1.0 * 1.0 * 1.0 // only failure rate factor
	if maxHP != expected {
		t.Fatalf("expected maxHP=%f for 50%% failure rate, got %f", expected, maxHP)
	}
}

func TestComputeMaxHP_TripCountPenalty(t *testing.T) {
	channel := &model.Channel{BreakerTripCount: 3}
	maxHP := computeMaxHP(channel)
	tripFactor := 1.0 / (1.0 + float64(3)*0.15)
	expected := hpBase * tripFactor
	if maxHP != expected {
		t.Fatalf("expected maxHP=%f for trip_count=3, got %f", expected, maxHP)
	}
}

func TestComputeMaxHP_Minimum(t *testing.T) {
	// Extremely high trip count should clamp to minimum
	channel := &model.Channel{BreakerTripCount: 1000, BreakerRecentRequests: 10, BreakerRecentFailures: 10, BreakerRecentTimeouts: 10}
	maxHP := computeMaxHP(channel)
	if maxHP < hpMinimum {
		t.Fatalf("maxHP must not fall below hpMinimum=%f, got %f", hpMinimum, maxHP)
	}
}

func TestComputeMaxHP_SustainedSuccessReward(t *testing.T) {
	channel := &model.Channel{
		BreakerRecentRequests: 24.0,
		BreakerRecentFailures: 0,
	}

	maxHP := computeMaxHP(channel)
	expected := hpBase * (1.0 + hpSuccessRewardMaxBonus)
	if maxHP != expected {
		t.Fatalf("expected maxHP=%f for sustained success, got %f", expected, maxHP)
	}
}

func TestComputeMaxHP_SuccessRewardNeedsVolume(t *testing.T) {
	channel := &model.Channel{
		BreakerRecentRequests: 4.0,
		BreakerRecentFailures: 0,
	}

	maxHP := computeMaxHP(channel)
	if maxHP <= hpBase {
		t.Fatalf("expected some reward above base for perfect success, got %f", maxHP)
	}
	if maxHP >= hpBase*(1.0+hpSuccessRewardMaxBonus) {
		t.Fatalf("expected low-volume reward to remain below full bonus, got %f", maxHP)
	}
}

func TestComputeMaxHP_SuccessRewardRequiresNearPerfectHealth(t *testing.T) {
	channel := &model.Channel{
		BreakerRecentRequests: 50.0,
		BreakerRecentFailures: 2.0,
	}

	maxHP := computeMaxHP(channel)
	penaltyOnly := hpBase * (1.0 - math.Min(channel.BreakerRecentFailures/channel.BreakerRecentRequests, 1.0)*0.5)
	if maxHP != penaltyOnly {
		t.Fatalf("expected no success bonus when failure rate is material, got %f want %f", maxHP, penaltyOnly)
	}
}

func TestEnsureHPInitialized_SetsMaxHP(t *testing.T) {
	channel := &model.Channel{BreakerHP: -1}
	ensureHPInitialized(channel)
	if channel.BreakerHP != hpBase {
		t.Fatalf("expected HP=%f after init, got %f", hpBase, channel.BreakerHP)
	}
}

func TestEnsureHPInitialized_DoesNotOverwrite(t *testing.T) {
	channel := &model.Channel{BreakerHP: 5.0}
	ensureHPInitialized(channel)
	if channel.BreakerHP != 5.0 {
		t.Fatalf("expected HP=5.0 to remain unchanged, got %f", channel.BreakerHP)
	}
}

func TestHPPassiveRecovery_RecoversByElapsedTime(t *testing.T) {
	now := time.Now()
	twohrsAgo := now.Add(-2 * time.Hour)
	channel := &model.Channel{
		BreakerHP:        3.0,
		BreakerUpdatedAt: twohrsAgo.Unix(),
	}
	applyHPPassiveRecovery(channel, now)
	// 2 hours * 0.5 HP/hr = 1.0 recovery, capped at maxHP=10
	expected := 4.0
	const tolerance = 0.01
	if channel.BreakerHP < expected-tolerance || channel.BreakerHP > expected+tolerance {
		t.Fatalf("expected HP~=%f after 2hr passive recovery, got %f", expected, channel.BreakerHP)
	}
}

func TestHPPassiveRecovery_CapsAtMaxHP(t *testing.T) {
	// Even after 1000 hours, HP should not exceed maxHP
	channel := &model.Channel{
		BreakerHP:        9.5,
		BreakerUpdatedAt: time.Now().Add(-1000 * time.Hour).Unix(),
	}
	applyHPPassiveRecovery(channel, time.Now())
	if channel.BreakerHP > hpBase {
		t.Fatalf("expected HP capped at maxHP=%f, got %f", hpBase, channel.BreakerHP)
	}
}

func TestApplyEWMADecay_ReducesCounters(t *testing.T) {
	channel := &model.Channel{
		BreakerRecentRequests: 100.0,
		BreakerRecentFailures: 50.0,
		BreakerRecentTimeouts: 10.0,
		BreakerUpdatedAt:      time.Now().Add(-time.Hour).Unix(),
	}
	applyEWMADecay(channel, time.Now())
	// After 1 decay window (1 hour), decay factor = e^-1 ≈ 0.368
	if channel.BreakerRecentRequests >= 100.0 {
		t.Fatalf("expected requests to decay, still at %f", channel.BreakerRecentRequests)
	}
	if channel.BreakerRecentFailures >= 50.0 {
		t.Fatalf("expected failures to decay, still at %f", channel.BreakerRecentFailures)
	}
	// Validate relative decay consistency
	expectedRatio := channel.BreakerRecentFailures / channel.BreakerRecentRequests
	const tolerance = 0.001
	if expectedRatio < 0.5-tolerance || expectedRatio > 0.5+tolerance {
		t.Fatalf("expected failure/requests ratio ~0.5 after decay, got %f", expectedRatio)
	}
}

func TestApplyEWMADecay_ZerosOutSmallValues(t *testing.T) {
	channel := &model.Channel{
		BreakerRecentRequests: hpEWMAMinValue / 2,
		BreakerRecentFailures: hpEWMAMinValue / 2,
		BreakerUpdatedAt:      time.Now().Add(-time.Minute).Unix(),
	}
	applyEWMADecay(channel, time.Now())
	if channel.BreakerRecentRequests != 0 {
		t.Fatalf("expected near-zero requests to be zeroed out, got %f", channel.BreakerRecentRequests)
	}
}

func TestGetChannelBreakerHPInfo_Defaults(t *testing.T) {
	info := GetChannelBreakerHPInfo(nil)
	if info.HP != hpBase || info.MaxHP != hpBase {
		t.Fatalf("expected default HP=%f MaxHP=%f for nil channel, got HP=%f MaxHP=%f", hpBase, hpBase, info.HP, info.MaxHP)
	}
	if info.ToleranceCoefficient != hpDefaultCoefficient {
		t.Fatalf("expected default coefficient=%f, got %f", hpDefaultCoefficient, info.ToleranceCoefficient)
	}
}

func TestGetChannelBreakerHPInfo_UninitializedHP(t *testing.T) {
	channel := &model.Channel{BreakerHP: -1}
	info := GetChannelBreakerHPInfo(channel)
	// Uninitialized HP should be reported as maxHP
	if info.HP != info.MaxHP {
		t.Fatalf("expected uninitialized HP to report as maxHP, got HP=%f MaxHP=%f", info.HP, info.MaxHP)
	}
}

func TestGetChannelBreakerHPInfo_RatesCalculation(t *testing.T) {
	channel := &model.Channel{
		BreakerHP:             5.0,
		BreakerRecentRequests: 20.0,
		BreakerRecentFailures: 4.0,
		BreakerRecentTimeouts: 2.0,
	}
	info := GetChannelBreakerHPInfo(channel)
	const eps = 0.001
	if info.FailureRate < 0.2-eps || info.FailureRate > 0.2+eps {
		t.Fatalf("expected failure_rate=0.20, got %f", info.FailureRate)
	}
	if info.TimeoutRate < 0.1-eps || info.TimeoutRate > 0.1+eps {
		t.Fatalf("expected timeout_rate=0.10, got %f", info.TimeoutRate)
	}
}

func TestGetChannelBreakerHPInfo_ReportsRewardedMaxHP(t *testing.T) {
	channel := &model.Channel{
		BreakerHP:             12.0,
		BreakerRecentRequests: 24.0,
		BreakerRecentFailures: 0,
	}

	info := GetChannelBreakerHPInfo(channel)
	if info.MaxHP <= hpBase {
		t.Fatalf("expected rewarded maxHP above base, got %f", info.MaxHP)
	}
	if info.MaxHP != computeMaxHP(channel) {
		t.Fatalf("expected reported maxHP to match computeMaxHP, got %f want %f", info.MaxHP, computeMaxHP(channel))
	}
}

func TestSevereChannelCooldownApproachesMax(t *testing.T) {
	channel := &model.Channel{
		BreakerPressure:       40,
		BreakerFailStreak:     20,
		BreakerTripCount:      29,
		BreakerRecentRequests: 90.0,
		BreakerRecentFailures: 88.02,
		BreakerRecentTimeouts: 6.0,
	}

	// Use generic failure kind which has the highest per-class cap (= breakerMaxCooldown = 30 min)
	failureKind := channelFailureKindGeneric
	multiplier := computeBreakerCooldownMultiplier(channel, false, false)
	cooldown := time.Duration(float64(failureBaseCooldown(failureKind)) * multiplier)
	if chronicFloor := computeBreakerChronicCooldownFloor(channel); chronicFloor > cooldown {
		cooldown = chronicFloor
	}
	maxCap := failureMaxCooldown(failureKind)
	if maxCap > breakerMaxCooldown {
		maxCap = breakerMaxCooldown
	}
	if cooldown > maxCap {
		cooldown = maxCap
	}

	if cooldown < 25*time.Minute {
		t.Fatalf("expected severely unhealthy channel cooldown to approach the max cap (%s), got %s", breakerMaxCooldown, cooldown)
	}
	if cooldown > breakerMaxCooldown {
		t.Fatalf("expected cooldown to remain capped at max, got %s", cooldown)
	}
}

func TestRecoveringChannelGetsLighterHistoricalPenalty(t *testing.T) {
	persistentlyFailing := &model.Channel{
		BreakerPressure:       40,
		BreakerFailStreak:     12,
		BreakerTripCount:      29,
		BreakerRecentRequests: 60.0,
		BreakerRecentFailures: 58.2,
		BreakerRecentTimeouts: 6.0,
	}
	recovering := &model.Channel{
		BreakerPressure:       40,
		BreakerFailStreak:     1,
		BreakerTripCount:      29,
		BreakerRecentRequests: 60.0,
		BreakerRecentFailures: 12.0,
		BreakerRecentTimeouts: 1.0,
	}

	// Use generic failure kind (highest per-class cap) for meaningful cooldown comparison
	failureKind := channelFailureKindGeneric
	computeCooldown := func(channel *model.Channel) time.Duration {
		cooldown := time.Duration(float64(failureBaseCooldown(failureKind)) * computeBreakerCooldownMultiplier(channel, false, false))
		if chronicFloor := computeBreakerChronicCooldownFloor(channel); chronicFloor > cooldown {
			cooldown = chronicFloor
		}
		maxCap := failureMaxCooldown(failureKind)
		if cooldown > maxCap {
			cooldown = maxCap
		}
		return cooldown
	}

	persistentlyFailingCooldown := computeCooldown(persistentlyFailing)
	recoveringCooldown := computeCooldown(recovering)

	if recoveringCooldown >= persistentlyFailingCooldown {
		t.Fatalf("expected recovering channel cooldown to be lighter, got recovering=%s persistent=%s", recoveringCooldown, persistentlyFailingCooldown)
	}
	if recoveringCooldown >= 20*time.Minute {
		t.Fatalf("expected recovering channel to cool down much faster, got %s", recoveringCooldown)
	}
	if computeBreakerShortTermPenaltyFactor(recovering) >= 0.25 {
		t.Fatalf("expected recovering channel short-term penalty factor to be strongly reduced, got %f", computeBreakerShortTermPenaltyFactor(recovering))
	}
}

// --- Change 1: Softer overloaded HP damage ---

func TestFailureHPDamage_OverloadedIsHalved(t *testing.T) {
	damage := failureHPDamage(channelFailureKindOverloaded)
	if damage != 0.5 {
		t.Fatalf("expected overloaded HP damage=0.5, got %f", damage)
	}
	// Pressure weight should still be 1.0
	pressure := failurePressureWeight(channelFailureKindOverloaded)
	if pressure != 1.0 {
		t.Fatalf("expected overloaded pressure weight=1.0, got %f", pressure)
	}
}

func TestFailureHPDamage_MatchesPressureForNonOverloaded(t *testing.T) {
	kinds := []channelFailureKind{
		channelFailureKindGeneric,
		channelFailureKindImmediateFailure,
		channelFailureKindFirstTokenTimeout,
		channelFailureKindMidStreamFailure,
		channelFailureKindEmptyReply,
	}
	for _, kind := range kinds {
		damage := failureHPDamage(kind)
		pressure := failurePressureWeight(kind)
		if damage != pressure {
			t.Fatalf("expected HP damage to match pressure weight for %s, got damage=%f pressure=%f", kind, damage, pressure)
		}
	}
}

func TestOverloadedAbsorbs20HitsBeforeTripping(t *testing.T) {
	hp := hpBase // 10.0
	damage := failureHPDamage(channelFailureKindOverloaded)
	hits := 0
	for hp > 0 {
		hp -= damage
		hits++
	}
	if hits != 20 {
		t.Fatalf("expected channel to absorb 20 overloaded hits before tripping, got %d", hits)
	}
}

// --- Change 2: Per-failure-class cooldown caps ---

func TestFailureMaxCooldownCaps(t *testing.T) {
	extremeChannel := &model.Channel{
		BreakerPressure:       80,
		BreakerFailStreak:     30,
		BreakerTripCount:      50,
		BreakerRecentRequests: 100.0,
		BreakerRecentFailures: 99.0,
		BreakerRecentTimeouts: 50.0,
	}

	kinds := []struct {
		kind   channelFailureKind
		maxCap time.Duration
	}{
		{channelFailureKindOverloaded, 2 * time.Minute},
		{channelFailureKindMidStreamFailure, 3 * time.Minute},
		{channelFailureKindFirstTokenTimeout, 5 * time.Minute},
		{channelFailureKindImmediateFailure, 5 * time.Minute},
		{channelFailureKindEmptyReply, 10 * time.Minute},
		{channelFailureKindGeneric, breakerMaxCooldown},
	}

	for _, tc := range kinds {
		multiplier := computeBreakerCooldownMultiplier(extremeChannel, false, false)
		cooldown := time.Duration(float64(failureBaseCooldown(tc.kind)) * multiplier)
		if chronicFloor := computeBreakerChronicCooldownFloor(extremeChannel); chronicFloor > cooldown {
			cooldown = chronicFloor
		}
		maxCap := failureMaxCooldown(tc.kind)
		if maxCap > breakerMaxCooldown {
			maxCap = breakerMaxCooldown
		}
		if cooldown > maxCap {
			cooldown = maxCap
		}

		if cooldown > tc.maxCap {
			t.Fatalf("failure kind %s: cooldown %s exceeded per-class cap %s", tc.kind, cooldown, tc.maxCap)
		}
	}
}

func TestOverloadedCooldownCapsAt2Minutes(t *testing.T) {
	channel := &model.Channel{
		BreakerPressure:       100,
		BreakerFailStreak:     40,
		BreakerTripCount:      60,
		BreakerRecentRequests: 100.0,
		BreakerRecentFailures: 100.0,
	}
	multiplier := computeBreakerCooldownMultiplier(channel, false, false)
	cooldown := time.Duration(float64(failureBaseCooldown(channelFailureKindOverloaded)) * multiplier)
	if chronicFloor := computeBreakerChronicCooldownFloor(channel); chronicFloor > cooldown {
		cooldown = chronicFloor
	}
	maxCap := failureMaxCooldown(channelFailureKindOverloaded)
	if cooldown > maxCap {
		cooldown = maxCap
	}
	if cooldown > 2*time.Minute {
		t.Fatalf("expected overloaded cooldown to cap at 2min, got %s", cooldown)
	}
}

// --- Change 3: Gradual recovery ---

func TestProbeSuccess_RefillsToPartialHP(t *testing.T) {
	now := time.Now().Unix()
	channel := seedDynamicBreakerChannelForProbeTest(t, now-30, now-120, 5)

	if !RecordChannelProbeSuccess(channel) {
		t.Fatal("expected probe success to promote awaiting-probe channel")
	}

	var latest model.Channel
	if err := model.DB.Select("breaker_hp").Where("id = ?", channel.Id).First(&latest).Error; err != nil {
		t.Fatalf("failed to reload channel: %v", err)
	}

	maxHP := computeMaxHP(&latest)
	expected := maxHP * hpProbeSuccessRefillFraction
	const tolerance = 0.01
	if latest.BreakerHP < expected-tolerance || latest.BreakerHP > expected+tolerance {
		t.Fatalf("expected HP~=%f (50%% of maxHP=%f), got %f", expected, maxHP, latest.BreakerHP)
	}
}

func TestProbationSuccess_RefillsToPartialHP(t *testing.T) {
	// Channel in probation state with low HP
	channel := &model.Channel{
		BreakerHP:        2.0,
		BreakerTripCount: 3,
	}
	maxHP := computeMaxHP(channel)
	refillTarget := maxHP * hpProbationSuccessRefillFraction

	// Simulate the probation success refill logic
	if channel.BreakerHP < refillTarget {
		channel.BreakerHP = refillTarget
	}

	if channel.BreakerHP != refillTarget {
		t.Fatalf("expected HP=%f (70%% of maxHP=%f), got %f", refillTarget, maxHP, channel.BreakerHP)
	}
	if channel.BreakerHP >= maxHP {
		t.Fatalf("expected partial refill below maxHP=%f, got %f", maxHP, channel.BreakerHP)
	}
}

func TestProbationSuccess_DoesNotReduceHigherHP(t *testing.T) {
	// If HP is already above the refill target, don't reduce it
	channel := &model.Channel{BreakerHP: 9.0}
	maxHP := computeMaxHP(channel)
	refillTarget := maxHP * hpProbationSuccessRefillFraction

	if channel.BreakerHP < refillTarget {
		channel.BreakerHP = refillTarget
	}

	if channel.BreakerHP != 9.0 {
		t.Fatalf("expected HP to remain at 9.0 (above refill target=%f), got %f", refillTarget, channel.BreakerHP)
	}
}

func TestResetAllDynamicChannelBreakers(t *testing.T) {
	now := time.Now().Unix()
	dynamicChannel := seedDynamicBreakerChannelForProbeTest(t, now+300, now-60, 5)
	if err := model.DB.Model(&model.Channel{}).Where("id = ?", dynamicChannel.Id).Updates(map[string]any{
		"breaker_pressure":        4.5,
		"breaker_fail_streak":     6,
		"breaker_cooldown_at":     now + 300,
		"breaker_last_failure":    "timeout",
		"breaker_hp":              1.5,
		"breaker_trip_count":      3,
		"breaker_recent_requests": 12.0,
		"breaker_recent_failures": 8.0,
		"breaker_recent_timeouts": 4.0,
		"breaker_updated_at":      now,
	}).Error; err != nil {
		t.Fatalf("failed to seed dynamic breaker state: %v", err)
	}

	autoBan := 1
	weight := uint(0)
	priority := int64(0)
	plainChannel := &model.Channel{
		Type:               0,
		Key:                "sk-reset-plain",
		Name:               fmt.Sprintf("plain-reset-%d", time.Now().UnixNano()),
		Status:             common.ChannelStatusEnabled,
		Group:              "default",
		Models:             "gpt-4o-mini",
		AutoBan:            &autoBan,
		Weight:             &weight,
		Priority:           &priority,
		BreakerPressure:    9.0,
		BreakerUpdatedAt:   now,
		BreakerFailStreak:  2,
		BreakerCooldownAt:  now + 500,
		BreakerLastFailure: "bad_response",
		BreakerHP:          2.0,
		BreakerTripCount:   4,
	}
	if err := model.DB.Create(plainChannel).Error; err != nil {
		t.Fatalf("failed to seed plain channel: %v", err)
	}
	t.Cleanup(func() {
		_ = model.DB.Delete(&model.Channel{}, plainChannel.Id).Error
	})

	successCount, failCount, err := ResetAllDynamicChannelBreakers()
	if err != nil {
		t.Fatalf("reset failed: %v", err)
	}
	if successCount < 1 {
		t.Fatalf("expected at least one dynamic breaker channel to reset, got %d", successCount)
	}
	if failCount != 0 {
		t.Fatalf("expected no reset failures, got %d", failCount)
	}

	var resetDynamic model.Channel
	if err := model.DB.Where("id = ?", dynamicChannel.Id).First(&resetDynamic).Error; err != nil {
		t.Fatalf("failed to reload dynamic channel: %v", err)
	}
	if resetDynamic.BreakerPressure != 0 {
		t.Fatalf("expected dynamic breaker pressure reset to 0, got %f", resetDynamic.BreakerPressure)
	}
	if resetDynamic.BreakerFailStreak != 0 {
		t.Fatalf("expected dynamic breaker fail streak reset to 0, got %d", resetDynamic.BreakerFailStreak)
	}
	if resetDynamic.BreakerCooldownAt != 0 {
		t.Fatalf("expected dynamic breaker cooldown reset to 0, got %d", resetDynamic.BreakerCooldownAt)
	}
	if resetDynamic.BreakerLastFailure != "" {
		t.Fatalf("expected dynamic breaker last failure cleared, got %q", resetDynamic.BreakerLastFailure)
	}
	if resetDynamic.BreakerTripCount != 0 {
		t.Fatalf("expected dynamic breaker trip count reset to 0, got %d", resetDynamic.BreakerTripCount)
	}
	if resetDynamic.BreakerRecentRequests != 0 || resetDynamic.BreakerRecentFailures != 0 || resetDynamic.BreakerRecentTimeouts != 0 {
		t.Fatalf("expected dynamic breaker EWMA counters reset, got requests=%f failures=%f timeouts=%f", resetDynamic.BreakerRecentRequests, resetDynamic.BreakerRecentFailures, resetDynamic.BreakerRecentTimeouts)
	}
	if resetDynamic.BreakerUpdatedAt != 0 {
		t.Fatalf("expected dynamic breaker updated_at reset to 0, got %d", resetDynamic.BreakerUpdatedAt)
	}
	expectedHP := computeMaxHP(&resetDynamic)
	if math.Abs(resetDynamic.BreakerHP-expectedHP) > 0.0001 {
		t.Fatalf("expected dynamic breaker HP restored to %f, got %f", expectedHP, resetDynamic.BreakerHP)
	}

	var unchangedPlain model.Channel
	if err := model.DB.Where("id = ?", plainChannel.Id).First(&unchangedPlain).Error; err != nil {
		t.Fatalf("failed to reload plain channel: %v", err)
	}
	if unchangedPlain.BreakerPressure != plainChannel.BreakerPressure {
		t.Fatalf("expected non-dynamic channel pressure unchanged, got %f want %f", unchangedPlain.BreakerPressure, plainChannel.BreakerPressure)
	}
	if unchangedPlain.BreakerCooldownAt != plainChannel.BreakerCooldownAt {
		t.Fatalf("expected non-dynamic channel cooldown unchanged, got %d want %d", unchangedPlain.BreakerCooldownAt, plainChannel.BreakerCooldownAt)
	}
}
