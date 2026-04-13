package service

import (
	"math"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
)

type breakerShortTermPenaltyBreakdown struct {
	StreakInstability  float64
	FailureRate        float64
	TimeoutRate        float64
	Confidence         float64
	BlendedInstability float64
	Factor             float64
}

type breakerCooldownMultiplierBreakdown struct {
	ShortTermPenaltyFactor    float64
	PressurePenaltyFactor     float64
	HistoryPenaltyFactor      float64
	PressureContribution      float64
	StreakContribution        float64
	TripContribution          float64
	FailureRateContribution   float64
	TimeoutRateContribution   float64
	ProbationContribution     float64
	AwaitingProbeContribution float64
	FailureRate               float64
	TimeoutRate               float64
	Confidence                float64
	Multiplier                float64
}

type breakerChronicCooldownFloorBreakdown struct {
	ShortTermPenaltyFactor float64
	HistoryPenaltyFactor   float64
	TripFloor              time.Duration
	FailureRateFloor       time.Duration
	StreakFloor            time.Duration
	FailureRate            float64
	Confidence             float64
	Floor                  time.Duration
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
	factor, _ := computeBreakerShortTermPenaltyFactorWithBreakdown(channel)
	return factor
}

func computeBreakerShortTermPenaltyFactorWithBreakdown(channel *model.Channel) (float64, breakerShortTermPenaltyBreakdown) {
	if channel == nil {
		return 1.0, breakerShortTermPenaltyBreakdown{Factor: 1.0}
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
			return 1.0, breakerShortTermPenaltyBreakdown{
				StreakInstability:  streakInstability,
				FailureRate:        failureRate,
				TimeoutRate:        timeoutRate,
				Confidence:         confidence,
				BlendedInstability: 0,
				Factor:             1.0,
			}
		}
		factor := breakerShortTermPenaltyMinFactor + (1.0-breakerShortTermPenaltyMinFactor)*streakInstability
		return factor, breakerShortTermPenaltyBreakdown{
			StreakInstability:  streakInstability,
			FailureRate:        failureRate,
			TimeoutRate:        timeoutRate,
			Confidence:         confidence,
			BlendedInstability: streakInstability,
			Factor:             factor,
		}
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
		factor = breakerShortTermPenaltyMinFactor
	}
	if factor > 1.0 {
		factor = 1.0
	}
	return factor, breakerShortTermPenaltyBreakdown{
		StreakInstability:  streakInstability,
		FailureRate:        failureRate,
		TimeoutRate:        timeoutRate,
		Confidence:         confidence,
		BlendedInstability: blendedInstability,
		Factor:             factor,
	}
}

func computeBreakerCooldownMultiplier(channel *model.Channel, wasInProbation bool, wasAwaitingProbe bool) float64 {
	multiplier, _ := computeBreakerCooldownMultiplierWithBreakdown(channel, wasInProbation, wasAwaitingProbe)
	return multiplier
}

