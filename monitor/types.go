package monitor

import (
	"time"
)

// Status constants for request records
const (
	StatusPending    = "pending"
	StatusProcessing = "processing"
	StatusCompleted  = "completed"
	StatusError      = "error"
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
}

// DownstreamInfo contains information about the client request
type DownstreamInfo struct {
	Method   string            `json:"method"`
	Path     string            `json:"path"`
	Headers  map[string]string `json:"headers"`
	Body     string            `json:"body"`
	BodySize int               `json:"body_size"`
	ClientIP string            `json:"client_ip"`
}

// UpstreamInfo contains information about the request sent to the provider
type UpstreamInfo struct {
	URL      string            `json:"url"`
	Method   string            `json:"method"`
	Headers  map[string]string `json:"headers"`
	Body     string            `json:"body"`
	BodySize int               `json:"body_size"`
}

// ResponseInfo contains information about the response
type ResponseInfo struct {
	StatusCode       int               `json:"status_code"`
	Headers          map[string]string `json:"headers"`
	Body             string            `json:"body"`
	BodySize         int               `json:"body_size"`
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
)

// MonitorStats contains monitoring statistics
type MonitorStats struct {
	TotalRequests  int `json:"total_requests"`
	ActiveRequests int `json:"active_requests"`
	Completed      int `json:"completed"`
	Errors         int `json:"errors"`
}

// RequestSummary is a lightweight version of RequestRecord for WebSocket broadcasts
// It excludes large body data to reduce bandwidth
type RequestSummary struct {
	ID        string     `json:"id"`
	Status    string     `json:"status"`
	StartTime time.Time  `json:"start_time"`
	EndTime   *time.Time `json:"end_time,omitempty"`
	DurationMs int64     `json:"duration_ms,omitempty"`

	// Lightweight downstream info (no body/headers)
	Method   string `json:"method"`
	Path     string `json:"path"`
	ClientIP string `json:"client_ip"`

	// Metadata
	UserId      int    `json:"user_id"`
	TokenId     int    `json:"token_id"`
	TokenName   string `json:"token_name"`
	ChannelId   int    `json:"channel_id"`
	ChannelName string `json:"channel_name"`
	Model       string `json:"model"`
	IsStream    bool   `json:"is_stream"`

	// Response summary (no body/headers)
	StatusCode       int  `json:"status_code,omitempty"`
	HasError         bool `json:"has_error"`
	PromptTokens     int  `json:"prompt_tokens,omitempty"`
	CompletionTokens int  `json:"completion_tokens,omitempty"`
}

// ToSummary converts a full RequestRecord to a lightweight RequestSummary
func (r *RequestRecord) ToSummary() *RequestSummary {
	summary := &RequestSummary{
		ID:          r.ID,
		Status:      r.Status,
		StartTime:   r.StartTime,
		EndTime:     r.EndTime,
		DurationMs:  r.Duration,
		Method:      r.Downstream.Method,
		Path:        r.Downstream.Path,
		ClientIP:    r.Downstream.ClientIP,
		UserId:      r.UserId,
		TokenId:     r.TokenId,
		TokenName:   r.TokenName,
		ChannelId:   r.ChannelId,
		ChannelName: r.ChannelName,
		Model:       r.Model,
		IsStream:    r.IsStream,
	}

	if r.Response != nil {
		summary.StatusCode = r.Response.StatusCode
		summary.HasError = r.Response.Error != nil
		summary.PromptTokens = r.Response.PromptTokens
		summary.CompletionTokens = r.Response.CompletionTokens
	}

	return summary
}
