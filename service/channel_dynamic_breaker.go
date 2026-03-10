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
	breakerDecayWindow                    = 4 * time.Hour
	breakerMaxCooldown                    = 120 * time.Minute
	breakerMinimumCooldown                = 30 * time.Second
	breakerNormalRecoveryFactor           = 0.7
	breakerProbationRecoveryFactor        = 0.45
	breakerSlowSuccessPressure            = 0.35
	breakerProbationPenalty               = 0.75
	breakerAwaitingProbePenalty           = 4.0
	breakerProbeTestPenalty               = 2.5
	breakerProbeObservationPenalty        = 5.0
	breakerMinPressure                    = 0.05
	breakerMaxPressureContribution        = 100.0
	breakerSlowSuccessThreshold           = 15 * time.Second
	breakerPressureCooldownWeight         = 0.7
	breakerFailStreakCooldownWeight       = 1.1
	breakerFailStreakCooldownExponent     = 0.9
	breakerFailStreakCooldownCap          = 40
	breakerTripCooldownWeight             = 12.0
	breakerTripCooldownStart              = 2
	breakerTripCooldownCap                = 60
	breakerFailureRateCooldownThreshold   = 0.65
	breakerFailureRateCooldownWeight      = 25.0
	breakerFailureRateCooldownExponent    = 1.35
	breakerTimeoutRateCooldownThreshold   = 0.35
	breakerTimeoutRateCooldownWeight      = 12.0
	breakerTimeoutRateCooldownExponent    = 1.25
	breakerCooldownRateConfidenceRequests = 10.0
	breakerShortTermPenaltyMinFactor      = 0.08
	breakerShortTermPressureMinFactor     = 0.35
	breakerShortTermStreakScale           = 8.0
	breakerShortTermStreakExponent        = 1.1
	breakerShortTermRateExponent          = 1.35
	breakerShortTermHistoryExponent       = 1.35
	breakerChronicTripFloorStart          = 3
	breakerChronicTripFloorWeight         = 12.0
	breakerChronicFailureFloorThreshold   = 0.8
	breakerChronicFailureFloorWeight      = 50.0
	breakerChronicFailureFloorExponent    = 1.4
	breakerChronicStreakFloorStart        = 10
	breakerChronicStreakFloorWeight       = 5.0
	breakerChronicStreakFloorExponent     = 0.75
)

