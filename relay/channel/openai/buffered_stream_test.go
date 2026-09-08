package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newBufferedStreamTestContext(body string, relayMode int) (*gin.Context, *httptest.ResponseRecorder, *http.Response, *relaycommon.RelayInfo) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":  []string{"text/event-stream"},
			"Cache-Control": []string{"no-cache"},
		},
		Body: io.NopCloser(strings.NewReader(body)),
	}
	info := &relaycommon.RelayInfo{
		RelayMode:           relayMode,
		RelayFormat:         types.RelayFormatOpenAI,
		OriginModelName:     "gpt-test",
		ChannelMeta:         &relaycommon.ChannelMeta{UpstreamModelName: "gpt-test"},
		MonitorResponseBody: relaycommon.NewMonitorResponseBody(),
	}
	return c, recorder, resp, info
}

func TestOaiBufferedStreamHandlerReturnsChatCompletionJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := strings.Join([]string{
		`data: {"id":"chatcmpl_1","object":"chat.completion.chunk","created":1710000000,"model":"gpt-test","system_fingerprint":"fp_1","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl_1","object":"chat.completion.chunk","created":1710000000,"model":"gpt-test","choices":[{"index":0,"delta":{"content":"hello "},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl_1","object":"chat.completion.chunk","created":1710000000,"model":"gpt-test","choices":[{"index":0,"delta":{"content":"world","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{\"q\":"}}]},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl_1","object":"chat.completion.chunk","created":1710000000,"model":"gpt-test","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"x\"}"}}]},"finish_reason":"tool_calls"}]}`,
		`data: {"id":"chatcmpl_1","object":"chat.completion.chunk","created":1710000000,"model":"gpt-test","choices":[],"usage":{"prompt_tokens":2,"completion_tokens":4,"total_tokens":6}}`,
		`data: [DONE]`,
		``,
	}, "\n")
	c, recorder, resp, info := newBufferedStreamTestContext(body, relayconstant.RelayModeChatCompletions)

	usage, newAPIError := OaiBufferedStreamHandler(c, info, resp)
	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	assert.Equal(t, 6, usage.TotalTokens)
	assert.Equal(t, "application/json", recorder.Header().Get("Content-Type"))
	assert.Empty(t, recorder.Header().Get("Cache-Control"))
	assert.NotContains(t, recorder.Body.String(), "data:")

	var response dto.OpenAITextResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "chat.completion", response.Object)
	assert.Equal(t, "hello world", response.Choices[0].Message.StringContent())
	assert.Equal(t, "tool_calls", response.Choices[0].FinishReason)
	toolCalls := response.Choices[0].Message.ParseToolCalls()
	require.Len(t, toolCalls, 1)
	assert.Equal(t, "lookup", toolCalls[0].Function.Name)
	assert.Equal(t, `{"q":"x"}`, toolCalls[0].Function.Arguments)
}

func TestOaiBufferedStreamHandlerReturnsLegacyCompletionJSON(t *testing.T) {
	body := strings.Join([]string{
		`data: {"id":"cmpl_1","object":"text_completion","created":1710000000,"model":"gpt-test","choices":[{"index":0,"text":"hello ","logprobs":null,"finish_reason":null}]}`,
		`data: {"id":"cmpl_1","object":"text_completion","created":1710000000,"model":"gpt-test","choices":[{"index":0,"text":"world","logprobs":null,"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`,
		`data: [DONE]`,
		``,
	}, "\n")
	c, recorder, resp, info := newBufferedStreamTestContext(body, relayconstant.RelayModeCompletions)

	usage, newAPIError := OaiBufferedStreamHandler(c, info, resp)
	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	assert.Equal(t, 3, usage.TotalTokens)

	var response bufferedCompletionResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "text_completion", response.Object)
	require.Len(t, response.Choices, 1)
	assert.Equal(t, "hello world", response.Choices[0].Text)
	assert.Equal(t, "stop", response.Choices[0].FinishReason)
}

func TestOaiBufferedStreamHandlerReturnsUpstreamStreamError(t *testing.T) {
	body := "data: {\"error\":{\"message\":\"upstream failed\",\"type\":\"server_error\",\"code\":\"failed\"}}\n\n"
	c, recorder, resp, info := newBufferedStreamTestContext(body, relayconstant.RelayModeChatCompletions)

	usage, newAPIError := OaiBufferedStreamHandler(c, info, resp)

	assert.Nil(t, usage)
	require.NotNil(t, newAPIError)
	assert.Equal(t, "upstream failed", newAPIError.ToOpenAIError().Message)
	assert.Empty(t, recorder.Body.String())
}
