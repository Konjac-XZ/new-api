package service

import (
	"fmt"
	"math"
	"strings"
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
		// Suppress channels that are either actively cooling or awaiting scheduled
		// probe validation. Awaiting-probe channels should be validated by probes,
		// not by live user traffic.
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

// GetObservedChannelIDsIfNormalExist returns a set of channel IDs that are in an
// observed state (awaiting_probe or probation) — but only when at least one normal
// (healthy, non-observed) channel exists in the same group/model pool.
//
// This is used to enforce the single-chance rule: once a request has already been
// through an observed channel and failed, it should fall back exclusively to normal
// channels rather than burning another retry on a second observed candidate.
//
// Returns nil when no normal channels exist (e.g. all channels are observed or in
// cooldown) so the caller can fall back to the full candidate pool.
func GetObservedChannelIDsIfNormalExist(group string, modelName string) (map[int]bool, error) {
	channels, err := model.GetEnabledChannelsByGroupModel(group, modelName)
	if err != nil {
		return nil, err
	}
	if len(channels) == 0 {
		return nil, nil
	}
	now := time.Now().Unix()
	observedIDs := make(map[int]bool)
	hasNormal := false
	for _, channel := range channels {
		if channel == nil {
			continue
		}
		// Cooling channels are already suppressed by GetDynamicSuppressedChannelIDs;
		// skip them here — they are neither normal nor candidates for selection.
		if channel.IsBreakerCoolingAt(now) {
			continue
		}
		if IsChannelObserved(channel, now) {
			observedIDs[channel.Id] = true
		} else {
			hasNormal = true
		}
	}
	if !hasNormal {
		// No normal channel exists — do not restrict; let the caller use observed channels.
		return nil, nil
	}
	if len(observedIDs) == 0 {
		return nil, nil
	}
	return observedIDs, nil
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

func marshalBreakerTraceJSON(value any) string {
	data, err := common.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func buildBreakerPenaltyTrace(
	current *model.Channel,
	now time.Time,
	eventType string,
	failureKind channelFailureKind,
	wasInProbation bool,
	wasAwaitingProbe bool,
	forceCooldown bool,
	pressureBefore float64,
	hpBefore float64,
	failStreakBefore int,
	tripCountBefore int,
	cooldownAtBefore int64,
	damage float64,
	triggeredCooldown bool,
	baseCooldown time.Duration,
	multiplier float64,
	multiplierBreakdown breakerCooldownMultiplierBreakdown,
	chronicFloor time.Duration,
	chronicBreakdown breakerChronicCooldownFloorBreakdown,
	finalCooldown time.Duration,
) *model.BreakerPenaltyTrace {
	inputs := map[string]any{
		"failure_kind":                       failureKind,
		"pressure_before":                    pressureBefore,
		"pressure_after":                     current.BreakerPressure,
		"fail_streak_before":                 failStreakBefore,
		"fail_streak_after":                  current.BreakerFailStreak,
		"trip_count_before":                  tripCountBefore,
		"trip_count_after":                   current.BreakerTripCount,
		"hp_before":                          hpBefore,
		"hp_damage":                          damage,
		"hp_after":                           current.BreakerHP,
		"was_in_probation":                   wasInProbation,
		"was_awaiting_probe":                 wasAwaitingProbe,
		"force_cooldown":                     forceCooldown,
		"cooldown_at_before":                 cooldownAtBefore,
		"cooldown_at_after":                  current.BreakerCooldownAt,
		"base_cooldown_seconds":              int64(baseCooldown / time.Second),
		"cooldown_multiplier":                multiplier,
		"chronic_floor_seconds":              int64(chronicFloor / time.Second),
		"final_cooldown_seconds":             int64(finalCooldown / time.Second),
		"triggered_cooldown":                 triggeredCooldown,
		"short_term_penalty_factor":          multiplierBreakdown.ShortTermPenaltyFactor,
		"pressure_penalty_factor":            multiplierBreakdown.PressurePenaltyFactor,
		"history_penalty_factor":             multiplierBreakdown.HistoryPenaltyFactor,
		"failure_rate":                       multiplierBreakdown.FailureRate,
		"timeout_rate":                       multiplierBreakdown.TimeoutRate,
		"confidence":                         multiplierBreakdown.Confidence,
		"chronic_trip_floor_seconds":         int64(chronicBreakdown.TripFloor / time.Second),
		"chronic_failure_rate_floor_seconds": int64(chronicBreakdown.FailureRateFloor / time.Second),
		"chronic_streak_floor_seconds":       int64(chronicBreakdown.StreakFloor / time.Second),
	}
	rawCooldown := time.Duration(float64(baseCooldown) * multiplier)

	steps := []string{
		fmt.Sprintf("Step 1: HP_after = HP_before - damage = %.6f - %.6f = %.6f", hpBefore, damage, current.BreakerHP),
	}
	if triggeredCooldown {
		steps = append(steps,
			fmt.Sprintf("Step 2: multiplier = 1 + pressure(%.6f) + streak(%.6f) + trip(%.6f) + failure_rate(%.6f) + timeout_rate(%.6f) + probation(%.6f) + awaiting_probe(%.6f) = %.6f",
				multiplierBreakdown.PressureContribution,
				multiplierBreakdown.StreakContribution,
				multiplierBreakdown.TripContribution,
				multiplierBreakdown.FailureRateContribution,
				multiplierBreakdown.TimeoutRateContribution,
				multiplierBreakdown.ProbationContribution,
				multiplierBreakdown.AwaitingProbeContribution,
				multiplier,
			),
			fmt.Sprintf("Step 3: cooldown_raw = base_cooldown * multiplier = %ds * %.6f = %.6fs", int64(baseCooldown/time.Second), multiplier, baseCooldown.Seconds()*multiplier),
			fmt.Sprintf("Step 4: cooldown_with_floor = max(raw=%ds, chronic_floor=%ds) = %ds", int64(rawCooldown/time.Second), int64(chronicFloor/time.Second), int64(maxDuration(rawCooldown, chronicFloor)/time.Second)),
			fmt.Sprintf("Step 5: cooldown_clamped = clamp(min=%ds, max=%ds) = %ds", int64(breakerMinimumCooldown/time.Second), int64(breakerMaxCooldown/time.Second), int64(finalCooldown/time.Second)),
			fmt.Sprintf("Result: cooldown_at = now + %ds = %d", int64(finalCooldown/time.Second), current.BreakerCooldownAt),
		)
	} else {
		steps = append(steps, "Step 2: cooldown not triggered because HP remained above zero and no forced cooldown flag was set", "Result: final penalty duration = 0s")
	}

	result := map[string]any{
		"triggered_cooldown":     triggeredCooldown,
		"base_cooldown_seconds":  int64(baseCooldown / time.Second),
		"cooldown_multiplier":    multiplier,
		"chronic_floor_seconds":  int64(chronicFloor / time.Second),
		"final_cooldown_seconds": int64(finalCooldown / time.Second),
		"cooldown_at":            current.BreakerCooldownAt,
	}

	return &model.BreakerPenaltyTrace{
		ChannelId:            current.Id,
		CreatedAt:            now.Unix(),
		EventType:            eventType,
		FailureKind:          string(failureKind),
		WasInProbation:       wasInProbation,
		WasAwaitingProbe:     wasAwaitingProbe,
		ForceCooldown:        forceCooldown,
		TriggeredCooldown:    triggeredCooldown,
		CooldownAtBefore:     cooldownAtBefore,
		CooldownAtAfter:      current.BreakerCooldownAt,
		PressureBefore:       pressureBefore,
		PressureAfter:        current.BreakerPressure,
		FailStreakBefore:     failStreakBefore,
		FailStreakAfter:      current.BreakerFailStreak,
		TripCountBefore:      tripCountBefore,
		TripCountAfter:       current.BreakerTripCount,
		HPBefore:             hpBefore,
		HPDamage:             damage,
		HPAfter:              current.BreakerHP,
		BaseCooldownSeconds:  int64(baseCooldown / time.Second),
		CooldownMultiplier:   multiplier,
		ChronicFloorSeconds:  int64(chronicFloor / time.Second),
		FinalCooldownSeconds: int64(finalCooldown / time.Second),
		CalculationInputs:    marshalBreakerTraceJSON(inputs),
		CalculationSteps:     strings.Join(steps, "\n"),
		CalculationResult:    marshalBreakerTraceJSON(result),
	}
}

func maxDuration(a time.Duration, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
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
) *model.BreakerPenaltyTrace {
	if current == nil {
		return nil
	}
	pressureBefore := current.BreakerPressure
	hpBefore := current.BreakerHP
	failStreakBefore := current.BreakerFailStreak
	tripCountBefore := current.BreakerTripCount
	cooldownAtBefore := current.BreakerCooldownAt

	multiplier := 1.0
	var multiplierBreakdown breakerCooldownMultiplierBreakdown
	baseCooldown := time.Duration(0)
	chronicFloor := time.Duration(0)
	var chronicBreakdown breakerChronicCooldownFloorBreakdown
	finalCooldown := time.Duration(0)
	triggeredCooldown := false

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
		triggeredCooldown = true
		current.BreakerHP = 0
		current.BreakerTripCount++

		multiplier, multiplierBreakdown = computeBreakerCooldownMultiplierWithBreakdown(current, wasInProbation, wasAwaitingProbe)
		baseCooldown = failureBaseCooldown(failureKind)
		cooldown := time.Duration(float64(baseCooldown) * multiplier)
		chronicFloor, chronicBreakdown = computeBreakerChronicCooldownFloorWithBreakdown(current)
		if chronicFloor > cooldown {
			cooldown = chronicFloor
		}
		if cooldown < breakerMinimumCooldown {
			cooldown = breakerMinimumCooldown
		}
		if cooldown > breakerMaxCooldown {
			cooldown = breakerMaxCooldown
		}
		finalCooldown = cooldown

		current.BreakerCooldownAt = now.Add(cooldown).Unix()
	}
	// If HP > 0 and not force-cooling: no cooldown triggered, channel remains in service.

	current.BreakerLastFailure = string(failureKind)
	current.BreakerUpdatedAt = now.Unix()

	return buildBreakerPenaltyTrace(
		current,
		now,
		"relay_failure",
		failureKind,
		wasInProbation,
		wasAwaitingProbe,
		forceCooldown,
		pressureBefore,
		hpBefore,
		failStreakBefore,
		tripCountBefore,
		cooldownAtBefore,
		damage,
		triggeredCooldown,
		baseCooldown,
		multiplier,
		multiplierBreakdown,
		chronicFloor,
		chronicBreakdown,
		finalCooldown,
	)
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
	_, _, persistErr := mutateChannelBreakerStateWithTrace(channel, func(current *model.Channel, now time.Time) (bool, *model.BreakerPenaltyTrace) {
		wasInProbation := current.IsBreakerProbationAt(now.Unix())
		wasAwaitingProbe := current.IsBreakerAwaitingProbeAt(now.Unix())
		failureKind := classifyChannelFailure(info, err)
		forceCooldown := wasInProbation
		trace := applyRelayFailureStateLocked(current, info, now, failureKind, wasInProbation, wasAwaitingProbe, forceCooldown, 0, 1.0)
		return true, trace
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
	_, _, persistErr := mutateChannelBreakerStateWithTrace(channel, func(current *model.Channel, now time.Time) (bool, *model.BreakerPenaltyTrace) {
		nowUnix := now.Unix()
		if current.IsBreakerCoolingAt(nowUnix) {
			return false, nil
		}

		wasAwaitingProbe := current.IsBreakerAwaitingProbeAt(nowUnix)
		wasInProbation := current.IsBreakerProbationAt(nowUnix)
		pressureBefore := current.BreakerPressure
		hpBefore := current.BreakerHP
		failStreakBefore := current.BreakerFailStreak
		tripCountBefore := current.BreakerTripCount
		cooldownAtBefore := current.BreakerCooldownAt
		multiplier := 1.0
		var multiplierBreakdown breakerCooldownMultiplierBreakdown
		baseCooldown := time.Duration(0)
		chronicFloor := time.Duration(0)
		var chronicBreakdown breakerChronicCooldownFloorBreakdown
		finalCooldown := time.Duration(0)
		triggeredCooldown := false
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
			triggeredCooldown = true
			current.BreakerHP = 0
			current.BreakerTripCount++

			multiplier, multiplierBreakdown = computeBreakerCooldownMultiplierWithBreakdown(current, wasInProbation, wasAwaitingProbe)
			if wasInProbation {
				multiplier += 1.75
				multiplierBreakdown.ProbationContribution += 1.75
				multiplierBreakdown.Multiplier = multiplier
			}
			baseCooldown = failureBaseCooldown(failureKind)
			cooldown := time.Duration(float64(baseCooldown) * multiplier)
			chronicFloor, chronicBreakdown = computeBreakerChronicCooldownFloorWithBreakdown(current)
			if chronicFloor > cooldown {
				cooldown = chronicFloor
			}
			if cooldown < breakerMinimumCooldown {
				cooldown = breakerMinimumCooldown
			}
			if cooldown > breakerMaxCooldown {
				cooldown = breakerMaxCooldown
			}
			finalCooldown = cooldown

			current.BreakerCooldownAt = now.Add(cooldown).Unix()
		}

		current.BreakerLastFailure = string(failureKind)
		current.BreakerUpdatedAt = nowUnix
		trace := buildBreakerPenaltyTrace(
			current,
			now,
			"probe_failure",
			failureKind,
			wasInProbation,
			wasAwaitingProbe,
			wasAwaitingProbe || wasInProbation,
			pressureBefore,
			hpBefore,
			failStreakBefore,
			tripCountBefore,
			cooldownAtBefore,
			damage,
			triggeredCooldown,
			baseCooldown,
			multiplier,
			multiplierBreakdown,
			chronicFloor,
			chronicBreakdown,
			finalCooldown,
		)
		return true, trace
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
