package service

import (
	"fmt"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
)

type channelFailureKind string

const (
	channelFailureKindGeneric           channelFailureKind = "generic"
	channelFailureKindImmediateFailure  channelFailureKind = "immediate_failure"
	channelFailureKindFirstTokenTimeout channelFailureKind = "first_token_timeout"
	channelFailureKindMidStreamFailure  channelFailureKind = "mid_stream_failure"
	channelFailureKindOverloaded        channelFailureKind = "overloaded"
	channelFailureKindEmptyReply        channelFailureKind = "empty_reply"
)

// Pressure system constants (unchanged — controls cooldown duration)
const (
	breakerDecayWindow             = time.Hour
	breakerMaxCooldown             = 30 * time.Minute
	breakerMinimumCooldown         = 30 * time.Second
	breakerNormalRecoveryFactor    = 0.7
	breakerProbationRecoveryFactor = 0.45
	breakerSlowSuccessPressure     = 0.35
	breakerProbationPenalty        = 0.75
	breakerAwaitingProbePenalty    = 4.0
	breakerProbeTestPenalty        = 2.5
	breakerProbeObservationPenalty = 5.0
	breakerMinPressure             = 0.05
	breakerMaxPressureContribution = 8.0
	breakerSlowSuccessThreshold    = 15 * time.Second
)

// HP system constants — controls whether cooldown triggers
const (
	hpBase                       = 10.0  // base max HP before coefficient
	hpMinCoefficient             = 0.1   // minimum tolerance coefficient
	hpMaxCoefficient             = 10.0  // maximum tolerance coefficient
	hpDefaultCoefficient         = 1.0   // default when not configured
	hpMinimum                    = 1.0   // minimum maxHP floor
	hpPassiveRecoveryPerHour     = 0.5   // HP recovered per hour passively
	hpSuccessRecovery            = 0.5   // HP recovered per successful request
	hpProbationSuccessRecovery   = 0.8   // HP recovered per success during observation
	hpProbationDamageMultiplier  = 1.5   // damage multiplier during observation
	hpAwaitingProbeDamageMultiplier = 2.0 // damage multiplier when awaiting probe
	hpEWMADecayWindow            = time.Hour // EWMA decay time constant
	hpEWMAMinValue               = 0.01 // minimum EWMA value before zeroing
)

var channelBreakerStateLock sync.Mutex

func GetDynamicSuppressedChannelIDs(group string, modelName string) (map[int]bool, error) {
	channels, err := model.GetEnabledChannelsByGroupModel(group, modelName)
	if err != nil {
		return nil, err
	}
	if len(channels) == 0 {
		return nil, nil
	}
	now := time.Now().Unix()
	exclude := make(map[int]bool)
	for _, channel := range channels {
		if channel == nil {
			continue
		}
		if !channel.IsBreakerCoolingAt(now) && !channel.IsBreakerAwaitingProbeAt(now) {
			continue
		}
		exclude[channel.Id] = true
	}
	if len(exclude) == 0 {
		return nil, nil
	}
	return exclude, nil
}

