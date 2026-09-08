package helper

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestFirstTokenWatchdogSupportsForcedUpstreamStream(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeChatCompletions,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    constant.ChannelTypeOpenAI,
			ChannelSetting: dto.ChannelSettings{ForceStream: true},
		},
	}

	watchdog := EnsureFirstTokenWatchdog(c, info, 8, nil)
	require.NotNil(t, watchdog)

	ResetFirstTokenWatchdog(c, "test complete")
}

func TestResetFirstTokenWatchdogPreventsPreviousAttemptFromTimingOut(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{
		IsStream:    true,
		ChannelMeta: &relaycommon.ChannelMeta{},
	}

	watchdog := EnsureFirstTokenWatchdog(c, info, 8, nil)
	require.NotNil(t, watchdog)

	ResetFirstTokenWatchdog(c, "switching channel attempt")
	watchdog.triggerTimeout() // Simulate the old timer racing with the retry setup.

	require.False(t, HasFirstTokenTimeout(c))
	stored, exists := c.Get(string(constant.ContextKeyFirstTokenWatchdog))
	require.True(t, exists)
	require.Nil(t, stored)
}
