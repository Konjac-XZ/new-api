package monitor

import (
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

// Init initializes the monitor system
func Init() {
	GetManager().Init()
}

// IsEnabled returns whether monitoring is GetManager().IsEnabled()
func IsEnabled() bool {
	return GetManager().IsEnabled()
}

// SetEnabled enables or disables monitoring
func SetEnabled(e bool) {
	GetManager().SetEnabled(e)
}

// GetStore returns the store (for testing)
func GetStore() *Store {
	return GetManager().GetStore()
}

// GetHub returns the hub (for testing)
func GetHub() *Hub {
	return GetManager().GetHub()
}

// sensitiveHeaders are headers that should be masked
var sensitiveHeaders = map[string]bool{
	"authorization":       true,
	"x-api-key":           true,
	"api-key":             true,
	"x-goog-api-key":      true,
	"proxy-authorization": true,
}

// maskHeader masks sensitive header values
func maskHeader(key, value string) string {
	if sensitiveHeaders[strings.ToLower(key)] {
		if len(value) > 8 {
			return value[:4] + "****" + value[len(value)-4:]
		}
		return "****"
	}
	return value
}

// headersToMap converts http.Header to map[string]string with masking
func headersToMap(headers http.Header) map[string]string {
	result := make(map[string]string)
	for key, values := range headers {
		if len(values) > 0 {
			result[key] = maskHeader(key, values[0])
		}
	}
	return result
}

// ginHeadersToMap converts gin request headers to map[string]string with masking
func ginHeadersToMap(c *gin.Context) map[string]string {
	result := make(map[string]string)
	for key, values := range c.Request.Header {
		if len(values) > 0 {
			result[key] = maskHeader(key, values[0])
		}
	}
	return result
}

// truncatedBody returns full body and indicates no truncation
func truncatedBody(body []byte) (string, bool) {
	return string(body), false
}

// RecordStart records the start of a request
// Returns the record ID for subsequent updates
func RecordStart(c *gin.Context, requestBody []byte) string {
	if !GetManager().IsEnabled() || GetManager().GetStore() == nil {
		return ""
	}

	requestId := c.GetString(common.RequestIdKey)

	// Get metadata from context
	userId := c.GetInt("id")
	tokenId := c.GetInt("token_id")
	tokenName := c.GetString("token_name")
	model := c.GetString("original_model")

	bodyStr, truncated := truncatedBody(requestBody)

	record := &RequestRecord{
		ID:        requestId,
		Status:    StatusProcessing,
		StartTime: time.Now(),
		Downstream: DownstreamInfo{
			Method:        c.Request.Method,
			Path:          c.Request.URL.Path,
			Headers:       ginHeadersToMap(c),
			Body:          bodyStr,
			BodySize:      len(requestBody),
			BodyTruncated: truncated,
			ClientIP:      c.ClientIP(),
		},
		UserId:    userId,
		TokenId:   tokenId,
		TokenName: tokenName,
		Model:     model,
	}

	GetManager().GetStore().Add(record)
	return requestId
}

// RecordUpstream records the upstream request details
func RecordUpstream(recordID string, url string, method string, headers http.Header, body []byte) {
	if !GetManager().IsEnabled() || GetManager().GetStore() == nil || recordID == "" {
		return
	}

	bodyStr, truncated := truncatedBody(body)
	GetManager().GetStore().Update(recordID, func(r *RequestRecord) {
		r.Upstream = &UpstreamInfo{
			URL:           url,
			Method:        method,
			Headers:       headersToMap(headers),
			Body:          bodyStr,
			BodySize:      len(body),
			BodyTruncated: truncated,
		}
	})
}

// RecordUpstreamWithContext records upstream request using gin context
func RecordUpstreamWithContext(c *gin.Context, url string, method string, headers http.Header, body []byte) {
	recordID := c.GetString("monitor_id")
	if recordID == "" {
		return
	}
	RecordUpstream(recordID, url, method, headers, body)
}

// RecordResponse records the response details.
// Uses BatchUpdate to apply all mutations under a single write lock.
func RecordResponse(recordID string, statusCode int, headers http.Header, body []byte, promptTokens, completionTokens int, err error) {
	if !GetManager().IsEnabled() || GetManager().GetStore() == nil || recordID == "" {
		return
	}

	bodyStr, truncated := truncatedBody(body)
	response := &ResponseInfo{
		StatusCode:       statusCode,
		Headers:          headersToMap(headers),
		Body:             bodyStr,
		BodySize:         len(body),
		BodyTruncated:    truncated,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
	}

	if err != nil {
		response.Error = &ErrorInfo{Message: err.Error()}
	}

	// Inline MarkComplete logic
	markComplete := func(r *RequestRecord) {
		now := time.Now()
		r.EndTime = &now
		r.Duration = now.Sub(r.StartTime).Milliseconds()
		r.Response = response
		if response.Error != nil {
			r.Status = StatusError
		} else {
			r.Status = StatusCompleted
		}
	}

	// Inline MarkChannelPhase logic
	var phase string
	if err != nil {
		phase = PhaseError
	} else {
		phase = PhaseCompleted
	}
	markPhase := func(r *RequestRecord) {
		r.CurrentPhase = phase
		if len(r.ChannelAttempts) == 0 {
			return
		}
		last := &r.ChannelAttempts[len(r.ChannelAttempts)-1]
		switch phase {
		case PhaseCompleted:
			if last.EndedAt == nil {
				now := time.Now()
				last.EndedAt = &now
			}
			last.Status = AttemptStatusSucceeded
		case PhaseError:
			if last.EndedAt == nil {
				now := time.Now()
				last.EndedAt = &now
			}
			if last.Status != AttemptStatusSucceeded {
				last.Status = AttemptStatusFailed
			}
		}
	}

	// Inline FinishChannelAttempt logic
	var attemptStatus, reason string
	if err != nil {
		attemptStatus = AttemptStatusFailed
		reason = err.Error()
	} else {
		attemptStatus = AttemptStatusSucceeded
	}
	finishAttempt := func(r *RequestRecord) {
		if len(r.ChannelAttempts) == 0 {
			return
		}
		last := &r.ChannelAttempts[len(r.ChannelAttempts)-1]
		if last.EndedAt != nil {
			return
		}
		now := time.Now()
		last.EndedAt = &now
		last.Status = attemptStatus
		last.Reason = reason
		last.HTTPStatus = statusCode
	}

	GetManager().GetStore().BatchUpdate(recordID, true, markComplete, markPhase, finishAttempt)
}

// RecordResponseWithContext records response using gin context
func RecordResponseWithContext(c *gin.Context, statusCode int, headers http.Header, body []byte, promptTokens, completionTokens int, err error) {
	recordID := c.GetString("monitor_id")
	if recordID == "" {
		return
	}
	RecordResponse(recordID, statusCode, headers, body, promptTokens, completionTokens, err)
}

// RecordError records an error for a request.
// Uses BatchUpdate to apply all mutations under a single write lock.
func RecordError(recordID string, err error) {
	if !GetManager().IsEnabled() || GetManager().GetStore() == nil || recordID == "" {
		return
	}

	errMsg := err.Error()

	// Inline the error recording logic
	markError := func(r *RequestRecord) {
		now := time.Now()
		r.EndTime = &now
		r.Duration = now.Sub(r.StartTime).Milliseconds()
		r.Status = StatusError
		if r.Response == nil {
			r.Response = &ResponseInfo{}
		}
		r.Response.Error = &ErrorInfo{Message: errMsg}
	}

	// Inline FinishChannelAttempt logic
	finishAttempt := func(r *RequestRecord) {
		if len(r.ChannelAttempts) == 0 {
			return
		}
		last := &r.ChannelAttempts[len(r.ChannelAttempts)-1]
		if last.EndedAt != nil {
			return
		}
		now := time.Now()
		last.EndedAt = &now
		last.Status = AttemptStatusFailed
		last.Reason = errMsg
	}

	// Inline MarkChannelPhase(PhaseError) logic
	markPhase := func(r *RequestRecord) {
		r.CurrentPhase = PhaseError
		if len(r.ChannelAttempts) == 0 {
			return
		}
		last := &r.ChannelAttempts[len(r.ChannelAttempts)-1]
		if last.EndedAt == nil {
			now := time.Now()
			last.EndedAt = &now
		}
		if last.Status != AttemptStatusSucceeded {
			last.Status = AttemptStatusFailed
		}
	}

	GetManager().GetStore().BatchUpdate(recordID, true, markError, finishAttempt, markPhase)
}

// RecordErrorWithContext records an error using gin context
func RecordErrorWithContext(c *gin.Context, err error) {
	recordID := c.GetString("monitor_id")
	if recordID == "" {
		return
	}
	RecordError(recordID, err)
}

// StartChannelAttempt records that we are about to try a specific channel.
// Uses UpdateAndBroadcastChannel for a single lock acquisition.
func StartChannelAttempt(recordID string, channelId int, channelName string, attemptNo int) {
	if !GetManager().IsEnabled() || GetManager().GetStore() == nil || recordID == "" {
		return
	}

	now := time.Now()
	GetManager().GetStore().UpdateAndBroadcastChannel(recordID, func(r *RequestRecord) {
		attempt := ChannelAttempt{
			Attempt:     attemptNo,
			ChannelId:   channelId,
			ChannelName: channelName,
			StartedAt:   now,
			Status:      AttemptStatusWaiting,
		}
		r.ChannelAttempts = append(r.ChannelAttempts, attempt)
		r.ChannelId = channelId
		r.ChannelName = channelName
		r.CurrentChannel = &CurrentChannel{ID: channelId, Name: channelName, Attempt: attemptNo}
		r.CurrentPhase = PhaseWaitingUpstream
	})
}

// StartChannelAttemptWithContext is the gin-aware wrapper
func StartChannelAttemptWithContext(c *gin.Context, channelId int, channelName string, attemptNo int) {
	recordID := c.GetString("monitor_id")
	if recordID == "" {
		return
	}
	StartChannelAttempt(recordID, channelId, channelName, attemptNo)
}

// MarkChannelPhase updates the real-time phase (waiting_upstream/streaming/error/completed).
// Uses UpdateAndBroadcastChannel for a single lock acquisition.
func MarkChannelPhase(recordID string, phase string) {
	if !GetManager().IsEnabled() || GetManager().GetStore() == nil || recordID == "" || phase == "" {
		return
	}

	GetManager().GetStore().UpdateAndBroadcastChannelIfChanged(recordID, func(r *RequestRecord) bool {
		changed := false
		if r.CurrentPhase != phase {
			r.CurrentPhase = phase
			changed = true
		}
		if len(r.ChannelAttempts) == 0 {
			return changed
		}
		last := &r.ChannelAttempts[len(r.ChannelAttempts)-1]
		switch phase {
		case PhaseWaitingUpstream:
			if last.Status != AttemptStatusWaiting {
				last.Status = AttemptStatusWaiting
				changed = true
			}
		case PhaseStreaming:
			if last.Status != AttemptStatusStreaming {
				last.Status = AttemptStatusStreaming
				changed = true
			}
		case PhaseCompleted:
			if last.EndedAt == nil {
				now := time.Now()
				last.EndedAt = &now
				changed = true
			}
			if last.Status != AttemptStatusSucceeded {
				last.Status = AttemptStatusSucceeded
				changed = true
			}
		case PhaseError:
			if last.EndedAt == nil {
				now := time.Now()
				last.EndedAt = &now
				changed = true
			}
			if last.Status != AttemptStatusSucceeded && last.Status != AttemptStatusFailed {
				last.Status = AttemptStatusFailed
				changed = true
			}
		}
		return changed
	})
}

// MarkChannelPhaseWithContext wraps MarkChannelPhase using gin context
func MarkChannelPhaseWithContext(c *gin.Context, phase string) {
	recordID := c.GetString("monitor_id")
	if recordID == "" {
		return
	}
	MarkChannelPhase(recordID, phase)
}

// FinishChannelAttempt finalizes the latest attempt with a terminal status and reason.
// Uses UpdateAndBroadcastChannel for a single lock acquisition.
func FinishChannelAttempt(recordID string, status string, reason string, errorCode string, httpStatus int) {
	if !GetManager().IsEnabled() || GetManager().GetStore() == nil || recordID == "" {
		return
	}

	GetManager().GetStore().UpdateAndBroadcastChannel(recordID, func(r *RequestRecord) {
		if len(r.ChannelAttempts) == 0 {
			return
		}
		last := &r.ChannelAttempts[len(r.ChannelAttempts)-1]
		if last.EndedAt != nil {
			return
		}
		now := time.Now()
		last.EndedAt = &now
		last.Status = status
		last.Reason = reason
		last.ErrorCode = errorCode
		last.HTTPStatus = httpStatus
	})
}

// FinishChannelAttemptWithContext wraps FinishChannelAttempt using gin context
func FinishChannelAttemptWithContext(c *gin.Context, status string, reason string, errorCode string, httpStatus int) {
	recordID := c.GetString("monitor_id")
	if recordID == "" {
		return
	}
	FinishChannelAttempt(recordID, status, reason, errorCode, httpStatus)
}

// UpdateMetadata updates metadata for a request (channel info, stream status, etc.)
func UpdateMetadata(recordID string, channelId int, channelName string, isStream bool) {
	if !GetManager().IsEnabled() || GetManager().GetStore() == nil || recordID == "" {
		return
	}

	GetManager().GetStore().UpdateIfChanged(recordID, func(r *RequestRecord) bool {
		if r.ChannelId == channelId && r.ChannelName == channelName && r.IsStream == isStream {
			return false
		}
		r.ChannelId = channelId
		r.ChannelName = channelName
		r.IsStream = isStream
		return true
	})
}

// UpdateMetadataWithContext updates metadata using gin context
func UpdateMetadataWithContext(c *gin.Context) {
	recordID := c.GetString("monitor_id")
	if recordID == "" {
		return
	}

	channelId := c.GetInt("channel_id")
	channelName := c.GetString("channel_name")
	isStream := c.GetBool("is_stream")

	UpdateMetadata(recordID, channelId, channelName, isStream)
}
