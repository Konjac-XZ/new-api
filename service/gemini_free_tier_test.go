package service

import (
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func TestDetectGeminiDailyQuotaExhaustion(t *testing.T) {
	dailyMetadata := []byte(`{
		"@type":"type.googleapis.com/google.rpc.ErrorInfo",
		"reason":"RATE_LIMIT_EXCEEDED",
		"details":[{
			"@type":"type.googleapis.com/google.rpc.QuotaFailure",
			"violations":[{
				"quotaId":"GenerateRequestsPerDayPerProjectPerModel-FreeTier",
				"quotaMetric":"generativelanguage.googleapis.com/generate_content_free_tier_requests",
				"description":"Requests per day quota exceeded"
			}]
		}]
	}`)
	rpmMetadata := []byte(`{
		"details":[{
			"@type":"type.googleapis.com/google.rpc.QuotaFailure",
			"violations":[{
				"quotaId":"GenerateRequestsPerMinutePerProjectPerModel",
				"quotaMetric":"generativelanguage.googleapis.com/generate_content_requests"
			}]
		}]
	}`)

	tests := []struct {
		name string
		err  *types.NewAPIError
		want bool
	}{
		{
			name: "daily free tier resource exhausted",
			err: types.WithOpenAIError(types.OpenAIError{
				Message:  "quota exceeded",
				Type:     "RESOURCE_EXHAUSTED",
				Code:     "RESOURCE_EXHAUSTED",
				Metadata: dailyMetadata,
			}, http.StatusTooManyRequests),
			want: true,
		},
		{
			name: "rpm only resource exhausted",
			err: types.WithOpenAIError(types.OpenAIError{
				Message:  "quota exceeded",
				Type:     "RESOURCE_EXHAUSTED",
				Code:     "RESOURCE_EXHAUSTED",
				Metadata: rpmMetadata,
			}, http.StatusTooManyRequests),
			want: false,
		},
		{
			name: "non 429",
			err: types.WithOpenAIError(types.OpenAIError{
				Message:  "quota exceeded",
				Type:     "RESOURCE_EXHAUSTED",
				Code:     "RESOURCE_EXHAUSTED",
				Metadata: dailyMetadata,
			}, http.StatusBadRequest),
			want: false,
		},
		{
			name: "ambiguous resource exhausted",
			err: types.WithOpenAIError(types.OpenAIError{
				Message: "quota exceeded",
				Type:    "RESOURCE_EXHAUSTED",
				Code:    "RESOURCE_EXHAUSTED",
			}, http.StatusTooManyRequests),
			want: false,
		},
		{
			name: "non gemini style error",
			err: types.WithOpenAIError(types.OpenAIError{
				Message:  "too many requests",
				Type:     "rate_limit_error",
				Code:     "rate_limit_exceeded",
				Metadata: dailyMetadata,
			}, http.StatusTooManyRequests),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, got := DetectGeminiDailyQuotaExhaustion(tt.err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestNextPacificMidnightDST(t *testing.T) {
	loc, err := time.LoadLocation("America/Los_Angeles")
	require.NoError(t, err)

	winter := time.Date(2026, time.January, 15, 12, 0, 0, 0, time.UTC)
	require.Equal(t,
		time.Date(2026, time.January, 16, 0, 0, 0, 0, loc),
		NextPacificMidnight(winter),
	)

	summer := time.Date(2026, time.July, 15, 12, 0, 0, 0, time.UTC)
	require.Equal(t,
		time.Date(2026, time.July, 16, 0, 0, 0, 0, loc),
		NextPacificMidnight(summer),
	)
}

func TestChannelIsEffectiveGeminiFreeTier(t *testing.T) {
	emptyBaseURL := ""
	customBaseURL := "https://example.com"

	tests := []struct {
		name    string
		channel *model.Channel
		want    bool
	}{
		{
			name: "gemini enabled empty base url",
			channel: &model.Channel{
				Type:          constant.ChannelTypeGemini,
				BaseURL:       &emptyBaseURL,
				OtherSettings: `{"gemini_free_tier":true}`,
			},
			want: true,
		},
		{
			name: "gemini enabled nil base url",
			channel: &model.Channel{
				Type:          constant.ChannelTypeGemini,
				OtherSettings: `{"gemini_free_tier":true}`,
			},
			want: true,
		},
		{
			name: "gemini custom base url ineffective",
			channel: &model.Channel{
				Type:          constant.ChannelTypeGemini,
				BaseURL:       &customBaseURL,
				OtherSettings: `{"gemini_free_tier":true}`,
			},
			want: false,
		},
		{
			name: "non gemini ineffective",
			channel: &model.Channel{
				Type:          constant.ChannelTypeOpenAI,
				BaseURL:       &emptyBaseURL,
				OtherSettings: `{"gemini_free_tier":true}`,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.channel.IsEffectiveGeminiFreeTier())
		})
	}
}

func TestGeminiFreeTierModelResolution(t *testing.T) {
	oldSettings := *model_setting.GetGeminiSettings()
	model_setting.GetGeminiSettings().ThinkingAdapterEnabled = true
	defer func() {
		*model_setting.GetGeminiSettings() = oldSettings
	}()

	ch := &model.Channel{
		Type:          constant.ChannelTypeGemini,
		ModelMapping:  common.GetPointer(`{"alias":"gemini-2.5-flash-thinking-1024"}`),
		OtherSettings: mustMarshalSettings(t, dto.ChannelOtherSettings{GeminiFreeTier: true}),
	}
	require.Equal(t, "gemini-2.5-flash", ResolveGeminiFreeTierSuppressionModel(ch, "alias"))
}

func TestGeminiFreeTierSuppressionIsModelScoped(t *testing.T) {
	oldRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	geminiFreeTierSuppressionCache = nil
	geminiFreeTierSuppressionCacheOnce = sync.Once{}
	defer func() {
		common.RedisEnabled = oldRedisEnabled
		geminiFreeTierSuppressionCache = nil
		geminiFreeTierSuppressionCacheOnce = sync.Once{}
	}()

	channelID := 12345
	key := geminiFreeTierSuppressionKey(channelID, "gemini-2.5-flash")
	require.NoError(t, getGeminiFreeTierSuppressionCache().SetWithTTL(key, "1", time.Minute))

	require.True(t, IsGeminiFreeTierSuppressed(channelID, "gemini-2.5-flash"))
	require.False(t, IsGeminiFreeTierSuppressed(channelID, "gemini-2.5-pro"))
	require.False(t, IsGeminiFreeTierSuppressed(channelID+1, "gemini-2.5-flash"))
}

func mustMarshalSettings(t *testing.T, settings dto.ChannelOtherSettings) string {
	t.Helper()
	data, err := common.Marshal(settings)
	require.NoError(t, err)
	return string(data)
}
