package service

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
)

func TestApplyBreakerDecay(t *testing.T) {
	channel := &model.Channel{
		BreakerPressure:  4,
		BreakerUpdatedAt: time.Now().Add(-breakerDecayWindow).Unix(),
	}

	applyBreakerDecay(channel, time.Now())

	if channel.BreakerPressure <= 0 || channel.BreakerPressure >= 4 {
		t.Fatalf("expected decayed pressure between 0 and 4, got %f", channel.BreakerPressure)
	}
}

func TestClassifyChannelFailure(t *testing.T) {
	firstTokenErr := types.NewErrorWithStatusCode(
		fmt.Errorf("too slow"),
		types.ErrorCodeChannelFirstTokenLatencyExceeded,
		http.StatusGatewayTimeout,
	)
	if got := classifyChannelFailure(nil, firstTokenErr); got != channelFailureKindFirstTokenTimeout {
		t.Fatalf("expected first token timeout, got %s", got)
	}

	info := &relaycommon.RelayInfo{
		StartTime:         time.Now().Add(-2 * time.Second),
		FirstResponseTime: time.Now().Add(-time.Second),
	}
	midStreamErr := types.NewOpenAIError(fmt.Errorf("stream broken"), types.ErrorCodeBadResponse, http.StatusBadGateway)
	if got := classifyChannelFailure(info, midStreamErr); got != channelFailureKindMidStreamFailure {
		t.Fatalf("expected mid-stream failure, got %s", got)
	}
}

func TestIsSlowSuccessfulRequest(t *testing.T) {
	settingBytes, err := common.Marshal(dto.ChannelSettings{DynamicCircuitBreaker: true})
	if err != nil {
		t.Fatalf("marshal setting failed: %v", err)
	}
	channel := &model.Channel{
		Setting:              common.GetPointer(string(settingBytes)),
		MaxFirstTokenLatency: common.GetPointer(10),
	}
	info := &relaycommon.RelayInfo{
		IsStream:          true,
		StartTime:         time.Now().Add(-9 * time.Second),
		FirstResponseTime: time.Now(),
	}

	if !isSlowSuccessfulRequest(channel, info) {
		t.Fatal("expected slow successful stream request to be detected")
	}
}

func TestRecordChannelProbeSuccess_DoesNotFastForwardCooldown(t *testing.T) {
	now := time.Now().Unix()
	channel := seedDynamicBreakerChannelForProbeTest(t, now+120, now-10, 5)

	if RecordChannelProbeSuccess(channel) {
		t.Fatal("expected probe success during cooldown to be ignored")
	}

	var latest model.Channel
	if err := model.DB.Select("breaker_cooldown_at", "breaker_updated_at").Where("id = ?", channel.Id).First(&latest).Error; err != nil {
		t.Fatalf("failed to reload channel breaker state: %v", err)
	}
	if latest.BreakerCooldownAt != channel.BreakerCooldownAt {
		t.Fatalf("expected cooldown_at to remain unchanged, got %d want %d", latest.BreakerCooldownAt, channel.BreakerCooldownAt)
	}
	if latest.BreakerUpdatedAt != channel.BreakerUpdatedAt {
		t.Fatalf("expected updated_at to remain unchanged, got %d want %d", latest.BreakerUpdatedAt, channel.BreakerUpdatedAt)
	}
}

func TestRecordChannelProbeSuccess_PromotesOnlyInAwaitingProbe(t *testing.T) {
	now := time.Now().Unix()
	channel := seedDynamicBreakerChannelForProbeTest(t, now-30, now-120, 5)

	if !RecordChannelProbeSuccess(channel) {
		t.Fatal("expected probe success to promote awaiting-probe channel")
	}

	var latest model.Channel
	if err := model.DB.Select("breaker_cooldown_at", "breaker_updated_at").Where("id = ?", channel.Id).First(&latest).Error; err != nil {
		t.Fatalf("failed to reload channel breaker state: %v", err)
	}
	if latest.BreakerCooldownAt <= channel.BreakerCooldownAt {
		t.Fatalf("expected cooldown_at to be promoted to now, got %d <= %d", latest.BreakerCooldownAt, channel.BreakerCooldownAt)
	}
	if latest.BreakerUpdatedAt != latest.BreakerCooldownAt {
		t.Fatalf("expected updated_at to match promoted cooldown_at, got updated_at=%d cooldown_at=%d", latest.BreakerUpdatedAt, latest.BreakerCooldownAt)
	}
}

func seedDynamicBreakerChannelForProbeTest(t *testing.T, cooldownAt int64, updatedAt int64, interval int) *model.Channel {
	t.Helper()
	settingBytes, err := common.Marshal(dto.ChannelSettings{DynamicCircuitBreaker: true})
	if err != nil {
		t.Fatalf("marshal setting failed: %v", err)
	}
	autoBan := 1
	weight := uint(0)
	priority := int64(0)
	channel := &model.Channel{
		Type:                  0,
		Key:                   "sk-probe-test",
		Name:                  fmt.Sprintf("probe-test-%d", time.Now().UnixNano()),
		Status:                common.ChannelStatusEnabled,
		Group:                 "default",
		Models:                "gpt-4o-mini",
		AutoBan:               &autoBan,
		ScheduledTestInterval: &interval,
		Weight:                &weight,
		Priority:              &priority,
		Setting:               common.GetPointer(string(settingBytes)),
		BreakerCooldownAt:     cooldownAt,
		BreakerUpdatedAt:      updatedAt,
	}
	if err := model.DB.Create(channel).Error; err != nil {
		t.Fatalf("failed to seed channel: %v", err)
	}
	t.Cleanup(func() {
		_ = model.DB.Delete(&model.Channel{}, channel.Id).Error
	})
	return channel
}
