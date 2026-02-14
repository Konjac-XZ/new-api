package monitor

import (
	"encoding/json"
	"time"
)

// MonitorBody stores body content as raw bytes in memory.
// It is serialized as a UTF-8 JSON string only when APIs render records.
type MonitorBody []byte

func (body MonitorBody) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(body))
}

func (body MonitorBody) String() string {
	return string(body)
}

// Status constants for request records
const (
	StatusPending         = "pending"
	StatusProcessing      = "processing"
	StatusWaitingUpstream = "waiting_upstream"
	StatusStreaming       = "streaming"
	StatusCompleted       = "completed"
	StatusError           = "error"
)

// Channel phases (real-time state of upstream interaction)
const (
	PhaseWaitingUpstream = "waiting_upstream"
	PhaseStreaming       = "streaming"
	PhaseError           = "error"
	PhaseCompleted       = "completed"
)

// Channel attempt status values
const (
	AttemptStatusWaiting   = "waiting_upstream"
	AttemptStatusStreaming = "streaming"
	AttemptStatusFailed    = "failed"
	AttemptStatusAbandoned = "abandoned"
	AttemptStatusSucceeded = "succeeded"
)

// RequestRecord represents a single API request being monitored
type RequestRecord struct {
	ID        string     `json:"id"`
	Status    string     `json:"status"`
	StartTime time.Time  `json:"start_time"`
	EndTime   *time.Time `json:"end_time,omitempty"`
	Duration  int64      `json:"duration_ms,omitempty"`

	// Downstream (client) request info
	Downstream DownstreamInfo `json:"downstream"`

	// Upstream (provider) request info
	Upstream *UpstreamInfo `json:"upstream,omitempty"`

	// Response info
	Response *ResponseInfo `json:"response,omitempty"`

	// Metadata
	UserId      int    `json:"user_id"`
	TokenId     int    `json:"token_id"`
	TokenName   string `json:"token_name"`
	ChannelId   int    `json:"channel_id"`
	ChannelName string `json:"channel_name"`
	Model       string `json:"model"`
	IsStream    bool   `json:"is_stream"`

	// Channel switching / retry info
	CurrentPhase    string           `json:"current_phase,omitempty"`
	CurrentChannel  *CurrentChannel  `json:"current_channel,omitempty"`
	ChannelAttempts []ChannelAttempt `json:"channel_attempts,omitempty"`
}

// CurrentChannel describes which upstream channel is being used right now
type CurrentChannel struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Attempt int    `json:"attempt"`
}

// ChannelAttempt captures a single try against a specific channel
type ChannelAttempt struct {
	Attempt            int        `json:"attempt"`
	ChannelId          int        `json:"channel_id"`
	ChannelName        string     `json:"channel_name"`
	StartedAt          time.Time  `json:"started_at"`
	StreamingStartedAt *time.Time `json:"streaming_started_at,omitempty"`
	EndedAt            *time.Time `json:"ended_at,omitempty"`
	Status             string     `json:"status"` // waiting_upstream | streaming | failed | abandoned | succeeded
	Reason             string     `json:"reason,omitempty"`
	ErrorCode          string     `json:"error_code,omitempty"`
	HTTPStatus         int        `json:"http_status,omitempty"`
}

// DownstreamInfo contains information about the client request
type DownstreamInfo struct {
	Method        string            `json:"method"`
	Path          string            `json:"path"`
	Headers       map[string]string `json:"headers"`
	Body          MonitorBody       `json:"body"`
	BodySize      int               `json:"body_size"`
	BodyTruncated bool              `json:"body_truncated"`
	ClientIP      string            `json:"client_ip"`
}

// UpstreamInfo contains information about the request sent to the provider
type UpstreamInfo struct {
	URL           string            `json:"url"`
	Method        string            `json:"method"`
	Headers       map[string]string `json:"headers"`
	Body          MonitorBody       `json:"body"`
	BodySize      int               `json:"body_size"`
	BodyTruncated bool              `json:"body_truncated"`
}