func RecordChannelRelaySuccess(channel *model.Channel, info *relaycommon.RelayInfo) {
	if channel == nil {
		return
	}
	channelBreakerStateLock.Lock()
	defer channelBreakerStateLock.Unlock()

	current, err := model.GetChannelById(channel.Id, true)
	if err != nil {
		common.SysLog(fmt.Sprintf("failed to load channel breaker state on success: channel_id=%d, error=%v", channel.Id, err))
		return
	}
	if !current.IsDynamicCircuitBreakerEnabled() {
		return
	}

	now := time.Now()
	wasInProbation := current.IsBreakerProbationAt(now.Unix())

	// Pressure system (unchanged)
	applyBreakerDecay(current, now)
	current.BreakerFailStreak = 0
	current.BreakerCooldownAt = 0
	current.BreakerLastFailure = ""

	if isSlowSuccessfulRequest(current, info) {
		current.BreakerPressure += breakerSlowSuccessPressure
	} else {
		recoveryFactor := breakerNormalRecoveryFactor
		if wasInProbation {
			recoveryFactor = breakerProbationRecoveryFactor
		}
		current.BreakerPressure *= recoveryFactor
	}
	if current.BreakerPressure < breakerMinPressure {
		current.BreakerPressure = 0
	}

	// EWMA tracking: record success event
	applyEWMADecay(current, now)
	current.BreakerRecentRequests += 1.0

	// HP system
	ensureHPInitialized(current)
	applyHPPassiveRecovery(current, now)

	if wasInProbation {
		// Transitioning out of observation: refill HP to maxHP
		maxHP := computeMaxHP(current)
		current.BreakerHP = maxHP
		if current.BreakerTripCount > 0 {
			current.BreakerTripCount--
		}
	} else {
		recovery := hpSuccessRecovery
		maxHP := computeMaxHP(current)
		current.BreakerHP = math.Min(current.BreakerHP+recovery, maxHP)
	}

	current.BreakerUpdatedAt = now.Unix()

	if err := model.UpdateChannelBreakerState(current); err != nil {
		common.SysLog(fmt.Sprintf("failed to persist channel breaker success state: channel_id=%d, error=%v", channel.Id, err))
	}
}

func RecordChannelRelayFailure(channel *model.Channel, info *relaycommon.RelayInfo, err *types.NewAPIError) {
	if channel == nil || err == nil {
		return
	}
	channelBreakerStateLock.Lock()
	defer channelBreakerStateLock.Unlock()

	current, loadErr := model.GetChannelById(channel.Id, true)
	if loadErr != nil {
		common.SysLog(fmt.Sprintf("failed to load channel breaker state on failure: channel_id=%d, error=%v", channel.Id, loadErr))
		return
	}
	if !current.IsDynamicCircuitBreakerEnabled() {
		return
	}

	now := time.Now()
	wasInProbation := current.IsBreakerProbationAt(now.Unix())
	wasAwaitingProbe := current.IsBreakerAwaitingProbeAt(now.Unix())

	// Pressure system (unchanged — still accumulates for cooldown duration calculation)
	applyBreakerDecay(current, now)

	failureKind := classifyChannelFailure(info, err)
	current.BreakerPressure += failurePressureWeight(failureKind)
	if wasInProbation {
		current.BreakerPressure += breakerProbationPenalty
	}
	if wasAwaitingProbe {
		current.BreakerPressure += breakerAwaitingProbePenalty
	}
	current.BreakerFailStreak++

	// EWMA tracking: record failure event
	applyEWMADecay(current, now)
	current.BreakerRecentRequests += 1.0
	current.BreakerRecentFailures += 1.0
	if failureKind == channelFailureKindFirstTokenTimeout {
		current.BreakerRecentTimeouts += 1.0
	}

	// HP system: apply damage
	ensureHPInitialized(current)
	applyHPPassiveRecovery(current, now)

	damage := failurePressureWeight(failureKind)
	if wasInProbation {
		damage *= hpProbationDamageMultiplier
	}
	if wasAwaitingProbe {
		damage *= hpAwaitingProbeDamageMultiplier
	}
	current.BreakerHP -= damage

	if current.BreakerHP <= 0 {
		// HP depleted: trigger cooldown
		current.BreakerHP = 0
		current.BreakerTripCount++

		// Calculate cooldown duration using existing pressure-based formula
		multiplier := 1.0 + math.Min(current.BreakerPressure, breakerMaxPressureContribution)*0.5
		if current.BreakerFailStreak > 1 {
			multiplier += float64(minInt(current.BreakerFailStreak-1, 6)) * 0.75
		}
		if wasInProbation {
			multiplier += 0.75
		}
		if wasAwaitingProbe {
			multiplier += 1.5
		}

		baseCooldown := failureBaseCooldown(failureKind)
		cooldown := time.Duration(float64(baseCooldown) * multiplier)
		if cooldown < breakerMinimumCooldown {
			cooldown = breakerMinimumCooldown
		}
		if cooldown > breakerMaxCooldown {
			cooldown = breakerMaxCooldown
		}

		current.BreakerCooldownAt = now.Add(cooldown).Unix()
	}
	// If HP > 0: no cooldown triggered, channel remains in service

	current.BreakerLastFailure = string(failureKind)
	current.BreakerUpdatedAt = now.Unix()

	if err := model.UpdateChannelBreakerState(current); err != nil {
		common.SysLog(fmt.Sprintf("failed to persist channel breaker failure state: channel_id=%d, error=%v", channel.Id, err))
	}
}

