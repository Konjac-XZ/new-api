package service

import (
	"fmt"
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
	if observationLatest.BreakerCooldownAt <= awaitingLatest.BreakerCooldownAt {
		t.Fatalf("expected observation cooldown restart to be stricter, got observation=%d awaiting=%d", observationLatest.BreakerCooldownAt, awaitingLatest.BreakerCooldownAt)
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
	twohrs := time.Now().Add(-2 * time.Hour)
	channel := &model.Channel{
		BreakerHP:        3.0,
		BreakerUpdatedAt: twohrs.Unix(),
	}
	applyHPPassiveRecovery(channel, time.Now())
	// 2 hours * 0.5 HP/hr = 1.0 recovery, capped at maxHP=10
	expected := 4.0
	if channel.BreakerHP != expected {
		t.Fatalf("expected HP=%f after 2hr passive recovery, got %f", expected, channel.BreakerHP)
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