// ResponseInfo contains information about the response
type ResponseInfo struct {
	StatusCode       int               `json:"status_code"`
	Headers          map[string]string `json:"headers"`
	Body             MonitorBody       `json:"body"`
	BodySize         int               `json:"body_size"`
	BodyTruncated    bool              `json:"body_truncated"`
	Error            *ErrorInfo        `json:"error,omitempty"`
	PromptTokens     int               `json:"prompt_tokens,omitempty"`
	CompletionTokens int               `json:"completion_tokens,omitempty"`
}

// ErrorInfo contains error details
type ErrorInfo struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// WSMessage represents a WebSocket message
type WSMessage struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

// WSMessageType constants
const (
	WSMessageTypeNew      = "new"
	WSMessageTypeUpdate   = "update"
	WSMessageTypeDelete   = "delete"
	WSMessageTypeSnapshot = "snapshot"
	WSMessageTypeChannel  = "channel_update"
)

// ChannelUpdate is sent over websocket when upstream channel/phase changes
type ChannelUpdate struct {
	RequestID       string           `json:"request_id"`
	CurrentPhase    string           `json:"current_phase,omitempty"`
	CurrentChannel  *CurrentChannel  `json:"current_channel,omitempty"`
	ChannelAttempts []ChannelAttempt `json:"channel_attempts,omitempty"`
}

// MonitorStats contains monitoring statistics
type MonitorStats struct {
	TotalRequests  int   `json:"total_requests"`
	ActiveRequests int   `json:"active_requests"`
	Completed      int   `json:"completed"`
	Errors         int   `json:"errors"`
	MemoryBytes    int64 `json:"memory_bytes"`
}

// RequestSummary is a lightweight version of RequestRecord for WebSocket broadcasts
// It excludes large body data to reduce bandwidth
type RequestSummary struct {
	ID         string     `json:"id"`
	Status     string     `json:"status"`
	StartTime  time.Time  `json:"start_time"`
	EndTime    *time.Time `json:"end_time,omitempty"`
	DurationMs int64      `json:"duration_ms,omitempty"`

	// Lightweight downstream info (no body/headers)
	Method   string `json:"method"`
	Path     string `json:"path"`
	ClientIP string `json:"client_ip"`

	// Metadata
	UserId         int             `json:"user_id"`
	TokenId        int             `json:"token_id"`
	TokenName      string          `json:"token_name"`
	ChannelId      int             `json:"channel_id"`
	ChannelName    string          `json:"channel_name"`
	Model          string          `json:"model"`
	IsStream       bool            `json:"is_stream"`
	CurrentPhase   string          `json:"current_phase,omitempty"`
	CurrentChannel *CurrentChannel `json:"current_channel,omitempty"`

	// Response summary (no body/headers)
	StatusCode       int  `json:"status_code,omitempty"`
	HasError         bool `json:"has_error"`
	PromptTokens     int  `json:"prompt_tokens,omitempty"`
	CompletionTokens int  `json:"completion_tokens,omitempty"`
}

// recordSnapshot holds only the scalar/value-type fields needed to build a
// RequestSummary. It is captured under the write lock (flat copy, no heap
// allocations) so that the actual RequestSummary can be constructed outside
// the lock.
type recordSnapshot struct {
	ID           string
	Status       string
	StartTime    time.Time
	EndTime      *time.Time
	Duration     int64
	Method       string
	Path         string
	ClientIP     string
	UserId       int
	TokenId      int
	TokenName    string
	ChannelId    int
	ChannelName  string
	Model        string
	IsStream     bool
	CurrentPhase string

	// CurrentChannel is copied by value (3 small fields).
	HasCurrentChannel bool
	CurrentChannelID  int
	CurrentChannelNm  string
	CurrentChannelAtt int

	// Response scalars
	HasResponse      bool
	StatusCode       int
	HasError         bool
	PromptTokens     int
	CompletionTokens int
}

