package monitor

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadStateDegradesWithHysteresis(t *testing.T) {
	load := NewLoadState()

	for i := 0; i <= MonitorDegradeActiveLimit; i++ {
		load.Start(fmt.Sprintf("req-%d", i))
	}

	snapshot := load.Snapshot()
	require.True(t, snapshot.Degraded)
	assert.Equal(t, MonitorDegradeActiveLimit+1, snapshot.ActiveRequests)
	assert.Equal(t, MonitorDegradeActiveLimit, snapshot.Capacity)
	assert.Equal(t, uint64(1), snapshot.DegradationGeneration)

	for i := 0; i < MonitorDegradeActiveLimit-MonitorRecoverActiveLimit; i++ {
		load.Finish(fmt.Sprintf("req-%d", i))
	}
	assert.True(t, load.Snapshot().Degraded)

	load.Finish(fmt.Sprintf("req-%d", MonitorDegradeActiveLimit-MonitorRecoverActiveLimit))
	snapshot = load.Snapshot()
	require.False(t, snapshot.Degraded)
	assert.Equal(t, MonitorRecoverActiveLimit, snapshot.ActiveRequests)
	assert.Equal(t, uint64(1), snapshot.DegradationGeneration)

	for i := 0; i <= MonitorDegradeActiveLimit; i++ {
		load.Start(fmt.Sprintf("req-again-%d", i))
	}
	snapshot = load.Snapshot()
	require.True(t, snapshot.Degraded)
	assert.Equal(t, uint64(2), snapshot.DegradationGeneration)
}

func TestMonitorBodyCapturePolicy(t *testing.T) {
	manager := resetMonitorManagerForTest()
	manager.load = NewLoadState()

	body, truncated := monitorBody([]byte("small body"))
	require.False(t, truncated)
	assert.Equal(t, MonitorBody("small body"), body)

	maxBody := make([]byte, MonitorBodyCaptureMaxBytes)
	body, truncated = monitorBody(maxBody)
	require.False(t, truncated)
	assert.Len(t, body, MonitorBodyCaptureMaxBytes)

	overLimitBody := make([]byte, MonitorBodyCaptureMaxBytes+1)
	body, truncated = monitorBody(overLimitBody)
	require.True(t, truncated)
	assert.Empty(t, body)

	for i := 0; i <= MonitorDegradeActiveLimit; i++ {
		manager.load.Start(fmt.Sprintf("active-%d", i))
	}

	body, truncated = monitorBody([]byte("small body"))
	require.True(t, truncated)
	assert.Empty(t, body)
}

func TestRecordLifecycleUpdatesLoadState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resetMonitorManagerForTest()

	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = request
	c.Set(common.RequestIdKey, "req-lifecycle")

	recordID := RecordStart(c, []byte("request body"), 1)
	require.Equal(t, "req-lifecycle", recordID)

	snapshot := GetLoadSnapshot()
	require.False(t, snapshot.Degraded)
	assert.Equal(t, 1, snapshot.ActiveRequests)

	RecordResponse(recordID, http.StatusOK, nil, []byte("response body"), 1, 2, nil)

	snapshot = GetLoadSnapshot()
	require.False(t, snapshot.Degraded)
	assert.Equal(t, 0, snapshot.ActiveRequests)
}
