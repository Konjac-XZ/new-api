package helper

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/channelcache"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

type FirstTokenWatchdog struct {
	c           *gin.Context
	limit       time.Duration
	start       time.Time
	timer       *time.Timer
	ctx         context.Context
	cancel      context.CancelFunc
	reasonMu    sync.Mutex
	reason      string
	reqCancelMu sync.Mutex
	reqCancel   context.CancelFunc
	respMu      sync.Mutex
	resp        *http.Response
	channelInfo string
	state       atomic.Int32
}

const (
	firstTokenWatchdogStateRunning int32 = iota
	firstTokenWatchdogStateTimedOut
	firstTokenWatchdogStateStopped
)

func NewFirstTokenWatchdog(c *gin.Context, info *relaycommon.RelayInfo, limitSeconds int, reqCancel context.CancelFunc) *FirstTokenWatchdog {
	if c == nil || info == nil || limitSeconds <= 0 || !info.IsStream {
		return nil
	}

	channelInfo := ""
	if info.ChannelMeta != nil {
		channelType := info.ChannelMeta.ChannelType
		channelName := channelcache.NameOr(info.ChannelMeta.ChannelId, constant.ChannelTypeNames[channelType])
		if channelName == "" {
			channelName = "Unknown"
		}
		channelInfo = fmt.Sprintf(" (channel #%d %s)", info.ChannelMeta.ChannelId, channelName)
	}

	common.SetContextKey(c, constant.ContextKeyFirstTokenLatencyExceeded, false)

	ctx, cancel := context.WithCancel(context.Background())
	watchdog := &FirstTokenWatchdog{
		c:           c,
		limit:       time.Duration(limitSeconds) * time.Second,
		start:       time.Now(),
		timer:       time.NewTimer(time.Duration(limitSeconds) * time.Second),
		ctx:         ctx,
		cancel:      cancel,
		reqCancel:   reqCancel,
		channelInfo: channelInfo,
	}
	watchdog.state.Store(firstTokenWatchdogStateRunning)

	go watchdog.run()

	return watchdog
}

func (w *FirstTokenWatchdog) isRunning() bool {
    if w == nil {
        return false
    }
    return w.state.Load() == firstTokenWatchdogStateRunning
}

func (w *FirstTokenWatchdog) run() {
	if w == nil {
		return
	}

	defer func() {
		if !w.timer.Stop() {
			select {
			case <-w.timer.C:
			default:
			}
		}
	}()

	for {
		select {
		case <-w.timer.C:
			w.triggerTimeout()
			return
		case <-w.ctx.Done():
			reason := w.getReason()
			if reason == "" {
				reason = "watchdog canceled"
			}
			elapsed := time.Since(w.start)
			logger.LogInfo(w.c, fmt.Sprintf("first token watchdog canceled after %dms%s (%s)", elapsed.Milliseconds(), w.channelInfo, reason))
			return
		case <-w.c.Request.Context().Done():
			if w.setReasonIfEmpty("client context done") {
				w.state.CompareAndSwap(firstTokenWatchdogStateRunning, firstTokenWatchdogStateStopped)
				if !w.timer.Stop() {
					select {
					case <-w.timer.C:
					default:
					}
				}
			}
			w.cancel()
			continue
		}
	}
}

func (w *FirstTokenWatchdog) AttachResponse(resp *http.Response) {
	if w == nil {
		return
	}
	w.respMu.Lock()
	defer w.respMu.Unlock()
	w.resp = resp
}

func (w *FirstTokenWatchdog) SetRequestCancel(cancel context.CancelFunc) {
	if w == nil {
		return
	}
	w.reqCancelMu.Lock()
	defer w.reqCancelMu.Unlock()
	w.reqCancel = cancel
}

func (w *FirstTokenWatchdog) Stop(reason string) {
	if w == nil || reason == "" {
		return
	}

	if !w.timer.Stop() {
		w.triggerTimeout()
		return
	}

	if !w.state.CompareAndSwap(firstTokenWatchdogStateRunning, firstTokenWatchdogStateStopped) {
		return
	}

	w.setReasonIfEmpty(reason)
	w.cancel()
}

func (w *FirstTokenWatchdog) setReasonIfEmpty(reason string) bool {
	w.reasonMu.Lock()
	defer w.reasonMu.Unlock()
	if w.reason != "" {
		return false
	}
	w.reason = reason
	return true
}

func (w *FirstTokenWatchdog) getReason() string {
	w.reasonMu.Lock()
	defer w.reasonMu.Unlock()
	return w.reason
}

func (w *FirstTokenWatchdog) closeResponse() {
	w.respMu.Lock()
	resp := w.resp
	w.respMu.Unlock()
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
}

func (w *FirstTokenWatchdog) triggerTimeout() {
	if w == nil {
		return
	}
	if !w.state.CompareAndSwap(firstTokenWatchdogStateRunning, firstTokenWatchdogStateTimedOut) {
		return
	}
	elapsed := time.Since(w.start)
	logger.LogWarn(w.c, fmt.Sprintf("first token watchdog triggered after %dms%s (limit %dms)", elapsed.Milliseconds(), w.channelInfo, w.limit.Milliseconds()))
	common.SetContextKey(w.c, constant.ContextKeyFirstTokenLatencyExceeded, true)
	w.reqCancelMu.Lock()
	reqCancel := w.reqCancel
	w.reqCancelMu.Unlock()
	if reqCancel != nil {
		reqCancel()
	}
	w.closeResponse()
}

func EnsureFirstTokenWatchdog(c *gin.Context, info *relaycommon.RelayInfo, limitSeconds int, reqCancel context.CancelFunc) *FirstTokenWatchdog {
    if c == nil || info == nil || limitSeconds <= 0 || !info.IsStream {
        return nil
    }

    if existing, ok := common.GetContextKeyType[*FirstTokenWatchdog](c, constant.ContextKeyFirstTokenWatchdog); ok && existing != nil {
        // Only reuse a running watchdog within the same attempt.
        // If it's stopped/timed out, replace it to avoid leaking state across attempts.
        if existing.isRunning() {
            if reqCancel != nil {
                existing.SetRequestCancel(reqCancel)
            }
            return existing
        }
        // ensure any lingering resources are closed
        existing.Stop("replacing watchdog for new attempt")
    }

	watchdog := NewFirstTokenWatchdog(c, info, limitSeconds, reqCancel)
	if watchdog != nil {
		common.SetContextKey(c, constant.ContextKeyFirstTokenWatchdog, watchdog)
	}
	return watchdog
}

func HasFirstTokenTimeout(c *gin.Context) bool {
	return common.GetContextKeyBool(c, constant.ContextKeyFirstTokenLatencyExceeded)
}

func FirstTokenLatencyError(info *relaycommon.RelayInfo) *types.NewAPIError {
	limit := 0
	if info != nil && info.ChannelMeta != nil {
		limit = info.ChannelMeta.MaxFirstTokenLatencySeconds
	}
	message := "first token latency exceeded"
	if limit > 0 {
		message = fmt.Sprintf("first token latency exceeded (%ds)", limit)
	}
	return types.NewErrorWithStatusCode(fmt.Errorf(message), types.ErrorCodeChannelFirstTokenLatencyExceeded, http.StatusGatewayTimeout)
}
