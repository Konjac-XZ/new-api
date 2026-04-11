package service

import "github.com/QuantumNous/new-api/model"

const (
	channelBreakerPhaseNil           = "nil"
	channelBreakerPhaseDisabled      = "disabled"
	channelBreakerPhaseCooling       = "cooling"
	channelBreakerPhaseAwaitingProbe = "awaiting_probe"
	channelBreakerPhaseObservation   = "observation"
	channelBreakerPhaseClosed        = "closed"
)

func GetChannelBreakerPhase(channel *model.Channel, now int64) string {
	if channel == nil {
		return channelBreakerPhaseNil
	}
	if !channel.IsDynamicCircuitBreakerEnabled() {
		return channelBreakerPhaseDisabled
	}
	if channel.IsBreakerCoolingAt(now) {
		return channelBreakerPhaseCooling
	}
	if channel.IsBreakerAwaitingProbeAt(now) {
		return channelBreakerPhaseAwaitingProbe
	}
	if channel.IsBreakerProbationAt(now) {
		return channelBreakerPhaseObservation
	}
	return channelBreakerPhaseClosed
}

// IsChannelObserved returns true if the channel's dynamic circuit breaker is enabled
// and the channel is currently in an observed state (awaiting_probe or probation).
// Observed channels are given limited exposure to real traffic and may trigger the
// single-chance rule that prevents repeated failovers through observed channels.
func IsChannelObserved(channel *model.Channel, now int64) bool {
	if channel == nil || !channel.IsDynamicCircuitBreakerEnabled() {
		return false
	}
	return channel.IsBreakerAwaitingProbeAt(now) || channel.IsBreakerProbationAt(now)
}