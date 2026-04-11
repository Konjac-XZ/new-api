package controller

import (
	"testing"

	"github.com/gin-gonic/gin"
)

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