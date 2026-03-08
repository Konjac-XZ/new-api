package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

func newDynamicBreakerChannelForTest() *Channel {
	autoBan := 1
	priority := int64(0)
	weight := uint(0)
	settingBytes, _ := common.Marshal(dto.ChannelSettings{DynamicCircuitBreaker: true})
	setting := string(settingBytes)
	return &Channel{
		AutoBan:  &autoBan,
		Setting:  &setting,
		Status:   common.ChannelStatusEnabled,
		Group:    "default",
		Models:   "gpt-4o-mini",
		Priority: &priority,
		Weight:   &weight,
	}
}

func TestBreakerAwaitingProbeAndProbationWithScheduledProbe(t *testing.T) {
	ch := newDynamicBreakerChannelForTest()
	interval := 5
	ch.ScheduledTestInterval = &interval
	ch.BreakerCooldownAt = 100
	ch.BreakerUpdatedAt = 90

	now := int64(120)
	if !ch.IsBreakerAwaitingProbeAt(now) {
		t.Fatal("expected awaiting probe when cooldown expired but probe not confirmed")
	}
	if ch.IsBreakerProbationAt(now) {
		t.Fatal("did not expect probation before successful probe promotion")
	}

	// Probe success promotion marker.
	ch.BreakerUpdatedAt = ch.BreakerCooldownAt
	if ch.IsBreakerAwaitingProbeAt(now) {
		t.Fatal("did not expect awaiting probe after successful probe promotion")
	}
	if !ch.IsBreakerProbationAt(now) {
		t.Fatal("expected probation after successful probe promotion")
	}
}

func TestBreakerProbationWithoutScheduledProbe(t *testing.T) {
	ch := newDynamicBreakerChannelForTest()
	ch.BreakerCooldownAt = 100
	ch.BreakerUpdatedAt = 90

	now := int64(120)
	if ch.IsBreakerAwaitingProbeAt(now) {
		t.Fatal("did not expect awaiting probe without scheduled probe configured")
	}
	if !ch.IsBreakerProbationAt(now) {
		t.Fatal("expected probation when cooldown expires and no scheduled probe is configured")
	}
}
