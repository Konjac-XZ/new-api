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

const (
	breakerDecayWindow             = time.Hour
	breakerMaxCooldown             = 30 * time.Minute
	breakerMinimumCooldown         = 30 * time.Second
	breakerNormalRecoveryFactor    = 0.7
	breakerProbationRecoveryFactor = 0.45
	breakerSlowSuccessPressure     = 0.35
	breakerProbationPenalty        = 0.75
	breakerAwaitingProbePenalty    = 4.0
	breakerMinPressure             = 0.05
	breakerMaxPressureContribution = 8.0
	breakerSlowSuccessThreshold    = 15 * time.Second
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
	applyBreakerDecay(current, now)

	failureKind := classifyChannelFailure(info, err)
	current.BreakerPressure += failurePressureWeight(failureKind)
	if wasInProbation {
		current.BreakerPressure += breakerProbationPenalty
	}
	if wasAwaitingProbe {
		// Cooldown has already elapsed, but a probe still failed: treat this as a
		// high-confidence reliability signal and apply a strong pressure bonus.
		current.BreakerPressure += breakerAwaitingProbePenalty
	}
	current.BreakerFailStreak++

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
	current.BreakerLastFailure = string(failureKind)
	current.BreakerUpdatedAt = now.Unix()

	if err := model.UpdateChannelBreakerState(current); err != nil {
		common.SysLog(fmt.Sprintf("failed to persist channel breaker failure state: channel_id=%d, error=%v", channel.Id, err))
	}
}

// RecordChannelProbeSuccess promotes a cooling channel into probation.
// It intentionally does not clear breaker pressure/fail streak, so a probe
// success alone cannot fully recover the channel.
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
	isCooling := current.IsBreakerCoolingAt(nowUnix)
	isAwaitingProbe := current.IsBreakerAwaitingProbeAt(nowUnix)
	if current.BreakerCooldownAt <= 0 || current.IsBreakerProbationAt(nowUnix) {
		return false
	}
	if !isCooling && !isAwaitingProbe {
		return false
	}
	applyBreakerDecay(current, now)

	current.BreakerCooldownAt = nowUnix
	current.BreakerUpdatedAt = nowUnix
	if err := model.UpdateChannelBreakerState(current); err != nil {
		common.SysLog(fmt.Sprintf("failed to persist channel breaker probe success state: channel_id=%d, error=%v", channel.Id, err))
		return false
	}
	return true
}

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