// RecordChannelProbeFailure applies stage-aware handling for scheduled probe failures.
// Cooling phase failures are ignored. Awaiting-probe and observation failures both
// apply HP damage and may restart cooldown if HP depletes.
func RecordChannelProbeFailure(channel *model.Channel, err *types.NewAPIError) {
	if channel == nil || err == nil {
		return
	}
	channelBreakerStateLock.Lock()
	defer channelBreakerStateLock.Unlock()

	current, loadErr := model.GetChannelById(channel.Id, true)
	if loadErr != nil {
		common.SysLog(fmt.Sprintf("failed to load channel breaker state on probe failure: channel_id=%d, error=%v", channel.Id, loadErr))
		return
	}
	if !current.IsDynamicCircuitBreakerEnabled() {
		return
	}

	now := time.Now()
	nowUnix := now.Unix()
	if current.IsBreakerCoolingAt(nowUnix) {
		// During active cooldown, probe failures do not change breaker state.
		return
	}

	wasAwaitingProbe := current.IsBreakerAwaitingProbeAt(nowUnix)
	wasInProbation := current.IsBreakerProbationAt(nowUnix)

	// Pressure system (unchanged)
	applyBreakerDecay(current, now)

	failureKind := classifyChannelFailure(nil, err)
	current.BreakerPressure += failurePressureWeight(failureKind)
	current.BreakerFailStreak++

	switch {
	case wasInProbation:
		current.BreakerPressure += breakerProbeObservationPenalty
	case wasAwaitingProbe:
		current.BreakerPressure += breakerProbeTestPenalty
	}

	// EWMA tracking: record failure event
	applyEWMADecay(current, now)
	current.BreakerRecentRequests += 1.0
	current.BreakerRecentFailures += 1.0
	if failureKind == channelFailureKindFirstTokenTimeout {
		current.BreakerRecentTimeouts += 1.0
	}

	// HP system: apply damage
	ensureHPInitialized(current)
	applyHPPassiveRecovery(current, now)

	damage := failurePressureWeight(failureKind)
	if wasInProbation {
		damage *= hpProbationDamageMultiplier
	}
	if wasAwaitingProbe {
		damage *= hpAwaitingProbeDamageMultiplier
	}
	current.BreakerHP -= damage

	if current.BreakerHP <= 0 {
		// HP depleted: trigger cooldown
		current.BreakerHP = 0
		current.BreakerTripCount++

		multiplier := 1.0 + math.Min(current.BreakerPressure, breakerMaxPressureContribution)*0.5
		if current.BreakerFailStreak > 1 {
			multiplier += float64(minInt(current.BreakerFailStreak-1, 6)) * 0.75
		}

		switch {
		case wasInProbation:
			multiplier += 2.5
		case wasAwaitingProbe:
			multiplier += 1.5
		}

		baseCooldown := failureBaseCooldown(failureKind)
		cooldown := time.Duration(float64(baseCooldown) * multiplier)
		if cooldown < breakerMinimumCooldown {
			cooldown = breakerMinimumCooldown
		}
		if cooldown > breakerMaxCooldown {
			cooldown = breakerMaxCooldown
		}

		current.BreakerCooldownAt = now.Add(cooldown).Unix()
	}

	current.BreakerLastFailure = string(failureKind)
	current.BreakerUpdatedAt = nowUnix

	if err := model.UpdateChannelBreakerState(current); err != nil {
		common.SysLog(fmt.Sprintf("failed to persist channel breaker probe failure state: channel_id=%d, error=%v", channel.Id, err))
	}
}

