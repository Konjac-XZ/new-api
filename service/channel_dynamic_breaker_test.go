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
