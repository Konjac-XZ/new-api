package common

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

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
