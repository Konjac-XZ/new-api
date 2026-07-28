package helper

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

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
