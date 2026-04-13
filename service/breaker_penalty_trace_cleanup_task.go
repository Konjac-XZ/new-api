package service

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"

	"github.com/bytedance/gopkg/util/gopool"
)

const (
	breakerPenaltyTraceCleanupTickInterval = 30 * time.Minute
	breakerPenaltyTraceRetentionSeconds    = 90 * 24 * 3600
	breakerPenaltyTraceCleanupBatchSize    = 1000
)

var (
	breakerPenaltyTraceCleanupOnce    sync.Once
	breakerPenaltyTraceCleanupRunning atomic.Bool
)

func StartBreakerPenaltyTraceCleanupTask() {
	breakerPenaltyTraceCleanupOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}
		gopool.Go(func() {
			logger.LogInfo(context.Background(), fmt.Sprintf("breaker penalty trace cleanup task started: tick=%s retention_days=%d", breakerPenaltyTraceCleanupTickInterval, 90))
			ticker := time.NewTicker(breakerPenaltyTraceCleanupTickInterval)
			defer ticker.Stop()

			runBreakerPenaltyTraceCleanupOnce()
			for range ticker.C {
				runBreakerPenaltyTraceCleanupOnce()
			}
		})
	})
}

func runBreakerPenaltyTraceCleanupOnce() {
	if !breakerPenaltyTraceCleanupRunning.CompareAndSwap(false, true) {
		return
	}
	defer breakerPenaltyTraceCleanupRunning.Store(false)

	deleted, err := model.CleanupBreakerPenaltyTraceRecords(breakerPenaltyTraceRetentionSeconds, breakerPenaltyTraceCleanupBatchSize)
	if err != nil {
		logger.LogWarn(context.Background(), fmt.Sprintf("breaker penalty trace cleanup failed: %v", err))
		return
	}
	if common.DebugEnabled && deleted > 0 {
		logger.LogDebug(context.Background(), "breaker penalty trace cleanup: deleted=%d", deleted)
	}
}
