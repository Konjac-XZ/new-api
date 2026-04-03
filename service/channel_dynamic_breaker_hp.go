package service

import (
	"fmt"
	"math"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

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

	// Consecutive failure penalty: 1/(1 + streak*0.08)
	streakFactor := 1.0 / (1.0 + float64(channel.BreakerFailStreak)*0.08)

	successRewardFactor := computeHPSuccessRewardFactor(channel)

	maxHP := hpBase * coeff * failureRateFactor * timeoutRateFactor * tripFactor * streakFactor * successRewardFactor
	return math.Max(maxHP, hpMinimum)
}

func computeHPSuccessRewardFactor(channel *model.Channel) float64 {
	if channel == nil {
		return 1.0
	}
	requests := math.Max(channel.BreakerRecentRequests, 0)
	if requests <= 0 {
		return 1.0
	}
	if channel.BreakerRecentFailures > 0 {
		return 1.0
	}

	recentSuccesses := math.Max(channel.BreakerRecentRequests-channel.BreakerRecentFailures, 0)
	if recentSuccesses <= 0 {
		return 1.0
	}

	volumeFactor := math.Min(recentSuccesses/hpSuccessRewardConfidence, 1.0)
	if volumeFactor <= 0 {
		return 1.0
	}

	return 1.0 + hpSuccessRewardMaxBonus*volumeFactor
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

// ResetAllDynamicChannelBreakers clears breaker cooldown/history for every
// channel with dynamic circuit breaker enabled and restores HP to full.
func ResetAllDynamicChannelBreakers() (int, int, error) {
	channels, err := model.GetAllChannels(0, 0, true, false)
	if err != nil {
		return 0, 0, err
	}

	successCount := 0
	failCount := 0
	for _, channel := range channels {
		if channel == nil || !channel.IsDynamicCircuitBreakerEnabled() {
			continue
		}

		lock := getChannelBreakerLock(channel.Id)
		lock.Lock()

		channel.BreakerPressure = 0
		channel.BreakerUpdatedAt = 0
		channel.BreakerFailStreak = 0
		channel.BreakerCooldownAt = 0
		channel.BreakerLastFailure = ""
		channel.BreakerTripCount = 0
		channel.BreakerRecentRequests = 0
		channel.BreakerRecentFailures = 0
		channel.BreakerRecentTimeouts = 0
		channel.BreakerHP = computeMaxHP(channel)

		if updateErr := model.UpdateChannelBreakerState(channel); updateErr != nil {
			lock.Unlock()
			failCount++
			common.SysLog(fmt.Sprintf("failed to reset channel breaker state: channel_id=%d, error=%v", channel.Id, updateErr))
			continue
		}
		channelBreakerWorkingState.Store(channel.Id, snapshotChannelBreakerState(channel))
		lock.Unlock()
		successCount++
	}

	return successCount, failCount, nil
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