// RecordChannelProbeSuccess promotes an awaiting-probe channel into probation.
// On promotion, HP is refilled to maxHP and trip count is decremented.
func RecordChannelProbeSuccess(channel *model.Channel) bool {
	if channel == nil {
		return false
	}
	channelBreakerStateLock.Lock()
	defer channelBreakerStateLock.Unlock()

	current, err := model.GetChannelById(channel.Id, true)
	if err != nil {
		common.SysLog(fmt.Sprintf("failed to load channel breaker state on probe success: channel_id=%d, error=%v", channel.Id, err))
		return false
	}
	if !current.IsDynamicCircuitBreakerEnabled() {
		return false
	}

	now := time.Now()
	nowUnix := now.Unix()
	isAwaitingProbe := current.IsBreakerAwaitingProbeAt(nowUnix)
	if current.BreakerCooldownAt <= 0 || current.IsBreakerProbationAt(nowUnix) {
		return false
	}
	// Never allow probe success to fast-forward the stress-index cooldown.
	// Promotion is only valid after cooldown expires (awaiting probe phase).
	if !isAwaitingProbe {
		return false
	}
	applyBreakerDecay(current, now)

	// HP system: refill on entering observation phase
	ensureHPInitialized(current)
	maxHP := computeMaxHP(current)
	current.BreakerHP = maxHP
	if current.BreakerTripCount > 0 {
		current.BreakerTripCount--
	}

	current.BreakerCooldownAt = nowUnix
	current.BreakerUpdatedAt = nowUnix
	if err := model.UpdateChannelBreakerState(current); err != nil {
		common.SysLog(fmt.Sprintf("failed to persist channel breaker probe success state: channel_id=%d, error=%v", channel.Id, err))
		return false
	}
	return true
}

// --- HP system helpers ---

// getToleranceCoefficient returns the channel's configured tolerance coefficient,
// clamped to [hpMinCoefficient, hpMaxCoefficient]. Returns hpDefaultCoefficient if not set.
func getToleranceCoefficient(channel *model.Channel) float64 {
	if channel == nil {
		return hpDefaultCoefficient
	}
	setting := channel.GetSetting()
	if setting.ToleranceCoefficient == nil {
		return hpDefaultCoefficient
	}
	coeff := *setting.ToleranceCoefficient
	if coeff < hpMinCoefficient {
		coeff = hpMinCoefficient
	}
	if coeff > hpMaxCoefficient {
		coeff = hpMaxCoefficient
	}
	return coeff
}

// computeMaxHP calculates the dynamic maximum HP for a channel based on
// tolerance coefficient and reliability metrics.
func computeMaxHP(channel *model.Channel) float64 {
	if channel == nil {
		return hpBase
	}
	coeff := getToleranceCoefficient(channel)

	// Failure rate penalty: maps failure_rate [0, 1] to factor [0.5, 1.0]
	failureRate := 0.0
	if channel.BreakerRecentRequests >= 1.0 {
		failureRate = channel.BreakerRecentFailures / channel.BreakerRecentRequests
	}
	failureRateFactor := 1.0 - math.Min(failureRate, 1.0)*0.5

	// Timeout rate penalty: maps timeout_rate [0, 1] to factor [0.7, 1.0]
	timeoutRate := 0.0
	if channel.BreakerRecentRequests >= 1.0 {
		timeoutRate = channel.BreakerRecentTimeouts / channel.BreakerRecentRequests
	}
	timeoutRateFactor := 1.0 - math.Min(timeoutRate, 1.0)*0.3

	// Trip count penalty: diminishing — 1/(1 + trips*0.15)
	tripFactor := 1.0 / (1.0 + float64(channel.BreakerTripCount)*0.15)

	// Consecutive failure penalty: 1/(1 + streak*0.05)
	streakFactor := 1.0 / (1.0 + float64(channel.BreakerFailStreak)*0.05)

	maxHP := hpBase * coeff * failureRateFactor * timeoutRateFactor * tripFactor * streakFactor
	return math.Max(maxHP, hpMinimum)
}

