package monitor

import (
	"testing"
	"time"
)

func resetMonitorManagerForTest() *Manager {
	manager := GetManager()
	manager.mu.Lock()
	manager.store = NewStore()
	manager.hub = NewHub()
	manager.load = NewLoadState()
	manager.hub.SetStore(manager.store)
	manager.store.SetRealtimeEnabled(false)
	manager.mu.Unlock()
	manager.enabled.Store(true)
	return manager
}

func TestRequestSummaryIncludesMillisecondTimingFields(t *testing.T) {
	startTime := time.Unix(1711456789, 123000000)
	streamingStartedAt := startTime.Add(1500 * time.Millisecond)
	endTime := startTime.Add(3200 * time.Millisecond)

	record := &RequestRecord{
		ID:          "req-summary-ms",
		Status:      StatusProcessing,
		StartTime:   startTime,
		StartTimeMs: startTime.UnixMilli(),
		EndTime:     &endTime,
		EndTimeMs:   endTime.UnixMilli(),
		Downstream:  DownstreamInfo{},
		ChannelAttempts: []ChannelAttempt{{
			Attempt:              1,
			ChannelId:            9,
			ChannelName:          "demo",
			StartedAt:            startTime,
			StartedAtMs:          startTime.UnixMilli(),
			StreamingStartedAt:   &streamingStartedAt,
			StreamingStartedAtMs: streamingStartedAt.UnixMilli(),
			Status:               AttemptStatusStreaming,
		}},
	}

	summary := record.ToSummary()
	if summary.ServerNowMs == 0 {
		t.Fatal("expected server_now_ms to be populated")
	}
	if summary.StartTimeMs != startTime.UnixMilli() {
		t.Fatalf("expected start_time_ms %d, got %d", startTime.UnixMilli(), summary.StartTimeMs)
	}
	if summary.EndTimeMs != endTime.UnixMilli() {
		t.Fatalf("expected end_time_ms %d, got %d", endTime.UnixMilli(), summary.EndTimeMs)
	}
	if summary.CurrentAttemptStartedAtMs != startTime.UnixMilli() {
		t.Fatalf("expected current_attempt_started_at_ms %d, got %d", startTime.UnixMilli(), summary.CurrentAttemptStartedAtMs)
	}
	if summary.CurrentAttemptStreamingStartedAtMs != streamingStartedAt.UnixMilli() {
		t.Fatalf("expected current_attempt_streaming_started_at_ms %d, got %d", streamingStartedAt.UnixMilli(), summary.CurrentAttemptStreamingStartedAtMs)
	}
}

func TestRequestSummaryIncludesRetryCount(t *testing.T) {
	startTime := time.Unix(1711456789, 123000000)

	record := &RequestRecord{
		ID:          "req-summary-retry",
		Status:      StatusProcessing,
		StartTime:   startTime,
		StartTimeMs: startTime.UnixMilli(),
		Downstream:  DownstreamInfo{},
		ChannelAttempts: []ChannelAttempt{
			{
				Attempt:     1,
				ChannelId:   9,
				ChannelName: "demo-a",
				StartedAt:   startTime,
				StartedAtMs: startTime.UnixMilli(),
				Status:      AttemptStatusFailed,
			},
			{
				Attempt:     2,
				ChannelId:   10,
				ChannelName: "demo-b",
				StartedAt:   startTime.Add(time.Second),
				StartedAtMs: startTime.Add(time.Second).UnixMilli(),
				Status:      AttemptStatusFailed,
			},
			{
				Attempt:     3,
				ChannelId:   11,
				ChannelName: "demo-c",
				StartedAt:   startTime.Add(2 * time.Second),
				StartedAtMs: startTime.Add(2 * time.Second).UnixMilli(),
				Status:      AttemptStatusStreaming,
			},
		},
	}

	summary := record.ToSummary()
	if summary.RetryCount != 2 {
		t.Fatalf("expected retry_count 2, got %d", summary.RetryCount)
	}
}

func TestRequestSummaryOmitsRetryCountForFirstAttempt(t *testing.T) {
	startTime := time.Unix(1711456789, 123000000)

	record := &RequestRecord{
		ID:          "req-summary-no-retry",
		Status:      StatusProcessing,
		StartTime:   startTime,
		StartTimeMs: startTime.UnixMilli(),
		Downstream:  DownstreamInfo{},
		ChannelAttempts: []ChannelAttempt{{
			Attempt:     1,
			ChannelId:   9,
			ChannelName: "demo",
			StartedAt:   startTime,
			StartedAtMs: startTime.UnixMilli(),
			Status:      AttemptStatusStreaming,
		}},
	}

	summary := record.ToSummary()
	if summary.RetryCount != 0 {
		t.Fatalf("expected retry_count 0, got %d", summary.RetryCount)
	}
}

func TestRequestSummaryIncludesPromptTokensBeforeResponse(t *testing.T) {
	startTime := time.Unix(1711456789, 123000000)

	record := &RequestRecord{
		ID:           "req-summary-prompt-tokens",
		Status:       StatusProcessing,
		StartTime:    startTime,
		StartTimeMs:  startTime.UnixMilli(),
		Downstream:   DownstreamInfo{},
		PromptTokens: 1234,
	}

	summary := record.ToSummary()
	if summary.PromptTokens != 1234 {
		t.Fatalf("expected prompt_tokens 1234 before response, got %d", summary.PromptTokens)
	}
	if summary.CompletionTokens != 0 {
		t.Fatalf("expected completion_tokens 0 before response, got %d", summary.CompletionTokens)
	}
}

func TestMarkChannelPhaseStreamingSetsStreamingStartedTiming(t *testing.T) {
	resetMonitorManagerForTest()
	store := GetManager().GetStore()
	if store == nil {
		t.Fatal("expected monitor store to be initialized")
	}

	startTime := time.Now().Add(-2 * time.Second)
	store.Add(&RequestRecord{
		ID:          "req-streaming-ms",
		Status:      StatusProcessing,
		StartTime:   startTime,
		StartTimeMs: startTime.UnixMilli(),
		Downstream:  DownstreamInfo{},
	})

	StartChannelAttempt("req-streaming-ms", 7, "channel-a", 1)
	MarkChannelPhase("req-streaming-ms", PhaseStreaming)

	record := store.Get("req-streaming-ms")
	if record == nil {
		t.Fatal("expected request record to exist")
	}
	if len(record.ChannelAttempts) != 1 {
		t.Fatalf("expected 1 channel attempt, got %d", len(record.ChannelAttempts))
	}

	attempt := record.ChannelAttempts[0]
	if attempt.Status != AttemptStatusStreaming {
		t.Fatalf("expected attempt status %q, got %q", AttemptStatusStreaming, attempt.Status)
	}
	if attempt.StreamingStartedAt == nil {
		t.Fatal("expected streaming_started_at to be populated")
	}
	if attempt.StreamingStartedAtMs == 0 {
		t.Fatal("expected streaming_started_at_ms to be populated")
	}
	if attempt.StreamingStartedAtMs < attempt.StartedAtMs {
		t.Fatalf("expected streaming_started_at_ms >= started_at_ms, got %d < %d", attempt.StreamingStartedAtMs, attempt.StartedAtMs)
	}
}
