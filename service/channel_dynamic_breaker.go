package service

import (
	"fmt"
	"math"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
)

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
		// Only suppress channels that are actively cooling.
		// Awaiting-probe channels are allowed into routing as live canaries:
		// a success promotes them to probation, enabling faster recovery
		// without requiring a dedicated scheduled probe.
		if !channel.IsBreakerCoolingAt(now) {
			continue
		}
		exclude[channel.Id] = true
	}
	if len(exclude) == 0 {
		return nil, nil
	}
	return exclude, nil
}

func isImplicitProbationTimeout(channel *model.Channel, info *relaycommon.RelayInfo, now time.Time) bool {
	if channel == nil || info == nil || !info.IsStream || info.HasSendResponse() || info.StartTime.IsZero() {
		return false
	}
	threshold := successLatencyThreshold(channel)
	if threshold <= 0 {
		return false
	}
	return now.Sub(info.StartTime) >= threshold
}

func ShouldRecordChannelRelaySuccessOnFirstResponse(channel *model.Channel, info *relaycommon.RelayInfo, now time.Time) bool {
	if channel == nil || info == nil || !info.IsStream || !channel.IsDynamicCircuitBreakerEnabled() {
		return false
	}
	nowUnix := now.Unix()
	return channel.IsBreakerAwaitingProbeAt(nowUnix) || channel.IsBreakerProbationAt(nowUnix)
}