// ensureHPInitialized sets HP to maxHP if uninitialized (BreakerHP == -1).
func ensureHPInitialized(channel *model.Channel) {
	if channel == nil {
		return
	}
	if channel.BreakerHP < 0 {
		channel.BreakerHP = computeMaxHP(channel)
	}
}

// applyHPPassiveRecovery applies time-based passive HP recovery.
// Uses BreakerUpdatedAt as the reference timestamp.
func applyHPPassiveRecovery(channel *model.Channel, now time.Time) {
	if channel == nil || channel.BreakerUpdatedAt <= 0 {
		return
	}
	elapsed := now.Sub(time.Unix(channel.BreakerUpdatedAt, 0))
	if elapsed <= 0 {
		return
	}
	recovery := (elapsed.Seconds() / 3600.0) * hpPassiveRecoveryPerHour
	maxHP := computeMaxHP(channel)
	channel.BreakerHP = math.Min(channel.BreakerHP+recovery, maxHP)
}

// --- EWMA helpers ---

// applyEWMADecay applies exponential decay to EWMA counters based on elapsed time.
func applyEWMADecay(channel *model.Channel, now time.Time) {
	if channel == nil || channel.BreakerUpdatedAt <= 0 {
		return
	}
	elapsed := now.Sub(time.Unix(channel.BreakerUpdatedAt, 0))
	if elapsed <= 0 {
		return
	}
	decayFactor := math.Exp(-elapsed.Seconds() / hpEWMADecayWindow.Seconds())
	channel.BreakerRecentRequests *= decayFactor
	channel.BreakerRecentFailures *= decayFactor
	channel.BreakerRecentTimeouts *= decayFactor

	if channel.BreakerRecentRequests < hpEWMAMinValue {
		channel.BreakerRecentRequests = 0
	}
	if channel.BreakerRecentFailures < hpEWMAMinValue {
		channel.BreakerRecentFailures = 0
	}
	if channel.BreakerRecentTimeouts < hpEWMAMinValue {
		channel.BreakerRecentTimeouts = 0
	}
}

// ChannelBreakerHPInfo holds HP-related state for external consumers (e.g., controller API).
type ChannelBreakerHPInfo struct {
	HP                   float64
	MaxHP                float64
	TripCount            int
	ToleranceCoefficient float64
	FailureRate          float64
	TimeoutRate          float64
}

// GetChannelBreakerHPInfo returns the current HP state for a channel.
// Safe to call on nil channel — returns sensible defaults.
func GetChannelBreakerHPInfo(channel *model.Channel) ChannelBreakerHPInfo {
	if channel == nil {
		return ChannelBreakerHPInfo{HP: hpBase, MaxHP: hpBase, ToleranceCoefficient: hpDefaultCoefficient}
	}
	maxHP := computeMaxHP(channel)
	hp := channel.BreakerHP
	if hp < 0 {
		hp = maxHP
	}
	reqs := math.Max(channel.BreakerRecentRequests, 1.0)
	return ChannelBreakerHPInfo{
		HP:                   hp,
		MaxHP:                maxHP,
		TripCount:            channel.BreakerTripCount,
		ToleranceCoefficient: getToleranceCoefficient(channel),
		FailureRate:          channel.BreakerRecentFailures / reqs,
		TimeoutRate:          channel.BreakerRecentTimeouts / reqs,
	}
}