// HP system constants — controls whether cooldown triggers
const (
	hpBase                          = 10.0      // base max HP before coefficient
	hpMinCoefficient                = 0.1       // minimum tolerance coefficient
	hpMaxCoefficient                = 10.0      // maximum tolerance coefficient
	hpDefaultCoefficient            = 1.0       // default when not configured
	hpMinimum                       = 1.0       // minimum maxHP floor
	hpPassiveRecoveryPerHour        = 0.5       // HP recovered per hour passively
	hpSuccessRecovery               = 1.0       // HP recovered per successful request
	hpProbationSuccessRecovery      = 0.8       // HP recovered per success during observation
	hpProbationDamageMultiplier     = 1.5       // damage multiplier during observation
	hpAwaitingProbeDamageMultiplier = 2.0       // damage multiplier when awaiting probe
	hpEWMADecayWindow               = time.Hour // EWMA decay time constant
	hpEWMAMinValue                  = 0.01      // minimum EWMA value before zeroing
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

		multiplier := computeBreakerCooldownMultiplier(current, wasInProbation, wasAwaitingProbe)
		baseCooldown := failureBaseCooldown(failureKind)
		cooldown := time.Duration(float64(baseCooldown) * multiplier)
		if chronicFloor := computeBreakerChronicCooldownFloor(current); chronicFloor > cooldown {
			cooldown = chronicFloor
		}
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
// apply HP damage and always restart cooldown, because a failed probe means the
// channel is still not ready to serve traffic.
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

	if wasAwaitingProbe || wasInProbation || current.BreakerHP <= 0 {
		// Any failed probe must restart cooldown. Probe failures represent an
		// explicit readiness check failure, so keep the channel fully suppressed.
		current.BreakerHP = 0
		current.BreakerTripCount++

		multiplier := computeBreakerCooldownMultiplier(current, wasInProbation, wasAwaitingProbe)
		if wasInProbation {
			multiplier += 1.75
		}
		baseCooldown := failureBaseCooldown(failureKind)
		cooldown := time.Duration(float64(baseCooldown) * multiplier)
		if chronicFloor := computeBreakerChronicCooldownFloor(current); chronicFloor > cooldown {
			cooldown = chronicFloor
		}
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

func computeBreakerFailureRates(channel *model.Channel) (float64, float64, float64) {
	if channel == nil || channel.BreakerRecentRequests <= 0 {
		return 0, 0, 0
	}
	requests := channel.BreakerRecentRequests
	failureRate := math.Min(math.Max(channel.BreakerRecentFailures/requests, 0), 1)
	timeoutRate := math.Min(math.Max(channel.BreakerRecentTimeouts/requests, 0), 1)
	confidence := math.Min(requests/breakerCooldownRateConfidenceRequests, 1.0)
	return failureRate, timeoutRate, confidence
}

func computeBreakerShortTermPenaltyFactor(channel *model.Channel) float64 {
	if channel == nil {
		return 1.0
	}

	streakInstability := 0.0
	if channel.BreakerFailStreak > 0 {
		streakInstability = math.Min(
			math.Pow(float64(channel.BreakerFailStreak)/breakerShortTermStreakScale, breakerShortTermStreakExponent),
			1.0,
		)
	}

	failureRate, timeoutRate, confidence := computeBreakerFailureRates(channel)
	if confidence <= 0 {
		if streakInstability <= 0 {
			return 1.0
		}
		return breakerShortTermPenaltyMinFactor + (1.0-breakerShortTermPenaltyMinFactor)*streakInstability
	}

	instability := streakInstability
	if failureRate > 0 {
		instability = math.Max(instability, math.Pow(failureRate, breakerShortTermRateExponent))
	}
	if timeoutRate > 0 {
		instability = math.Max(instability, math.Pow(timeoutRate, breakerShortTermRateExponent)*0.75)
	}

	blendedInstability := (1.0 - confidence) + confidence*instability
	factor := breakerShortTermPenaltyMinFactor + (1.0-breakerShortTermPenaltyMinFactor)*blendedInstability
	if factor < breakerShortTermPenaltyMinFactor {
		return breakerShortTermPenaltyMinFactor
	}
	if factor > 1.0 {
		return 1.0
	}
	return factor
}

func computeBreakerCooldownMultiplier(channel *model.Channel, wasInProbation bool, wasAwaitingProbe bool) float64 {
	if channel == nil {
		return 1.0
	}

	multiplier := 1.0
	shortTermPenaltyFactor := computeBreakerShortTermPenaltyFactor(channel)
	pressurePenaltyFactor := breakerShortTermPressureMinFactor + (1.0-breakerShortTermPressureMinFactor)*shortTermPenaltyFactor
	historyPenaltyFactor := math.Pow(shortTermPenaltyFactor, breakerShortTermHistoryExponent)
	multiplier += math.Min(channel.BreakerPressure, breakerMaxPressureContribution) * breakerPressureCooldownWeight * pressurePenaltyFactor

	if channel.BreakerFailStreak > 1 {
		streakPenalty := math.Pow(
			float64(minInt(channel.BreakerFailStreak-1, breakerFailStreakCooldownCap)),
			breakerFailStreakCooldownExponent,
		) * breakerFailStreakCooldownWeight
		multiplier += streakPenalty
	}

	if channel.BreakerTripCount > breakerTripCooldownStart {
		tripPenalty := math.Log1p(float64(minInt(channel.BreakerTripCount-breakerTripCooldownStart, breakerTripCooldownCap))) * breakerTripCooldownWeight * historyPenaltyFactor
		multiplier += tripPenalty
	}

	failureRate, timeoutRate, confidence := computeBreakerFailureRates(channel)
	if confidence > 0 {
		if failureRate > breakerFailureRateCooldownThreshold {
			normalized := (failureRate - breakerFailureRateCooldownThreshold) / (1.0 - breakerFailureRateCooldownThreshold)
			multiplier += math.Pow(normalized, breakerFailureRateCooldownExponent) * breakerFailureRateCooldownWeight * confidence
		}
		if timeoutRate > breakerTimeoutRateCooldownThreshold {
			normalized := (timeoutRate - breakerTimeoutRateCooldownThreshold) / (1.0 - breakerTimeoutRateCooldownThreshold)
			multiplier += math.Pow(normalized, breakerTimeoutRateCooldownExponent) * breakerTimeoutRateCooldownWeight * confidence
		}
	}

	if wasInProbation {
		multiplier += 0.75
	}
	if wasAwaitingProbe {
		multiplier += 1.5
	}
	if multiplier < 1.0 {
		return 1.0
	}
	return multiplier
}

func computeBreakerChronicCooldownFloor(channel *model.Channel) time.Duration {
	if channel == nil {
		return 0
	}

	floor := time.Duration(0)
	shortTermPenaltyFactor := computeBreakerShortTermPenaltyFactor(channel)
	historyPenaltyFactor := math.Pow(shortTermPenaltyFactor, breakerShortTermHistoryExponent)
	if channel.BreakerTripCount > breakerChronicTripFloorStart {
		floor += time.Duration(
			math.Log1p(float64(minInt(channel.BreakerTripCount-breakerChronicTripFloorStart, breakerTripCooldownCap))) *
				breakerChronicTripFloorWeight * historyPenaltyFactor * float64(time.Minute),
		)
	}

	failureRate, _, confidence := computeBreakerFailureRates(channel)
	if confidence > 0 && failureRate > breakerChronicFailureFloorThreshold {
		normalized := (failureRate - breakerChronicFailureFloorThreshold) / (1.0 - breakerChronicFailureFloorThreshold)
		floor += time.Duration(
			math.Pow(normalized, breakerChronicFailureFloorExponent) *
				breakerChronicFailureFloorWeight *
				confidence * float64(time.Minute),
		)
	}

	if channel.BreakerFailStreak > breakerChronicStreakFloorStart {
		floor += time.Duration(
			math.Pow(
				float64(minInt(channel.BreakerFailStreak-breakerChronicStreakFloorStart, breakerFailStreakCooldownCap)),
				breakerChronicStreakFloorExponent,
			) * breakerChronicStreakFloorWeight * float64(time.Minute),
		)
	}

	if floor > breakerMaxCooldown {
		return breakerMaxCooldown
	}
	return floor
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
