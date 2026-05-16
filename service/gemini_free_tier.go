package service

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/cachex"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/reasoning"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/samber/hot"
)

const geminiFreeTierSuppressionNamespace = "new-api:gemini_free_tier_suppression:v1"

var (
	geminiFreeTierSuppressionCacheOnce sync.Once
	geminiFreeTierSuppressionCache     *cachex.HybridCache[string]
)

type GeminiQuotaFailureEvidence struct {
	QuotaID string
	Metric  string
	Subject string
	Message string
}

func getGeminiFreeTierSuppressionCache() *cachex.HybridCache[string] {
	geminiFreeTierSuppressionCacheOnce.Do(func() {
		geminiFreeTierSuppressionCache = cachex.NewHybridCache[string](cachex.HybridCacheConfig[string]{
			Namespace:    cachex.Namespace(geminiFreeTierSuppressionNamespace),
			Redis:        common.RDB,
			RedisCodec:   cachex.StringCodec{},
			RedisEnabled: func() bool { return common.RedisEnabled },
			Memory: func() *hot.HotCache[string, string] {
				return hot.NewHotCache[string, string](hot.LRU, 4096).Build()
			},
		})
	})
	return geminiFreeTierSuppressionCache
}

func ResolveGeminiFreeTierSuppressionModel(channel *model.Channel, modelName string) string {
	if channel == nil {
		return ""
	}
	upstreamModel := strings.TrimSpace(modelName)
	if upstreamModel == "" {
		return ""
	}
	if mapping := strings.TrimSpace(channel.GetModelMapping()); mapping != "" && mapping != "{}" {
		var modelMap map[string]string
		if err := common.Unmarshal([]byte(mapping), &modelMap); err == nil {
			visited := map[string]bool{upstreamModel: true}
			for {
				mapped := strings.TrimSpace(modelMap[upstreamModel])
				if mapped == "" || visited[mapped] {
					break
				}
				visited[mapped] = true
				upstreamModel = mapped
			}
		}
	}
	return NormalizeGeminiUpstreamModelForSuppression(upstreamModel, modelName)
}

func NormalizeGeminiUpstreamModelForSuppression(upstreamModel string, originModel string) string {
	upstreamModel = strings.TrimSpace(upstreamModel)
	if upstreamModel == "" {
		return ""
	}
	if !model_setting.GetGeminiSettings().ThinkingAdapterEnabled ||
		model_setting.ShouldPreserveThinkingSuffix(originModel) ||
		model_setting.ShouldPreserveThinkingSuffix(upstreamModel) {
		return upstreamModel
	}
	if strings.Contains(upstreamModel, "-thinking-") {
		return strings.Split(upstreamModel, "-thinking-")[0]
	}
	if strings.HasSuffix(upstreamModel, "-thinking") {
		return strings.TrimSuffix(upstreamModel, "-thinking")
	}
	if strings.HasSuffix(upstreamModel, "-nothinking") {
		return strings.TrimSuffix(upstreamModel, "-nothinking")
	}
	if baseModel, level, ok := reasoning.TrimEffortSuffix(upstreamModel); ok && level != "" {
		return baseModel
	}
	return upstreamModel
}

func geminiFreeTierSuppressionKey(channelID int, upstreamModel string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(upstreamModel)))
	return fmt.Sprintf("%d:%s", channelID, hex.EncodeToString(sum[:])[:32])
}

func IsGeminiFreeTierSuppressed(channelID int, upstreamModel string) bool {
	if channelID <= 0 || strings.TrimSpace(upstreamModel) == "" {
		return false
	}
	_, found, err := getGeminiFreeTierSuppressionCache().Get(geminiFreeTierSuppressionKey(channelID, upstreamModel))
	if err != nil {
		common.SysLog(fmt.Sprintf("failed to read Gemini Free Tier suppression: channel_id=%d model=%s error=%v", channelID, upstreamModel, err))
		return false
	}
	return found
}

func GetGeminiFreeTierSuppressedChannelIDs(group string, modelName string) (map[int]bool, error) {
	channels, err := model.GetEnabledChannelsByGroupModel(group, modelName)
	if err != nil {
		return nil, err
	}
	if len(channels) == 0 {
		return nil, nil
	}
	exclude := make(map[int]bool)
	for _, channel := range channels {
		if channel == nil || !channel.IsEffectiveGeminiFreeTier() {
			continue
		}
		upstreamModel := ResolveGeminiFreeTierSuppressionModel(channel, modelName)
		if IsGeminiFreeTierSuppressed(channel.Id, upstreamModel) {
			exclude[channel.Id] = true
		}
	}
	if len(exclude) == 0 {
		return nil, nil
	}
	return exclude, nil
}

func NextPacificMidnight(now time.Time) time.Time {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		loc = time.FixedZone("America/Los_Angeles", -8*3600)
	}
	pt := now.In(loc)
	y, m, d := pt.Date()
	return time.Date(y, m, d+1, 0, 0, 0, 0, loc)
}

func DetectGeminiDailyQuotaExhaustion(err *types.NewAPIError) (GeminiQuotaFailureEvidence, bool) {
	if err == nil || err.StatusCode != http.StatusTooManyRequests {
		return GeminiQuotaFailureEvidence{}, false
	}
	oai, ok := err.RelayError.(types.OpenAIError)
	if !ok {
		return GeminiQuotaFailureEvidence{}, false
	}
	if !isResourceExhausted(oai) {
		return GeminiQuotaFailureEvidence{}, false
	}
	evidence, foundStructured := extractGeminiDailyQuotaEvidence(oai.Metadata)
	if foundStructured {
		return evidence, true
	}
	if raw, marshalErr := common.Marshal(err.RelayError); marshalErr == nil {
		evidence, foundStructured = extractGeminiDailyQuotaEvidence(raw)
		if foundStructured {
			return evidence, true
		}
	}
	return GeminiQuotaFailureEvidence{}, false
}