func computeBreakerCooldownMultiplierWithBreakdown(channel *model.Channel, wasInProbation bool, wasAwaitingProbe bool) (float64, breakerCooldownMultiplierBreakdown) {
	if channel == nil {
		return 1.0, breakerCooldownMultiplierBreakdown{Multiplier: 1.0}
	}

	multiplier := 1.0
	shortTermPenaltyFactor, _ := computeBreakerShortTermPenaltyFactorWithBreakdown(channel)
	pressurePenaltyFactor := breakerShortTermPressureMinFactor + (1.0-breakerShortTermPressureMinFactor)*shortTermPenaltyFactor
	historyPenaltyFactor := math.Pow(shortTermPenaltyFactor, breakerShortTermHistoryExponent)
	pressureContribution := math.Min(channel.BreakerPressure, breakerMaxPressureContribution) * breakerPressureCooldownWeight * pressurePenaltyFactor
	multiplier += pressureContribution

	streakContribution := 0.0
	tripContribution := 0.0
	failureRateContribution := 0.0
	timeoutRateContribution := 0.0
	probationContribution := 0.0
	awaitingProbeContribution := 0.0

	if channel.BreakerFailStreak > 1 {
		streakPenalty := math.Pow(
			float64(minInt(channel.BreakerFailStreak-1, breakerFailStreakCooldownCap)),
			breakerFailStreakCooldownExponent,
		) * breakerFailStreakCooldownWeight
		streakContribution = streakPenalty
		multiplier += streakContribution
	}

	if channel.BreakerTripCount > breakerTripCooldownStart {
		tripPenalty := math.Log1p(float64(minInt(channel.BreakerTripCount-breakerTripCooldownStart, breakerTripCooldownCap))) * breakerTripCooldownWeight * historyPenaltyFactor
		tripContribution = tripPenalty
		multiplier += tripContribution
	}

	failureRate, timeoutRate, confidence := computeBreakerFailureRates(channel)
	if confidence > 0 {
		if failureRate > breakerFailureRateCooldownThreshold {
			normalized := (failureRate - breakerFailureRateCooldownThreshold) / (1.0 - breakerFailureRateCooldownThreshold)
			failureRateContribution = math.Pow(normalized, breakerFailureRateCooldownExponent) * breakerFailureRateCooldownWeight * confidence
			multiplier += failureRateContribution
		}
		if timeoutRate > breakerTimeoutRateCooldownThreshold {
			normalized := (timeoutRate - breakerTimeoutRateCooldownThreshold) / (1.0 - breakerTimeoutRateCooldownThreshold)
			timeoutRateContribution = math.Pow(normalized, breakerTimeoutRateCooldownExponent) * breakerTimeoutRateCooldownWeight * confidence
			multiplier += timeoutRateContribution
		}
	}

	if wasInProbation {
		probationContribution = 0.75
		multiplier += probationContribution
	}
	if wasAwaitingProbe {
		awaitingProbeContribution = 1.5
		multiplier += awaitingProbeContribution
	}
	if multiplier < 1.0 {
		multiplier = 1.0
	}
	return multiplier, breakerCooldownMultiplierBreakdown{
		ShortTermPenaltyFactor:    shortTermPenaltyFactor,
		PressurePenaltyFactor:     pressurePenaltyFactor,
		HistoryPenaltyFactor:      historyPenaltyFactor,
		PressureContribution:      pressureContribution,
		StreakContribution:        streakContribution,
		TripContribution:          tripContribution,
		FailureRateContribution:   failureRateContribution,
		TimeoutRateContribution:   timeoutRateContribution,
		ProbationContribution:     probationContribution,
		AwaitingProbeContribution: awaitingProbeContribution,
		FailureRate:               failureRate,
		TimeoutRate:               timeoutRate,
		Confidence:                confidence,
		Multiplier:                multiplier,
	}
}

func computeBreakerChronicCooldownFloor(channel *model.Channel) time.Duration {
	floor, _ := computeBreakerChronicCooldownFloorWithBreakdown(channel)
	return floor
}

