package service

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
)

func buildDynamicBreakerSettingForSelectTest(t *testing.T) *string {
	t.Helper()
	settingBytes, err := common.Marshal(dto.ChannelSettings{DynamicCircuitBreaker: true})
	if err != nil {
		t.Fatalf("marshal setting failed: %v", err)
	}
	setting := string(settingBytes)
	return &setting
}

func buildObservedChannelForSelectTest(t *testing.T, id int, priority int64) *model.Channel {
	t.Helper()
	autoBan := 1
	weight := uint(100)
	setting := buildDynamicBreakerSettingForSelectTest(t)
	now := time.Now().Unix()
	return &model.Channel{
		Id:               id,
		AutoBan:          &autoBan,
		Weight:           &weight,
		Priority:         &priority,
		Setting:          setting,
		BreakerCooldownAt: now - 30,
		BreakerUpdatedAt:  now - 30,
	}
}

func buildNormalChannelForSelectTest(id int, priority int64) *model.Channel {
	weight := uint(100)
	return &model.Channel{
		Id:       id,
		Weight:   &weight,
		Priority: &priority,
	}
}

func TestSelectObservedChannel_DoesNotPickLowerPriorityObservedWhenHigherTierExists(t *testing.T) {
	observedLow := buildObservedChannelForSelectTest(t, 101, 1)
	normalHigher := buildNormalChannelForSelectTest(103, 9)

	selected := selectObservedChannel([]*model.Channel{normalHigher, observedLow}, nil)
	if selected != nil {
		t.Fatalf("expected nil when highest-priority tier has no observed channel, got id=%d", selected.Id)
	}
}

func TestSelectObservedChannel_PicksObservedFromHighestAvailableTier(t *testing.T) {
	observedTop := buildObservedChannelForSelectTest(t, 111, 9)
	normalTop := buildNormalChannelForSelectTest(112, 9)
	observedLower := buildObservedChannelForSelectTest(t, 113, 3)

	selected := selectObservedChannel([]*model.Channel{normalTop, observedTop, observedLower}, nil)
	if selected == nil {
		t.Fatal("expected observation-period channel to be selected from top tier")
	}
	if selected.Id != observedTop.Id {
		t.Fatalf("expected observed channel id=%d from highest available tier, got id=%d", observedTop.Id, selected.Id)
	}
}

func TestSelectObservedChannel_SkipsExcludedObservedChannels(t *testing.T) {
	observedA := buildObservedChannelForSelectTest(t, 201, 2)
	observedB := buildObservedChannelForSelectTest(t, 202, 2)

	selected := selectObservedChannel([]*model.Channel{observedA, observedB}, map[int]bool{
		observedA.Id: true,
	})
	if selected == nil {
		t.Fatal("expected non-excluded observed channel to be selected")
	}
	if selected.Id != observedB.Id {
		t.Fatalf("expected id=%d after exclusion, got id=%d", observedB.Id, selected.Id)
	}
}

func TestSelectObservedChannel_ReturnsNilWhenNoObservedChannelExists(t *testing.T) {
	normalA := buildNormalChannelForSelectTest(301, 5)
	normalB := buildNormalChannelForSelectTest(302, 1)

	selected := selectObservedChannel([]*model.Channel{normalA, normalB}, nil)
	if selected != nil {
		t.Fatalf("expected nil when no observed channel exists, got id=%d", selected.Id)
	}
}
