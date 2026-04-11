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