func isResourceExhausted(oai types.OpenAIError) bool {
	if strings.EqualFold(strings.TrimSpace(oai.Type), "RESOURCE_EXHAUSTED") {
		return true
	}
	if s, ok := oai.Code.(string); ok && strings.EqualFold(strings.TrimSpace(s), "RESOURCE_EXHAUSTED") {
		return true
	}
	return false
}

func extractGeminiDailyQuotaEvidence(metadata []byte) (GeminiQuotaFailureEvidence, bool) {
	if len(metadata) == 0 {
		return GeminiQuotaFailureEvidence{}, false
	}
	var parsed map[string]any
	if err := common.Unmarshal(metadata, &parsed); err != nil {
		return GeminiQuotaFailureEvidence{}, false
	}
	return findQuotaFailureEvidence(parsed)
}

func findQuotaFailureEvidence(value any) (GeminiQuotaFailureEvidence, bool) {
	switch v := value.(type) {
	case map[string]any:
		if isQuotaFailureMap(v) {
			if evidence, ok := evidenceFromQuotaFailureMap(v); ok {
				return evidence, true
			}
		}
		for _, child := range v {
			if evidence, ok := findQuotaFailureEvidence(child); ok {
				return evidence, true
			}
		}
	case []any:
		for _, child := range v {
			if evidence, ok := findQuotaFailureEvidence(child); ok {
				return evidence, true
			}
		}
	}
	return GeminiQuotaFailureEvidence{}, false
}

func isQuotaFailureMap(v map[string]any) bool {
	typeValue := stringFromAny(v["@type"])
	return strings.Contains(typeValue, "google.rpc.QuotaFailure") || v["violations"] != nil
}

func evidenceFromQuotaFailureMap(v map[string]any) (GeminiQuotaFailureEvidence, bool) {
	violations, ok := v["violations"].([]any)
	if !ok {
		return GeminiQuotaFailureEvidence{}, false
	}
	for _, raw := range violations {
		violation, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		evidence := GeminiQuotaFailureEvidence{
			QuotaID: firstNonEmptyString(
				stringFromAny(violation["quotaId"]),
				stringFromAny(violation["quota_id"]),
				stringFromAny(violation["quota"]),
				stringFromAny(violation["limit"]),
			),
			Metric: firstNonEmptyString(
				stringFromAny(violation["quotaMetric"]),
				stringFromAny(violation["quota_metric"]),
				stringFromAny(violation["metric"]),
			),
			Subject: stringFromAny(violation["subject"]),
			Message: stringFromAny(violation["description"]),
		}
		combined := strings.Join([]string{evidence.QuotaID, evidence.Metric, evidence.Subject, evidence.Message}, " ")
		if hasDailyFreeTierEvidence(combined) {
			return evidence, true
		}
	}
	return GeminiQuotaFailureEvidence{}, false
}

func hasDailyFreeTierEvidence(s string) bool {
	lower := strings.ToLower(s)
	needles := []string{"per day", "daily", "rpd", "requests per day", "free tier"}
	for _, needle := range needles {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}

func stringFromAny(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case fmt.Stringer:
		return x.String()
	default:
		if x == nil {
			return ""
		}
		return fmt.Sprintf("%v", x)
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func MaybeRecordGeminiFreeTierSuppression(c *gin.Context, channel *model.Channel, info *relaycommon.RelayInfo, err *types.NewAPIError) bool {
	if channel == nil || info == nil || !channel.IsEffectiveGeminiFreeTier() {
		return false
	}
	evidence, ok := DetectGeminiDailyQuotaExhaustion(err)
	if !ok {
		return false
	}
	upstreamModel := NormalizeGeminiUpstreamModelForSuppression(info.UpstreamModelName, info.OriginModelName)
	if upstreamModel == "" {
		return false
	}
	resetAt := NextPacificMidnight(time.Now())
	ttl := time.Until(resetAt)
	if ttl <= 0 {
		return false
	}
	key := geminiFreeTierSuppressionKey(channel.Id, upstreamModel)
	if cacheErr := getGeminiFreeTierSuppressionCache().SetWithTTL(key, "1", ttl); cacheErr != nil {
		common.SysLog(fmt.Sprintf("failed to record Gemini Free Tier suppression: channel_id=%d model=%s error=%v", channel.Id, upstreamModel, cacheErr))
		return false
	}
	message := fmt.Sprintf(
		"Gemini Free Tier suppression recorded: channel_id=%d channel_name=%q upstream_model=%q reset_at_pt=%s reset_at_unix=%d quota_id=%q metric=%q subject=%q message=%q",
		channel.Id,
		channel.Name,
		upstreamModel,
		resetAt.Format(time.RFC3339),
		resetAt.Unix(),
		evidence.QuotaID,
		evidence.Metric,
		evidence.Subject,
		evidence.Message,
	)
	common.SysLog(message)
	model.RecordLog(0, model.LogTypeSystem, message)
	return true
}

func IsGeminiFreeTierSuppressionError(channel *model.Channel, err *types.NewAPIError) bool {
	if channel == nil || !channel.IsEffectiveGeminiFreeTier() {
		return false
	}
	_, ok := DetectGeminiDailyQuotaExhaustion(err)
	return ok
}