// snapshotRecord captures a flat copy of the fields needed for RequestSummary.
// Must be called while the caller holds the store lock.
func snapshotRecord(r *RequestRecord) recordSnapshot {
	snap := recordSnapshot{
		ID:           r.ID,
		Status:       r.Status,
		StartTime:    r.StartTime,
		EndTime:      r.EndTime,
		Duration:     r.Duration,
		Method:       r.Downstream.Method,
		Path:         r.Downstream.Path,
		ClientIP:     r.Downstream.ClientIP,
		UserId:       r.UserId,
		TokenId:      r.TokenId,
		TokenName:    r.TokenName,
		ChannelId:    r.ChannelId,
		ChannelName:  r.ChannelName,
		Model:        r.Model,
		IsStream:     r.IsStream,
		CurrentPhase: r.CurrentPhase,
	}
	if r.CurrentChannel != nil {
		snap.HasCurrentChannel = true
		snap.CurrentChannelID = r.CurrentChannel.ID
		snap.CurrentChannelNm = r.CurrentChannel.Name
		snap.CurrentChannelAtt = r.CurrentChannel.Attempt
	}
	if r.Response != nil {
		snap.HasResponse = true
		snap.StatusCode = r.Response.StatusCode
		snap.HasError = r.Response.Error != nil
		snap.PromptTokens = r.Response.PromptTokens
		snap.CompletionTokens = r.Response.CompletionTokens
	}
	return snap
}

// toSummary builds a RequestSummary from the snapshot. Safe to call without
// any lock held.
func (snap *recordSnapshot) toSummary() *RequestSummary {
	s := &RequestSummary{
		ID:           snap.ID,
		Status:       snap.Status,
		StartTime:    snap.StartTime,
		EndTime:      snap.EndTime,
		DurationMs:   snap.Duration,
		Method:       snap.Method,
		Path:         snap.Path,
		ClientIP:     snap.ClientIP,
		UserId:       snap.UserId,
		TokenId:      snap.TokenId,
		TokenName:    snap.TokenName,
		ChannelId:    snap.ChannelId,
		ChannelName:  snap.ChannelName,
		Model:        snap.Model,
		IsStream:     snap.IsStream,
		CurrentPhase: snap.CurrentPhase,
	}
	if snap.HasCurrentChannel {
		s.CurrentChannel = &CurrentChannel{
			ID:      snap.CurrentChannelID,
			Name:    snap.CurrentChannelNm,
			Attempt: snap.CurrentChannelAtt,
		}
	}
	if snap.HasResponse {
		s.StatusCode = snap.StatusCode
		s.HasError = snap.HasError
		s.PromptTokens = snap.PromptTokens
		s.CompletionTokens = snap.CompletionTokens
	}
	return s
}

// ToSummary converts a full RequestRecord to a lightweight RequestSummary
func (r *RequestRecord) ToSummary() *RequestSummary {
	summary := &RequestSummary{
		ID:             r.ID,
		Status:         r.Status,
		StartTime:      r.StartTime,
		EndTime:        r.EndTime,
		DurationMs:     r.Duration,
		Method:         r.Downstream.Method,
		Path:           r.Downstream.Path,
		ClientIP:       r.Downstream.ClientIP,
		UserId:         r.UserId,
		TokenId:        r.TokenId,
		TokenName:      r.TokenName,
		ChannelId:      r.ChannelId,
		ChannelName:    r.ChannelName,
		Model:          r.Model,
		IsStream:       r.IsStream,
		CurrentPhase:   r.CurrentPhase,
		CurrentChannel: cloneCurrentChannel(r.CurrentChannel),
	}

	if r.Response != nil {
		summary.StatusCode = r.Response.StatusCode
		summary.HasError = r.Response.Error != nil
		summary.PromptTokens = r.Response.PromptTokens
		summary.CompletionTokens = r.Response.CompletionTokens
	}

	return summary
}

