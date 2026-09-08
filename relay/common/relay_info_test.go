package common

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelayInfoForceStreamOnlyAppliesToOpenAICompletionsRequests(t *testing.T) {
	tests := []struct {
		name               string
		channelType        int
		relayMode          int
		clientStream       bool
		wantUpstreamStream bool
		wantBuffer         bool
	}{
		{name: "OpenAI chat completions", channelType: constant.ChannelTypeOpenAI, relayMode: relayconstant.RelayModeChatCompletions, wantUpstreamStream: true, wantBuffer: true},
		{name: "OpenAI legacy completions", channelType: constant.ChannelTypeOpenAI, relayMode: relayconstant.RelayModeCompletions, wantUpstreamStream: true, wantBuffer: true},
		{name: "OpenAI responses excluded", channelType: constant.ChannelTypeOpenAI, relayMode: relayconstant.RelayModeResponses},
		{name: "other channel excluded", channelType: constant.ChannelTypeAnthropic, relayMode: relayconstant.RelayModeChatCompletions},
		{name: "client stream remains streaming", channelType: constant.ChannelTypeAnthropic, relayMode: relayconstant.RelayModeChatCompletions, clientStream: true, wantUpstreamStream: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &RelayInfo{
				IsStream:  tt.clientStream,
				RelayMode: tt.relayMode,
				ChannelMeta: &ChannelMeta{
					ChannelType:    tt.channelType,
					ChannelSetting: dto.ChannelSettings{ForceStream: true},
				},
			}

			assert.Equal(t, tt.wantUpstreamStream, info.IsUpstreamStream())
			assert.Equal(t, tt.wantBuffer, info.ShouldBufferUpstreamStream())
		})
	}
}

func TestRelayInfoGetFinalRequestRelayFormatPrefersExplicitFinal(t *testing.T) {
	info := &RelayInfo{
		RelayFormat:             types.RelayFormatOpenAI,
		RequestConversionChain:  []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude},
		FinalRequestRelayFormat: types.RelayFormatOpenAIResponses,
	}

	require.Equal(t, types.RelayFormat(types.RelayFormatOpenAIResponses), info.GetFinalRequestRelayFormat())
}

func TestRelayInfoGetFinalRequestRelayFormatFallsBackToConversionChain(t *testing.T) {
	info := &RelayInfo{
		RelayFormat:            types.RelayFormatOpenAI,
		RequestConversionChain: []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude},
	}

	require.Equal(t, types.RelayFormat(types.RelayFormatClaude), info.GetFinalRequestRelayFormat())
}

func TestRelayInfoGetFinalRequestRelayFormatFallsBackToRelayFormat(t *testing.T) {
	info := &RelayInfo{
		RelayFormat: types.RelayFormatGemini,
	}

	require.Equal(t, types.RelayFormat(types.RelayFormatGemini), info.GetFinalRequestRelayFormat())
}

func TestRelayInfoGetFinalRequestRelayFormatNilReceiver(t *testing.T) {
	var info *RelayInfo
	require.Equal(t, types.RelayFormat(""), info.GetFinalRequestRelayFormat())
}

func TestRelayInfoSetFirstResponseTimeRecordsChannelSuccessOnce(t *testing.T) {
	info := &RelayInfo{
		StartTime:       time.Now().Add(-time.Second),
		isFirstResponse: true,
	}
	var callCount int32
	info.SetChannelSuccessRecorder(func() {
		atomic.AddInt32(&callCount, 1)
	})

	info.SetFirstResponseTime()
	require.True(t, info.HasSendResponse())
	require.EqualValues(t, 1, atomic.LoadInt32(&callCount))

	info.SetFirstResponseTime()
	require.EqualValues(t, 1, atomic.LoadInt32(&callCount))

	info.RecordChannelSuccess(func() {
		atomic.AddInt32(&callCount, 1)
	})
	require.EqualValues(t, 1, atomic.LoadInt32(&callCount))
}

func TestRelayInfoMetaTypedNilReceiver(t *testing.T) {
	var info *RelayInfo
	var meta convmeta.Meta = info

	assert.Empty(t, meta.GetOriginModelName())
	assert.Empty(t, meta.GetUpstreamModelName())
	assert.False(t, meta.HasChannelMeta())
	assert.Zero(t, meta.GetChannelID())
	assert.Zero(t, meta.GetChannelType())
	assert.False(t, meta.GetIsStream())
	assert.Empty(t, meta.GetReasoningEffort())
	assert.Zero(t, meta.GetEstimatePromptTokens())
	assert.Zero(t, meta.GetSendResponseCount())

	assert.NotPanics(t, func() {
		meta.SetReasoningEffort("high")
		meta.IncrSendResponseCount()
		meta.AppendRequestConversion(types.RelayFormatClaude)
	})

	firstState := meta.EnsureClaudeConvertInfo()
	secondState := meta.EnsureClaudeConvertInfo()
	require.NotNil(t, firstState)
	require.NotNil(t, secondState)
	assert.Equal(t, convmeta.LastMessageTypeNone, firstState.LastMessagesType)
	assert.NotSame(t, firstState, secondState)

	firstOptions := meta.ConvOptions()
	secondOptions := meta.ConvOptions()
	require.NotNil(t, firstOptions)
	require.NotNil(t, secondOptions)
	assert.NotSame(t, firstOptions, secondOptions)
	assert.NotNil(t, firstOptions.Claude.DefaultMaxTokens)
	assert.NotNil(t, firstOptions.Gemini.SupportsImagine)
	assert.NotNil(t, firstOptions.Gemini.SafetySetting)
	assert.NotNil(t, firstOptions.PreserveThinkingSuffix)
}
