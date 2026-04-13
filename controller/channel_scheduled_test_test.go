package controller

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func intPtr(v int) *int {
	return &v
}

func TestScheduledTestFirstTokenLatencyMs_AllowsZero(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	c.Set("first_token_latency_ms", 0)

	latency, ok := scheduledTestFirstTokenLatencyMs(c)
	if !ok {
		t.Fatal("expected zero latency to be treated as a measured value")
	}
	if latency != 0 {
		t.Fatalf("expected latency=0, got %d", latency)
	}
}

func TestScheduledTestFirstTokenLatencyMs_MissingValue(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)

	if latency, ok := scheduledTestFirstTokenLatencyMs(c); ok {
		t.Fatalf("expected missing latency to be treated as absent, got latency=%d", latency)
	}
}

func TestDispatchScheduledTestsCycle_SkipsWhenSystemIdle(t *testing.T) {
	origHasRecent := hasRecentLLMRequestForScheduledTest
	origDispatch := scheduledTestDispatchFunc
	defer func() {
		hasRecentLLMRequestForScheduledTest = origHasRecent
		scheduledTestDispatchFunc = origDispatch
	}()

	hasRecentLLMRequestForScheduledTest = func() bool {
		return false
	}

	dispatched := 0
	scheduledTestDispatchFunc = func(channel *model.Channel) {
		dispatched++
	}

	nextTestTimes := make(map[int]int64)
	now := time.Now().Unix()
	channels := []*model.Channel{
		{Id: 113, Name: "idle-probe", ScheduledTestInterval: intPtr(5)},
	}

	dispatchScheduledTestsCycle(channels, nextTestTimes, now)

	if dispatched != 0 {
		t.Fatalf("expected no dispatch while system idle, got %d", dispatched)
	}
	if _, exists := nextTestTimes[113]; exists {
		t.Fatal("expected next test time not to be updated while system idle")
	}
}

func TestDispatchScheduledTestsCycle_DispatchesWhenRecentTrafficAndDue(t *testing.T) {
	origHasRecent := hasRecentLLMRequestForScheduledTest
	origDispatch := scheduledTestDispatchFunc
	defer func() {
		hasRecentLLMRequestForScheduledTest = origHasRecent
		scheduledTestDispatchFunc = origDispatch
	}()

	hasRecentLLMRequestForScheduledTest = func() bool {
		return true
	}

	dispatched := 0
	scheduledTestDispatchFunc = func(channel *model.Channel) {
		dispatched++
	}

	nextTestTimes := make(map[int]int64)
	now := time.Now().Unix()
	channels := []*model.Channel{
		{Id: 131, Name: "active-probe", ScheduledTestInterval: intPtr(5)},
		{Id: 999, Name: "disabled-interval", ScheduledTestInterval: intPtr(0)},
	}

	dispatchScheduledTestsCycle(channels, nextTestTimes, now)
	if dispatched != 1 {
		t.Fatalf("expected exactly one dispatch on first due cycle, got %d", dispatched)
	}

	if _, exists := nextTestTimes[131]; !exists {
		t.Fatal("expected next test time to be set for dispatched channel")
	}

	// Not due yet (within interval)
	dispatchScheduledTestsCycle(channels, nextTestTimes, now+120)
	if dispatched != 1 {
		t.Fatalf("expected no extra dispatch before interval elapses, got %d", dispatched)
	}

	// Due again after interval
	dispatchScheduledTestsCycle(channels, nextTestTimes, now+301)
	if dispatched != 2 {
		t.Fatalf("expected second dispatch after interval elapsed, got %d", dispatched)
	}
}