// --- Pressure system helpers (unchanged) ---

func applyBreakerDecay(channel *model.Channel, now time.Time) {
	if channel == nil {
		return
	}
	if channel.BreakerUpdatedAt <= 0 {
		channel.BreakerUpdatedAt = now.Unix()
		return
	}
	if channel.BreakerPressure <= 0 {
		channel.BreakerPressure = 0
		channel.BreakerUpdatedAt = now.Unix()
		return
	}
	elapsed := now.Sub(time.Unix(channel.BreakerUpdatedAt, 0))
	if elapsed <= 0 {
		return
	}
	decayFactor := math.Exp(-elapsed.Seconds() / breakerDecayWindow.Seconds())
	channel.BreakerPressure *= decayFactor
	if channel.BreakerPressure < breakerMinPressure {
		channel.BreakerPressure = 0
	}
	channel.BreakerUpdatedAt = now.Unix()
}

func classifyChannelFailure(info *relaycommon.RelayInfo, err *types.NewAPIError) channelFailureKind {
	if err == nil {
		return channelFailureKindGeneric
	}
	switch err.GetErrorCode() {
	case types.ErrorCodeChannelFirstTokenLatencyExceeded, types.ErrorCodeChannelResponseTimeExceeded:
		return channelFailureKindFirstTokenTimeout
	case types.ErrorCodeChannelEmptyReply:
		return channelFailureKindEmptyReply
	}
	if info != nil && info.HasSendResponse() {
		return channelFailureKindMidStreamFailure
	}
	switch err.StatusCode {
	case http.StatusTooManyRequests, http.StatusServiceUnavailable, http.StatusBadGateway:
		return channelFailureKindOverloaded
	case http.StatusGatewayTimeout, http.StatusRequestTimeout:
		return channelFailureKindImmediateFailure
	}
	if err.StatusCode == 0 || err.StatusCode < 100 || err.StatusCode > 599 || err.StatusCode >= 500 {
		return channelFailureKindImmediateFailure
	}
	if types.IsChannelError(err) {
		return channelFailureKindImmediateFailure
	}
	return channelFailureKindGeneric
}

func failurePressureWeight(kind channelFailureKind) float64 {
	switch kind {
	case channelFailureKindFirstTokenTimeout:
		return 3.0
	case channelFailureKindMidStreamFailure:
		return 2.5
	case channelFailureKindImmediateFailure:
		return 2.0
	case channelFailureKindEmptyReply:
		return 1.8
	case channelFailureKindOverloaded:
		return 1.0
	default:
		return 1.4
	}
}

func failureBaseCooldown(kind channelFailureKind) time.Duration {
	switch kind {
	case channelFailureKindFirstTokenTimeout:
		return 90 * time.Second
	case channelFailureKindMidStreamFailure:
		return 75 * time.Second
	case channelFailureKindImmediateFailure:
		return 60 * time.Second
	case channelFailureKindEmptyReply:
		return 50 * time.Second
	case channelFailureKindOverloaded:
		return 35 * time.Second
	default:
		return 45 * time.Second
	}
}

func isSlowSuccessfulRequest(channel *model.Channel, info *relaycommon.RelayInfo) bool {
	if channel == nil || info == nil {
		return false
	}
	if info.IsStream && info.HasSendResponse() {
		firstTokenLatency := info.FirstResponseTime.Sub(info.StartTime)
		if maxLatency := channel.GetMaxFirstTokenLatency(); maxLatency > 0 {
			threshold := time.Duration(maxLatency) * time.Second * 4 / 5
			return firstTokenLatency >= threshold
		}
		return firstTokenLatency >= breakerSlowSuccessThreshold
	}
	return time.Since(info.StartTime) >= breakerSlowSuccessThreshold
}

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}
