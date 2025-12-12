package monitor

import (
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

var (
	globalStore *Store
	globalHub   *Hub
	enabled     = true // Enabled by default
)

// Init initializes the monitor system
func Init() {
	globalHub = NewHub()
	globalStore = NewStore(globalHub)
	go globalHub.Run()
}

// IsEnabled returns whether monitoring is enabled
func IsEnabled() bool {
	return enabled
}

// SetEnabled enables or disables monitoring
func SetEnabled(e bool) {
	enabled = e
}

// GetStore returns the global store (for testing)
func GetStore() *Store {
	return globalStore
}

// GetHub returns the global hub (for testing)
func GetHub() *Hub {
	return globalHub
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

// RecordStart records the start of a request
// Returns the record ID for subsequent updates
func RecordStart(c *gin.Context, requestBody []byte) string {
	if !enabled || globalStore == nil {
		return ""
	}

	requestId := c.GetString(common.RequestIdKey)

	// Get metadata from context
	userId := c.GetInt("id")
	tokenId := c.GetInt("token_id")
	tokenName := c.GetString("token_name")
	model := c.GetString("original_model")

	body, bodyExceedsThreshold := CheckBodySize(string(requestBody))
	record := &RequestRecord{
		ID:        requestId,
		Status:    StatusProcessing,
		StartTime: time.Now(),
		Downstream: DownstreamInfo{
			Method:        c.Request.Method,
			Path:          c.Request.URL.Path,
			Headers:       ginHeadersToMap(c),
			Body:          body,
			BodySize:      len(requestBody),
			BodyTruncated: bodyExceedsThreshold,
			ClientIP:      c.ClientIP(),
		},
		UserId:    userId,
		TokenId:   tokenId,
		TokenName: tokenName,
		Model:     model,
	}

	globalStore.Add(record)
	return requestId
}

// RecordUpstream records the upstream request details
func RecordUpstream(recordID string, url string, method string, headers http.Header, body []byte) {
	if !enabled || globalStore == nil || recordID == "" {
		return
	}

	bodyStr, bodyExceedsThreshold := CheckBodySize(string(body))
	globalStore.Update(recordID, func(r *RequestRecord) {
		r.Upstream = &UpstreamInfo{
			URL:           url,
			Method:        method,
			Headers:       headersToMap(headers),
			Body:          bodyStr,
			BodySize:      len(body),
			BodyTruncated: bodyExceedsThreshold,
		}
	})
}

// RecordUpstreamWithContext records upstream request using gin context
func RecordUpstreamWithContext(c *gin.Context, url string, method string, headers http.Header, body []byte) {
	recordID := c.GetString("monitor_id")
	RecordUpstream(recordID, url, method, headers, body)
}

// RecordResponse records the response details
func RecordResponse(recordID string, statusCode int, headers http.Header, body []byte, promptTokens, completionTokens int, err error) {
	if !enabled || globalStore == nil || recordID == "" {
		return
	}

	bodyStr, bodyExceedsThreshold := CheckBodySize(string(body))
	response := &ResponseInfo{
		StatusCode:       statusCode,
		Headers:          headersToMap(headers),
		Body:             bodyStr,
		BodySize:         len(body),
		BodyTruncated:    bodyExceedsThreshold,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
	}

	if err != nil {
		response.Error = &ErrorInfo{
			Message: err.Error(),
		}
	}

	globalStore.MarkComplete(recordID, response)
	if err != nil {
		MarkChannelPhase(recordID, PhaseError)
		FinishChannelAttempt(recordID, AttemptStatusFailed, err.Error(), "", statusCode)
	} else {
		MarkChannelPhase(recordID, PhaseCompleted)
		FinishChannelAttempt(recordID, AttemptStatusSucceeded, "", "", statusCode)
	}
}

// RecordResponseWithContext records response using gin context
func RecordResponseWithContext(c *gin.Context, statusCode int, headers http.Header, body []byte, promptTokens, completionTokens int, err error) {
	recordID := c.GetString("monitor_id")
	RecordResponse(recordID, statusCode, headers, body, promptTokens, completionTokens, err)
}

// RecordError records an error for a request
func RecordError(recordID string, err error) {
	if !enabled || globalStore == nil || recordID == "" {
		return
	}

	globalStore.Update(recordID, func(r *RequestRecord) {
		now := time.Now()
		r.EndTime = &now
		r.Duration = now.Sub(r.StartTime).Milliseconds()
		r.Status = StatusError
		if r.Response == nil {
			r.Response = &ResponseInfo{}
		}
		r.Response.Error = &ErrorInfo{
			Message: err.Error(),
		}
	})

	FinishChannelAttempt(recordID, AttemptStatusFailed, err.Error(), "", 0)
	MarkChannelPhase(recordID, PhaseError)
}

// RecordErrorWithContext records an error using gin context
func RecordErrorWithContext(c *gin.Context, err error) {
	recordID := c.GetString("monitor_id")
	RecordError(recordID, err)
}

// StartChannelAttempt records that we are about to try a specific channel
func StartChannelAttempt(recordID string, channelId int, channelName string, attemptNo int) {
	if !enabled || globalStore == nil || recordID == "" {
		return
	}

	globalStore.Update(recordID, func(r *RequestRecord) {
		now := time.Now()
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
		r.Status = StatusWaitingUpstream
	})

	globalStore.BroadcastChannelUpdate(recordID)
}

// StartChannelAttemptWithContext is the gin-aware wrapper
func StartChannelAttemptWithContext(c *gin.Context, channelId int, channelName string, attemptNo int) {
	recordID := c.GetString("monitor_id")
	StartChannelAttempt(recordID, channelId, channelName, attemptNo)
}

// MarkChannelPhase updates the real-time phase (waiting_upstream/streaming/error/completed)
func MarkChannelPhase(recordID string, phase string) {
	if !enabled || globalStore == nil || recordID == "" || phase == "" {
		return
	}

	globalStore.Update(recordID, func(r *RequestRecord) {
		r.CurrentPhase = phase
		switch phase {
		case PhaseWaitingUpstream:
			r.Status = StatusWaitingUpstream
		case PhaseStreaming:
			r.Status = StatusStreaming
		}
		if len(r.ChannelAttempts) == 0 {
			return
		}
		last := &r.ChannelAttempts[len(r.ChannelAttempts)-1]
		switch phase {
		case PhaseWaitingUpstream:
			last.Status = AttemptStatusWaiting
		case PhaseStreaming:
			last.Status = AttemptStatusStreaming
			if last.StreamingStartedAt == nil {
				now := time.Now()
				last.StreamingStartedAt = &now
			}
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
	})

	globalStore.BroadcastChannelUpdate(recordID)
}

// MarkChannelPhaseWithContext wraps MarkChannelPhase using gin context
func MarkChannelPhaseWithContext(c *gin.Context, phase string) {
	recordID := c.GetString("monitor_id")
	MarkChannelPhase(recordID, phase)
}

// FinishChannelAttempt finalizes the latest attempt with a terminal status and reason
func FinishChannelAttempt(recordID string, status string, reason string, errorCode string, httpStatus int) {
	if !enabled || globalStore == nil || recordID == "" {
		return
	}

	globalStore.Update(recordID, func(r *RequestRecord) {
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

	globalStore.BroadcastChannelUpdate(recordID)
}

// FinishChannelAttemptWithContext wraps FinishChannelAttempt using gin context
func FinishChannelAttemptWithContext(c *gin.Context, status string, reason string, errorCode string, httpStatus int) {
	recordID := c.GetString("monitor_id")
	FinishChannelAttempt(recordID, status, reason, errorCode, httpStatus)
}

// UpdateMetadata updates metadata for a request (channel info, stream status, etc.)
func UpdateMetadata(recordID string, channelId int, channelName string, isStream bool) {
	if !enabled || globalStore == nil || recordID == "" {
		return
	}

	globalStore.Update(recordID, func(r *RequestRecord) {
		r.ChannelId = channelId
		r.ChannelName = channelName
		r.IsStream = isStream
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