func computeBreakerChronicCooldownFloorWithBreakdown(channel *model.Channel) (time.Duration, breakerChronicCooldownFloorBreakdown) {
	if channel == nil {
		return 0, breakerChronicCooldownFloorBreakdown{}
	}

	floor := time.Duration(0)
	shortTermPenaltyFactor, _ := computeBreakerShortTermPenaltyFactorWithBreakdown(channel)
	historyPenaltyFactor := math.Pow(shortTermPenaltyFactor, breakerShortTermHistoryExponent)
	tripFloor := time.Duration(0)
	if channel.BreakerTripCount > breakerChronicTripFloorStart {
		tripFloor = time.Duration(
			math.Log1p(float64(minInt(channel.BreakerTripCount-breakerChronicTripFloorStart, breakerTripCooldownCap))) *
				breakerChronicTripFloorWeight * historyPenaltyFactor * float64(time.Minute),
		)
		floor += tripFloor
	}

	failureRate, _, confidence := computeBreakerFailureRates(channel)
	failureRateFloor := time.Duration(0)
	if confidence > 0 && failureRate > breakerChronicFailureFloorThreshold {
		normalized := (failureRate - breakerChronicFailureFloorThreshold) / (1.0 - breakerChronicFailureFloorThreshold)
		failureRateFloor = time.Duration(
			math.Pow(normalized, breakerChronicFailureFloorExponent) *
				breakerChronicFailureFloorWeight *
				confidence * float64(time.Minute),
		)
		floor += failureRateFloor
	}

	streakFloor := time.Duration(0)
	if channel.BreakerFailStreak > breakerChronicStreakFloorStart {
		streakFloor = time.Duration(
			math.Pow(
				float64(minInt(channel.BreakerFailStreak-breakerChronicStreakFloorStart, breakerFailStreakCooldownCap)),
				breakerChronicStreakFloorExponent,
			) * breakerChronicStreakFloorWeight * float64(time.Minute),
		)
		floor += streakFloor
	}

	if floor > breakerMaxCooldown {
		floor = breakerMaxCooldown
	}
	return floor, breakerChronicCooldownFloorBreakdown{
		ShortTermPenaltyFactor: shortTermPenaltyFactor,
		HistoryPenaltyFactor:   historyPenaltyFactor,
		TripFloor:              tripFloor,
		FailureRateFloor:       failureRateFloor,
		StreakFloor:            streakFloor,
		FailureRate:            failureRate,
		Confidence:             confidence,
		Floor:                  floor,
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

// failureHPDamage returns the HP damage for a given failure kind.
// Decoupled from pressure weight to allow softer treatment of transient failures.
// Overloaded (429/502/503) deals half damage: a channel can absorb ~20 consecutive
// rate-limit responses before tripping, vs ~10 with pressure weight.
func failureHPDamage(kind channelFailureKind) float64 {
	switch kind {
	case channelFailureKindOverloaded:
		return 0.5
	default:
		return failurePressureWeight(kind)
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

func classifySuccessfulRequestLatency(channel *model.Channel, info *relaycommon.RelayInfo) channelSuccessLatencyClass {
	latency, ok := getStreamingFirstResponseLatency(info)
	if !ok {
		return channelSuccessLatencyNormal
	}
	threshold := successLatencyThreshold(channel)
	if threshold <= 0 {
		return channelSuccessLatencyNormal
	}
	if latency <= threshold/2 {
		return channelSuccessLatencyFast
	}
	nearTimeoutThreshold := threshold * 4 / 5
	if latency >= nearTimeoutThreshold {
		return channelSuccessLatencyNearTimeout
	}
	return channelSuccessLatencyNormal
}

func successLatencyThreshold(channel *model.Channel) time.Duration {
	if channel != nil {
		if maxLatency := channel.GetMaxFirstTokenLatency(); maxLatency > 0 {
			return time.Duration(maxLatency) * time.Second
		}
	}
	return breakerSlowSuccessThreshold
}

func getStreamingFirstResponseLatency(info *relaycommon.RelayInfo) (time.Duration, bool) {
	if info == nil || !info.IsStream || info.StartTime.IsZero() || !info.HasSendResponse() {
		return 0, false
	}
	latency := info.FirstResponseTime.Sub(info.StartTime)
	if latency < 0 {
		return 0, false
	}
	return latency, true
}

func failureDamageLatencyMultiplier(channel *model.Channel, info *relaycommon.RelayInfo, kind channelFailureKind, now time.Time) float64 {
	if kind == channelFailureKindFirstTokenTimeout {
		return hpTimeoutFailureDamageFactor
	}
	if info == nil || !info.IsStream || info.StartTime.IsZero() || info.HasSendResponse() {
		return 1.0
	}
	threshold := successLatencyThreshold(channel)
	if threshold <= 0 {
		return 1.0
	}
	elapsed := now.Sub(info.StartTime)
	if elapsed <= 0 {
		return 1.0
	}
	if elapsed <= threshold/2 {
		return hpQuickHonestFailureDamageFactor
	}
	if elapsed >= threshold*4/5 {
		return hpDelayedFailureDamageFactor
	}
	return 1.0
}

func probationSuccessRefillFraction(latencyClass channelSuccessLatencyClass) float64 {
	if latencyClass == channelSuccessLatencyFast {
		return hpFastProbationSuccessRefillFraction
	}
	return hpProbationSuccessRefillFraction
}

func awaitingProbeSuccessRefillFraction(latencyClass channelSuccessLatencyClass) float64 {
	if latencyClass == channelSuccessLatencyFast {
		return hpFastProbeSuccessRefillFraction
	}
	return hpProbeSuccessRefillFraction
}

func isSlowSuccessfulRequest(channel *model.Channel, info *relaycommon.RelayInfo) bool {
	return classifySuccessfulRequestLatency(channel, info) == channelSuccessLatencyNearTimeout
}

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}