func applyRelayFailureStateLocked(
	current *model.Channel,
	info *relaycommon.RelayInfo,
	now time.Time,
	failureKind channelFailureKind,
	wasInProbation bool,
	wasAwaitingProbe bool,
	forceCooldown bool,
	extraPressure float64,
	extraDamageMultiplier float64,
) {
	if current == nil {
		return
	}

	// Pressure system (unchanged — still accumulates for cooldown duration calculation)
	applyBreakerDecay(current, now)

	current.BreakerPressure += failurePressureWeight(failureKind)
	if wasInProbation {
		current.BreakerPressure += breakerProbationPenalty
	}
	if wasAwaitingProbe {
		current.BreakerPressure += breakerAwaitingProbePenalty
	}
	if extraPressure > 0 {
		current.BreakerPressure += extraPressure
	}
	current.BreakerFailStreak++

	// EWMA tracking: record failure event
	applyEWMADecay(current, now)
	current.BreakerRecentRequests += 1.0
	current.BreakerRecentFailures += 1.0
	if failureKind == channelFailureKindFirstTokenTimeout {
		current.BreakerRecentTimeouts += 1.0
	}

	// HP system: apply damage (uses failureHPDamage, not failurePressureWeight,
	// so overloaded failures deal reduced damage)
	ensureHPInitialized(current)
	applyHPPassiveRecovery(current, now)

	damage := failureHPDamage(failureKind) * failureDamageLatencyMultiplier(current, info, failureKind, now)
	if wasInProbation {
		damage *= hpProbationDamageMultiplier
	}
	if wasAwaitingProbe {
		damage *= hpAwaitingProbeDamageMultiplier
	}
	if extraDamageMultiplier > 1.0 {
		damage *= extraDamageMultiplier
	}
	current.BreakerHP -= damage

	if forceCooldown || current.BreakerHP <= 0 {
		// HP depleted or explicitly forced: trigger cooldown.
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
	// If HP > 0 and not force-cooling: no cooldown triggered, channel remains in service.

	current.BreakerLastFailure = string(failureKind)
	current.BreakerUpdatedAt = now.Unix()
}

func RecordChannelRelaySuccess(channel *model.Channel, info *relaycommon.RelayInfo) {
	if channel == nil {
		return
	}
	persistErrMsg := "failed to persist channel breaker success state"
	_, err := mutateChannelBreakerState(channel, func(current *model.Channel, now time.Time) bool {
		beforePhase := GetChannelBreakerPhase(current, now.Unix())
		beforeCooldownAt := current.BreakerCooldownAt
		beforeUpdatedAt := current.BreakerUpdatedAt
		wasInProbation := current.IsBreakerProbationAt(now.Unix())
		wasAwaitingProbe := current.IsBreakerAwaitingProbeAt(now.Unix())
		if wasInProbation && isImplicitProbationTimeout(current, info, now) {
			persistErrMsg = "failed to persist channel breaker implicit-timeout failure state"
			common.SysLog(fmt.Sprintf("[breaker-debug] relay success treated as implicit timeout: channel_id=%d, phase_before=%s, cooldown_at=%d, updated_at=%d, is_stream=%t, has_send_response=%t",
				channel.Id,
				beforePhase,
				beforeCooldownAt,
				beforeUpdatedAt,
				info != nil && info.IsStream,
				info != nil && info.HasSendResponse(),
			))
			applyRelayFailureStateLocked(
				current,
				info,
				now,
				channelFailureKindFirstTokenTimeout,
				wasInProbation,
				wasAwaitingProbe,
				true,
				breakerProbationSilentTimeoutPenalty,
				hpProbationSilentTimeoutDamageMultiplier,
			)
			return true
		}

		latencyClass := classifySuccessfulRequestLatency(current, info)
		applyBreakerDecay(current, now)
		current.BreakerFailStreak = 0
		current.BreakerLastFailure = ""
		current.BreakerCooldownAt = 0

		if latencyClass == channelSuccessLatencyNearTimeout {
			current.BreakerPressure += breakerSlowSuccessPressure
		} else {
			recoveryFactor := breakerNormalRecoveryFactor
			if latencyClass == channelSuccessLatencyFast {
				recoveryFactor = breakerFastSuccessPressureFactor
			} else if wasInProbation || wasAwaitingProbe {
				recoveryFactor = breakerProbationRecoveryFactor
			}
			current.BreakerPressure *= recoveryFactor
		}
		if current.BreakerPressure < breakerMinPressure {
			current.BreakerPressure = 0
		}

		applyEWMADecay(current, now)
		current.BreakerRecentRequests += 1.0

		ensureHPInitialized(current)
		applyHPPassiveRecovery(current, now)

		if wasInProbation {
			maxHP := computeMaxHP(current)
			refillTarget := maxHP * probationSuccessRefillFraction(latencyClass)
			if current.BreakerHP < refillTarget {
				current.BreakerHP = refillTarget
			}
			if current.BreakerTripCount > 0 {
				current.BreakerTripCount--
			}
		} else if wasAwaitingProbe {
			maxHP := computeMaxHP(current)
			current.BreakerHP = maxHP * awaitingProbeSuccessRefillFraction(latencyClass)
			if current.BreakerTripCount > 0 {
				current.BreakerTripCount--
			}
		} else {
			recovery := hpSuccessRecovery
			if latencyClass == channelSuccessLatencyFast {
				recovery += hpFastSuccessRecoveryBonus
			}
			maxHP := computeMaxHP(current)
			current.BreakerHP = math.Min(current.BreakerHP+recovery, maxHP)
		}

		current.BreakerUpdatedAt = now.Unix()
		afterPhase := GetChannelBreakerPhase(current, now.Unix())
		common.SysLog(fmt.Sprintf("[breaker-debug] relay success state transition: channel_id=%d, phase_before=%s, phase_after=%s, was_probation=%t, was_awaiting_probe=%t, latency_class=%d, cooldown_at_before=%d, cooldown_at_after=%d, updated_at_before=%d, updated_at_after=%d, has_send_response=%t",
			channel.Id,
			beforePhase,
			afterPhase,
			wasInProbation,
			wasAwaitingProbe,
			latencyClass,
			beforeCooldownAt,
			current.BreakerCooldownAt,
			beforeUpdatedAt,
			current.BreakerUpdatedAt,
			info != nil && info.HasSendResponse(),
		))
		return true
	})
	if err != nil {
		common.SysLog(fmt.Sprintf("%s: channel_id=%d, error=%v", persistErrMsg, channel.Id, err))
	}
}

func RecordChannelRelayFailure(channel *model.Channel, info *relaycommon.RelayInfo, err *types.NewAPIError) {
	if channel == nil || err == nil {
		return
	}
	_, persistErr := mutateChannelBreakerState(channel, func(current *model.Channel, now time.Time) bool {
		wasInProbation := current.IsBreakerProbationAt(now.Unix())
		wasAwaitingProbe := current.IsBreakerAwaitingProbeAt(now.Unix())
		failureKind := classifyChannelFailure(info, err)
		forceCooldown := wasInProbation
		applyRelayFailureStateLocked(current, info, now, failureKind, wasInProbation, wasAwaitingProbe, forceCooldown, 0, 1.0)
		return true
	})
	if persistErr != nil {
		common.SysLog(fmt.Sprintf("failed to persist channel breaker failure state: channel_id=%d, error=%v", channel.Id, persistErr))
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
	_, persistErr := mutateChannelBreakerState(channel, func(current *model.Channel, now time.Time) bool {
		nowUnix := now.Unix()
		if current.IsBreakerCoolingAt(nowUnix) {
			return false
		}

		wasAwaitingProbe := current.IsBreakerAwaitingProbeAt(nowUnix)
		wasInProbation := current.IsBreakerProbationAt(nowUnix)
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

		applyEWMADecay(current, now)
		current.BreakerRecentRequests += 1.0
		current.BreakerRecentFailures += 1.0
		if failureKind == channelFailureKindFirstTokenTimeout {
			current.BreakerRecentTimeouts += 1.0
		}

		ensureHPInitialized(current)
		applyHPPassiveRecovery(current, now)

		damage := failureHPDamage(failureKind) * failureDamageLatencyMultiplier(current, nil, failureKind, now)
		if wasInProbation {
			damage *= hpProbationDamageMultiplier
		}
		if wasAwaitingProbe {
			damage *= hpAwaitingProbeDamageMultiplier
		}
		current.BreakerHP -= damage

		if wasAwaitingProbe || wasInProbation || current.BreakerHP <= 0 {
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
		return true
	})
	if persistErr != nil {
		common.SysLog(fmt.Sprintf("failed to persist channel breaker probe failure state: channel_id=%d, error=%v", channel.Id, persistErr))
	}
}

// RecordChannelProbeSuccess promotes an awaiting-probe channel into probation.
// On promotion, HP is refilled to maxHP, the fail streak is cleared, and trip count is decremented.
func RecordChannelProbeSuccess(channel *model.Channel) bool {
	if channel == nil {
		return false
	}
	promoted, err := mutateChannelBreakerState(channel, func(current *model.Channel, now time.Time) bool {
		nowUnix := now.Unix()
		isAwaitingProbe := current.IsBreakerAwaitingProbeAt(nowUnix)
		if current.BreakerCooldownAt <= 0 || current.IsBreakerProbationAt(nowUnix) {
			return false
		}
		if !isAwaitingProbe {
			return false
		}

		applyBreakerDecay(current, now)
		ensureHPInitialized(current)
		maxHP := computeMaxHP(current)
		current.BreakerHP = maxHP * hpProbeSuccessRefillFraction
		current.BreakerFailStreak = 0
		current.BreakerLastFailure = ""
		if current.BreakerTripCount > 0 {
			current.BreakerTripCount--
		}

		current.BreakerCooldownAt = nowUnix
		current.BreakerUpdatedAt = nowUnix
		return true
	})
	if err != nil {
		common.SysLog(fmt.Sprintf("failed to persist channel breaker probe success state: channel_id=%d, error=%v", channel.Id, err))
		return false
	}
	return promoted
}
