package gemini

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertGeminiRequestFiltersUnsupportedGenerateContentSafetySettings(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-flash:generateContent", nil)

	request := &dto.GeminiChatRequest{
		Contents: []dto.GeminiChatContent{
			{
				Parts: []dto.GeminiPart{{Text: "hello"}},
			},
		},
		SafetySettings: []dto.GeminiChatSafetySettings{
			{Category: "HARM_CATEGORY_UNSPECIFIED", Threshold: "BLOCK_NONE"},
			{Category: " HARM_CATEGORY_HARASSMENT ", Threshold: "OFF"},
			{Category: "HARM_CATEGORY_CIVIC_INTEGRITY", Threshold: "OFF"},
			{Category: "HARM_CATEGORY_MEDICAL", Threshold: "BLOCK_ONLY_HIGH"},
		},
		Requests: []dto.GeminiChatRequest{
			{
				SafetySettings: []dto.GeminiChatSafetySettings{
					{Category: "HARM_CATEGORY_TOXICITY", Threshold: "BLOCK_NONE"},
					{Category: "HARM_CATEGORY_DANGEROUS_CONTENT", Threshold: "OFF"},
				},
			},
		},
	}

	convertedRequest, err := (&Adaptor{}).ConvertGeminiRequest(c, &relaycommon.RelayInfo{}, request)
	require.NoError(t, err)
	geminiRequest, ok := convertedRequest.(*dto.GeminiChatRequest)
	require.True(t, ok)

	require.Len(t, geminiRequest.SafetySettings, 2)
	assert.Equal(t, "HARM_CATEGORY_HARASSMENT", geminiRequest.SafetySettings[0].Category)
	assert.Equal(t, "OFF", geminiRequest.SafetySettings[0].Threshold)
	assert.Equal(t, "HARM_CATEGORY_CIVIC_INTEGRITY", geminiRequest.SafetySettings[1].Category)
	assert.Equal(t, "OFF", geminiRequest.SafetySettings[1].Threshold)

	require.Len(t, geminiRequest.Requests, 1)
	require.Len(t, geminiRequest.Requests[0].SafetySettings, 1)
	assert.Equal(t, "HARM_CATEGORY_DANGEROUS_CONTENT", geminiRequest.Requests[0].SafetySettings[0].Category)
}