// ToChannelUpdate builds a lightweight payload describing channel attempts
func (r *RequestRecord) ToChannelUpdate() *ChannelUpdate {
	return &ChannelUpdate{
		RequestID:       r.ID,
		CurrentPhase:    r.CurrentPhase,
		CurrentChannel:  cloneCurrentChannel(r.CurrentChannel),
		ChannelAttempts: cloneChannelAttempts(r.ChannelAttempts),
	}
}

func cloneCurrentChannel(channel *CurrentChannel) *CurrentChannel {
	if channel == nil {
		return nil
	}
	cloned := *channel
	return &cloned
}

func cloneChannelAttempts(attempts []ChannelAttempt) []ChannelAttempt {
	if len(attempts) == 0 {
		return nil
	}
	cloned := make([]ChannelAttempt, len(attempts))
	for i, attempt := range attempts {
		cloned[i] = attempt
		if attempt.StreamingStartedAt != nil {
			startedAt := *attempt.StreamingStartedAt
			cloned[i].StreamingStartedAt = &startedAt
		}
		if attempt.EndedAt != nil {
			endedAt := *attempt.EndedAt
			cloned[i].EndedAt = &endedAt
		}
	}
	return cloned
}

// cloneChannelAttemptsForUpdate clones all attempts, but deep-copies time pointers
// only for the last attempt (the only one that can still change).
func cloneChannelAttemptsForUpdate(attempts []ChannelAttempt) []ChannelAttempt {
	if len(attempts) == 0 {
		return nil
	}
	cloned := make([]ChannelAttempt, len(attempts))
	copy(cloned, attempts)
	last := len(attempts) - 1
	if attempts[last].StreamingStartedAt != nil {
		startedAt := *attempts[last].StreamingStartedAt
		cloned[last].StreamingStartedAt = &startedAt
	}
	if attempts[last].EndedAt != nil {
		endedAt := *attempts[last].EndedAt
		cloned[last].EndedAt = &endedAt
	}
	return cloned
}

// EstimateSize returns approximate memory size in bytes for this RequestRecord
func (r *RequestRecord) EstimateSize() int64 {
	var size int64

	// Basic struct fields overhead
	size += 200

	// String fields
	size += int64(len(r.ID))
	size += int64(len(r.Status))
	size += int64(len(r.TokenName))
	size += int64(len(r.ChannelName))
	size += int64(len(r.Model))
	size += int64(len(r.CurrentPhase))

	// Downstream info
	size += int64(len(r.Downstream.Method))
	size += int64(len(r.Downstream.Path))
	size += int64(len(r.Downstream.Body))
	size += int64(len(r.Downstream.ClientIP))
	for k, v := range r.Downstream.Headers {
		size += int64(len(k) + len(v))
	}

	// Upstream info
	if r.Upstream != nil {
		size += int64(len(r.Upstream.URL))
		size += int64(len(r.Upstream.Method))
		size += int64(len(r.Upstream.Body))
		for k, v := range r.Upstream.Headers {
			size += int64(len(k) + len(v))
		}
	}

	// Response info
	if r.Response != nil {
		size += int64(len(r.Response.Body))
		for k, v := range r.Response.Headers {
			size += int64(len(k) + len(v))
		}
		if r.Response.Error != nil {
			size += int64(len(r.Response.Error.Code))
			size += int64(len(r.Response.Error.Message))
		}
	}

	// CurrentChannel
	if r.CurrentChannel != nil {
		size += int64(len(r.CurrentChannel.Name))
		size += 50 // overhead
	}

	// Channel attempts
	for _, attempt := range r.ChannelAttempts {
		size += int64(len(attempt.ChannelName))
		size += int64(len(attempt.Reason))
		size += int64(len(attempt.ErrorCode))
		size += 100 // overhead
	}

	return size
}
