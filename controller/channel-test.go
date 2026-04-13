package controller

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/channelcache"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	relaychannel "github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/samber/lo"
	"github.com/tidwall/gjson"

	"github.com/gin-gonic/gin"
)

type testResult struct {
	context     *gin.Context
	localErr    error
	newAPIError *types.NewAPIError
}

func scheduledTestFirstTokenLatencyMs(c *gin.Context) (int, bool) {
	if c == nil {
		return 0, false
	}
	value, exists := c.Get("first_token_latency_ms")
	if !exists {
		return 0, false
	}
	switch typed := value.(type) {
	case int:
		return typed, true
	case int8:
		return int(typed), true
	case int16:
		return int(typed), true
	case int32:
		return int(typed), true
	case int64:
		return int(typed), true
	case uint:
		return int(typed), true
	case uint8:
		return int(typed), true
	case uint16:
		return int(typed), true
	case uint32:
		return int(typed), true
	case uint64:
		return int(typed), true
	default:
		return 0, false
	}
}

var unsupportedTestChannelTypes = []int{
	constant.ChannelTypeMidjourney,
	constant.ChannelTypeMidjourneyPlus,
	constant.ChannelTypeSunoAPI,
	constant.ChannelTypeKling,
	constant.ChannelTypeJimeng,
	constant.ChannelTypeDoubaoVideo,
	constant.ChannelTypeVidu,
}

func resolveTestModel(channel *model.Channel, requestedModel string) string {
	testModel := strings.TrimSpace(requestedModel)
	if testModel != "" {
		return testModel
	}

	if channel.TestModel != nil && strings.TrimSpace(*channel.TestModel) != "" {
		return strings.TrimSpace(*channel.TestModel)
	}

	models := channel.GetModels()
	if len(models) > 0 {
		if candidate := strings.TrimSpace(models[0]); candidate != "" {
			return candidate
		}
	}
	return "gpt-4o-mini"
}

func normalizeChannelTestEndpoint(channel *model.Channel, modelName, endpointType string) string {
	normalized := strings.TrimSpace(endpointType)
	if normalized != "" {
		return normalized
	}
	if strings.HasSuffix(modelName, ratio_setting.CompactModelSuffix) {
		return string(constant.EndpointTypeOpenAIResponseCompact)
	}
	if channel != nil && channel.Type == constant.ChannelTypeCodex {
		return string(constant.EndpointTypeOpenAIResponse)
	}
	return normalized
}

func resolveTestRequestPath(channel *model.Channel, testModel string, endpointType string) string {
	endpointType = normalizeChannelTestEndpoint(channel, testModel, endpointType)
	requestPath := "/v1/chat/completions"

	if endpointType != "" {
		if endpointInfo, ok := common.GetDefaultEndpointInfo(constant.EndpointType(endpointType)); ok {
			return endpointInfo.Path
		}
		return requestPath
	}

	lowerModel := strings.ToLower(testModel)
	if strings.Contains(lowerModel, "embedding") ||
		strings.HasPrefix(testModel, "m3e") ||
		strings.Contains(testModel, "bge-") ||
		strings.Contains(lowerModel, "embed") ||
		channel.Type == constant.ChannelTypeMokaAI {
		return "/v1/embeddings"
	}

	if channel.Type == constant.ChannelTypeVolcEngine && strings.Contains(testModel, "seedream") {
		return "/v1/images/generations"
	}

	if strings.Contains(strings.ToLower(testModel), "codex") {
		return "/v1/responses"
	}

	if strings.HasSuffix(testModel, ratio_setting.CompactModelSuffix) {
		return "/v1/responses/compact"
	}

	return requestPath
}

func detectRelayFormat(endpointType string, requestPath string) types.RelayFormat {
	if endpointType != "" {
		switch constant.EndpointType(endpointType) {
		case constant.EndpointTypeOpenAI:
			return types.RelayFormatOpenAI
		case constant.EndpointTypeOpenAIResponse:
			return types.RelayFormatOpenAIResponses
		case constant.EndpointTypeOpenAIResponseCompact:
			return types.RelayFormatOpenAIResponsesCompaction
		case constant.EndpointTypeAnthropic:
			return types.RelayFormatClaude
		case constant.EndpointTypeGemini:
			return types.RelayFormatGemini
		case constant.EndpointTypeJinaRerank:
			return types.RelayFormatRerank
		case constant.EndpointTypeImageGeneration:
			return types.RelayFormatOpenAIImage
		case constant.EndpointTypeEmbeddings:
			return types.RelayFormatEmbedding
		default:
			return types.RelayFormatOpenAI
		}
	}

	switch {
	case requestPath == "/v1/embeddings":
		return types.RelayFormatEmbedding
	case requestPath == "/v1/images/generations":
		return types.RelayFormatOpenAIImage
	case requestPath == "/v1/messages":
		return types.RelayFormatClaude
	case strings.Contains(requestPath, "/v1beta/models"):
		return types.RelayFormatGemini
	case requestPath == "/v1/rerank" || requestPath == "/rerank":
		return types.RelayFormatRerank
	case requestPath == "/v1/responses":
		return types.RelayFormatOpenAIResponses
	case strings.HasPrefix(requestPath, "/v1/responses/compact"):
		return types.RelayFormatOpenAIResponsesCompaction
	default:
		return types.RelayFormatOpenAI
	}
}

func convertRequestForAdaptor(c *gin.Context, info *relaycommon.RelayInfo, adaptor relaychannel.Adaptor, request dto.Request) (any, error) {
	switch info.RelayMode {
	case relayconstant.RelayModeEmbeddings:
		if embeddingReq, ok := request.(*dto.EmbeddingRequest); ok {
			return adaptor.ConvertEmbeddingRequest(c, info, *embeddingReq)
		}
		return nil, errors.New("invalid embedding request type")
	case relayconstant.RelayModeImagesGenerations:
		if imageReq, ok := request.(*dto.ImageRequest); ok {
			return adaptor.ConvertImageRequest(c, info, *imageReq)
		}
		return nil, errors.New("invalid image request type")
	case relayconstant.RelayModeRerank:
		if rerankReq, ok := request.(*dto.RerankRequest); ok {
			return adaptor.ConvertRerankRequest(c, info.RelayMode, *rerankReq)
		}
		return nil, errors.New("invalid rerank request type")
	case relayconstant.RelayModeResponses:
		if responseReq, ok := request.(*dto.OpenAIResponsesRequest); ok {
			return adaptor.ConvertOpenAIResponsesRequest(c, info, *responseReq)
		}
		return nil, errors.New("invalid response request type")
	case relayconstant.RelayModeResponsesCompact:
		switch req := request.(type) {
		case *dto.OpenAIResponsesCompactionRequest:
			return adaptor.ConvertOpenAIResponsesRequest(c, info, dto.OpenAIResponsesRequest{
				Model:              req.Model,
				Input:              req.Input,
				Instructions:       req.Instructions,
				PreviousResponseID: req.PreviousResponseID,
			})
		case *dto.OpenAIResponsesRequest:
			return adaptor.ConvertOpenAIResponsesRequest(c, info, *req)
		default:
			return nil, errors.New("invalid response compaction request type")
		}
	default:
		if generalReq, ok := request.(*dto.GeneralOpenAIRequest); ok {
			return adaptor.ConvertOpenAIRequest(c, info, generalReq)
		}
		return nil, errors.New("invalid general request type")
	}
}

func extractExpectedAnswerContent(responseBody []byte) (string, error) {
	if len(responseBody) == 0 {
		return "", errors.New("empty response body")
	}

	var responsesResp dto.OpenAIResponsesResponse
	if err := json.Unmarshal(responseBody, &responsesResp); err == nil {
		if len(responsesResp.Output) > 0 || responsesResp.Object == "response" {
			text := service.ExtractOutputTextFromResponses(&responsesResp)
			if strings.TrimSpace(text) != "" {
				return text, nil
			}
		}
	}

	var textResp dto.OpenAITextResponse
	if err := json.Unmarshal(responseBody, &textResp); err == nil {
		if len(textResp.Choices) > 0 {
			content := textResp.Choices[0].Message.StringContent()
			if strings.TrimSpace(content) != "" {
				return content, nil
			}
		}
	}

	var generic map[string]any
	if err := json.Unmarshal(responseBody, &generic); err == nil {
		if choices, ok := generic["choices"].([]any); ok && len(choices) > 0 {
			if choice, ok := choices[0].(map[string]any); ok {
				if msg, ok := choice["message"].(map[string]any); ok {
					if content, ok := msg["content"].(string); ok && strings.TrimSpace(content) != "" {
						return content, nil
					}
					if contentList, ok := msg["content"].([]any); ok {
						var sb strings.Builder
						for _, item := range contentList {
							if itemMap, ok := item.(map[string]any); ok {
								if itemMap["type"] == "text" {
									if text, ok := itemMap["text"].(string); ok {
										sb.WriteString(text)
									}
								}
							}
						}
						if strings.TrimSpace(sb.String()) != "" {
							return sb.String(), nil
						}
					}
				}
				if text, ok := choice["text"].(string); ok && strings.TrimSpace(text) != "" {
					return text, nil
				}
			}
		}
	}

	return "", errors.New("unable to extract response content")
}

func validateExpectedAnswer(channel *model.Channel, responseBody []byte) error {
	if channel == nil || channel.ExpectedAnswer == nil {
		return nil
	}
	expectedAnswer := strings.TrimSpace(*channel.ExpectedAnswer)
	if expectedAnswer == "" {
		return nil
	}

	responseContent, err := extractExpectedAnswerContent(responseBody)
	if err != nil {
		return fmt.Errorf("failed to parse response for expected answer check: %v", err)
	}

	if !strings.Contains(strings.ToLower(responseContent), strings.ToLower(expectedAnswer)) {
		return fmt.Errorf("expected answer not found in response: expected '%s'", expectedAnswer)
	}

	return nil
}

func coerceTestUsage(usageAny any, isStream bool, estimatePromptTokens int) (*dto.Usage, error) {
	switch u := usageAny.(type) {
	case *dto.Usage:
		return u, nil
	case dto.Usage:
		return &u, nil
	case nil:
		if !isStream {
			return nil, errors.New("usage is nil")
		}
		usage := &dto.Usage{PromptTokens: estimatePromptTokens}
		usage.TotalTokens = usage.PromptTokens
		return usage, nil
	default:
		if !isStream {
			return nil, fmt.Errorf("invalid usage type: %T", usageAny)
		}
		usage := &dto.Usage{PromptTokens: estimatePromptTokens}
		usage.TotalTokens = usage.PromptTokens
		return usage, nil
	}
}

func readTestResponseBody(body io.ReadCloser, isStream bool) ([]byte, error) {
	defer func() { _ = body.Close() }()
	const maxStreamLogBytes = 8 << 10
	if isStream {
		return io.ReadAll(io.LimitReader(body, maxStreamLogBytes))
	}
	return io.ReadAll(body)
}

func detectErrorFromTestResponseBody(respBody []byte) error {
	b := bytes.TrimSpace(respBody)
	if len(b) == 0 {
		return nil
	}
	if message := detectErrorMessageFromJSONBytes(b); message != "" {
		return fmt.Errorf("upstream error: %s", message)
	}

	for _, line := range bytes.Split(b, []byte{'\n'}) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 || !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		payload := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
		if len(payload) == 0 || bytes.Equal(payload, []byte("[DONE]")) {
			continue
		}
		if message := detectErrorMessageFromJSONBytes(payload); message != "" {
			return fmt.Errorf("upstream error: %s", message)
		}
	}

	return nil
}

func detectErrorMessageFromJSONBytes(jsonBytes []byte) string {
	if len(jsonBytes) == 0 || (jsonBytes[0] != '{' && jsonBytes[0] != '[') {
		return ""
	}
	errVal := gjson.GetBytes(jsonBytes, "error")
	if !errVal.Exists() || errVal.Type == gjson.Null {
		return ""
	}

	message := gjson.GetBytes(jsonBytes, "error.message").String()
	if message == "" {
		message = gjson.GetBytes(jsonBytes, "error.error.message").String()
	}
	if message == "" && errVal.Type == gjson.String {
		message = errVal.String()
	}
	if message == "" {
		message = errVal.Raw
	}
	message = strings.TrimSpace(message)
	if message == "" {
		return "upstream returned error payload"
	}
	return message
}

func testChannel(channel *model.Channel, testModel string, endpointType string, isStream bool) testResult {
	tik := time.Now()
	if lo.Contains(unsupportedTestChannelTypes, channel.Type) {
		channelTypeName := constant.GetChannelTypeName(channel.Type)
		return testResult{
			localErr: fmt.Errorf("%s channel test is not supported", channelTypeName),
		}
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	testModel = resolveTestModel(channel, testModel)
	endpointType = normalizeChannelTestEndpoint(channel, testModel, endpointType)
	requestPath := resolveTestRequestPath(channel, testModel, endpointType)
	if strings.HasPrefix(requestPath, "/v1/responses/compact") {
		testModel = ratio_setting.WithCompactModelSuffix(testModel)
	}

	c.Request = &http.Request{
		Method: "POST",
		URL:    &url.URL{Path: requestPath}, // 使用动态路径
		Body:   nil,
		Header: make(http.Header),
	}

	cache, err := model.GetUserCache(1)
	if err != nil {
		return testResult{
			localErr:    err,
			newAPIError: nil,
		}
	}
	cache.WriteContext(c)
	c.Set("id", 1)

	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("channel", channel.Type)
	c.Set("base_url", channel.GetBaseURL())
	group, _ := model.GetUserGroup(1, false)
	c.Set("group", group)

	newAPIError := middleware.SetupContextForSelectedChannel(c, channel, testModel)
	if newAPIError != nil {
		return testResult{
			context:     c,
			localErr:    newAPIError,
			newAPIError: newAPIError,
		}
	}

	// Determine relay format based on endpoint type or request path
	relayFormat := detectRelayFormat(endpointType, c.Request.URL.Path)

	request := buildTestRequest(testModel, endpointType, channel, isStream)

	info, err := relaycommon.GenRelayInfo(c, relayFormat, request, nil)

	if err != nil {
		return testResult{
			context:     c,
			localErr:    err,
			newAPIError: types.NewError(err, types.ErrorCodeGenRelayInfoFailed),
		}
	}

	info.IsChannelTest = true
	info.InitChannelMeta(c)

	err = helper.ModelMappedHelper(c, info, request)
	if err != nil {
		return testResult{
			context:     c,
			localErr:    err,
			newAPIError: types.NewError(err, types.ErrorCodeChannelModelMappedError),
		}
	}

	testModel = info.UpstreamModelName
	// 更新请求中的模型名称
	request.SetModelName(testModel)

	apiType, _ := common.ChannelType2APIType(channel.Type)
	if info.RelayMode == relayconstant.RelayModeResponsesCompact &&
		apiType != constant.APITypeOpenAI &&
		apiType != constant.APITypeCodex {
		return testResult{
			context:     c,
			localErr:    fmt.Errorf("responses compaction test only supports openai/codex channels, got api type %d", apiType),
			newAPIError: types.NewError(fmt.Errorf("unsupported api type: %d", apiType), types.ErrorCodeInvalidApiType),
		}
	}
	adaptor := relay.GetAdaptor(apiType)
	if adaptor == nil {
		return testResult{
			context:     c,
			localErr:    fmt.Errorf("invalid api type: %d, adaptor is nil", apiType),
			newAPIError: types.NewError(fmt.Errorf("invalid api type: %d, adaptor is nil", apiType), types.ErrorCodeInvalidApiType),
		}
	}
	common.SysLog(fmt.Sprintf("testing channel %d with model %s , info %+v ", channel.Id, testModel, info.ToString()))

	priceData, err := helper.ModelPriceHelper(c, info, 0, request.GetTokenCountMeta())
	if err != nil {
		return testResult{
			context:     c,
			localErr:    err,
			newAPIError: types.NewError(err, types.ErrorCodeModelPriceError, types.ErrOptionWithStatusCode(http.StatusBadRequest)),
		}
	}

	adaptor.Init(info)

	convertedRequest, err := convertRequestForAdaptor(c, info, adaptor, request)
	if err != nil {
		return testResult{
			context:     c,
			localErr:    err,
			newAPIError: types.NewError(err, types.ErrorCodeConvertRequestFailed),
		}
	}
	jsonData, err := common.Marshal(convertedRequest)
	if err != nil {
		return testResult{
			context:     c,
			localErr:    err,
			newAPIError: types.NewError(err, types.ErrorCodeJsonMarshalFailed),
		}
	}

	if len(info.ParamOverride) > 0 {
		jsonData, err = relaycommon.ApplyParamOverrideWithRelayInfo(jsonData, info)
		if err != nil {
			if fixedErr, ok := relaycommon.AsParamOverrideReturnError(err); ok {
				return testResult{
					context:     c,
					localErr:    fixedErr,
					newAPIError: relaycommon.NewAPIErrorFromParamOverride(fixedErr),
				}
			}
			return testResult{
				context:     c,
				localErr:    err,
				newAPIError: types.NewError(err, types.ErrorCodeChannelParamOverrideInvalid),
			}
		}
	}

	requestBody := bytes.NewBuffer(jsonData)
	c.Request.Body = io.NopCloser(bytes.NewBuffer(jsonData))
	resp, err := adaptor.DoRequest(c, info, requestBody)
	if err != nil {
		return testResult{
			context:     c,
			localErr:    err,
			newAPIError: types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError),
		}
	}
	var httpResp *http.Response
	if resp != nil {
		httpResp = resp.(*http.Response)
		if httpResp.StatusCode != http.StatusOK {
			err := service.RelayErrorHandler(c.Request.Context(), httpResp, true)
			common.SysError(fmt.Sprintf(
				"channel test bad response: channel_id=%d name=%s type=%d model=%s endpoint_type=%s status=%d err=%v",
				channel.Id,
				channel.Name,
				channel.Type,
				testModel,
				endpointType,
				httpResp.StatusCode,
				err,
			))
			return testResult{
				context:     c,
				localErr:    err,
				newAPIError: types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError),
			}
		}
	}
	usageA, respErr := adaptor.DoResponse(c, httpResp, info)
	if respErr != nil {
		return testResult{
			context:     c,
			localErr:    respErr,
			newAPIError: respErr,
		}
	}
	usage, usageErr := coerceTestUsage(usageA, isStream, info.GetEstimatePromptTokens())
	if usageErr != nil {
		return testResult{
			context:     c,
			localErr:    usageErr,
			newAPIError: types.NewOpenAIError(usageErr, types.ErrorCodeBadResponseBody, http.StatusInternalServerError),
		}
	}
	result := w.Result()
	respBody, err := readTestResponseBody(result.Body, isStream)
	if err != nil {
		return testResult{
			context:     c,
			localErr:    err,
			newAPIError: types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError),
		}
	}
	if bodyErr := detectErrorFromTestResponseBody(respBody); bodyErr != nil {
		return testResult{
			context:     c,
			localErr:    bodyErr,
			newAPIError: types.NewOpenAIError(bodyErr, types.ErrorCodeBadResponseBody, http.StatusInternalServerError),
		}
	}
	if !isStream {
		if err := validateExpectedAnswer(channel, respBody); err != nil {
			return testResult{
				context:     c,
				localErr:    err,
				newAPIError: types.NewError(err, types.ErrorCodeBadResponseBody),
			}
		}
	}
	info.SetEstimatePromptTokens(usage.PromptTokens)

	quota := 0
	if !priceData.UsePrice {
		quota = usage.PromptTokens + int(math.Round(float64(usage.CompletionTokens)*priceData.CompletionRatio))
		quota = int(math.Round(float64(quota) * priceData.ModelRatio))
		if priceData.ModelRatio != 0 && quota <= 0 {
			quota = 1
		}
	} else {
		quota = int(priceData.ModelPrice * common.QuotaPerUnit)
	}
	tok := time.Now()
	milliseconds := tok.Sub(tik).Milliseconds()
	consumedTime := float64(milliseconds) / 1000.0
	other := service.GenerateTextOtherInfo(c, info, priceData.ModelRatio, priceData.GroupRatioInfo.GroupRatio, priceData.CompletionRatio,
		usage.PromptTokensDetails.CachedTokens, priceData.CacheRatio, priceData.ModelPrice, priceData.GroupRatioInfo.GroupSpecialRatio)
	model.RecordConsumeLog(c, 1, model.RecordConsumeLogParams{
		ChannelId:        channel.Id,
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		ModelName:        info.OriginModelName,
		TokenName:        "模型测试",
		Quota:            quota,
		Content:          "模型测试",
		UseTimeSeconds:   int(consumedTime),
		IsStream:         info.IsStream,
		Group:            info.UsingGroup,
		Other:            other,
	})
	common.SysLog(fmt.Sprintf("testing channel #%d, response: \n%s", channel.Id, string(respBody)))
	return testResult{
		context:     c,
		localErr:    nil,
		newAPIError: nil,
	}
}

func buildTestRequest(model string, endpointType string, channel *model.Channel, isStream bool) dto.Request {
	// Get custom test case from channel, default to "hi" for chat and "hello world" for embeddings
	testCase := "hi"
	embeddingTestCase := "hello world"
	if channel.TestCase != nil && strings.TrimSpace(*channel.TestCase) != "" {
		testCase = strings.TrimSpace(*channel.TestCase)
		embeddingTestCase = testCase
	}

	// Build JSON input for responses endpoints using the custom test case
	testResponsesInput := json.RawMessage(fmt.Sprintf(`[{"role":"user","content":"%s"}]`, testCase))

	// 根据端点类型构建不同的测试请求
	if endpointType != "" {
		switch constant.EndpointType(endpointType) {
		case constant.EndpointTypeEmbeddings:
			// 返回 EmbeddingRequest
			return &dto.EmbeddingRequest{
				Model: model,
				Input: []any{embeddingTestCase},
			}
		case constant.EndpointTypeImageGeneration:
			// 返回 ImageRequest
			return &dto.ImageRequest{
				Model:  model,
				Prompt: "a cute cat",
				N:      lo.ToPtr(uint(1)),
				Size:   "1024x1024",
			}
		case constant.EndpointTypeJinaRerank:
			// 返回 RerankRequest
			return &dto.RerankRequest{
				Model:     model,
				Query:     "What is Deep Learning?",
				Documents: []any{"Deep Learning is a subset of machine learning.", "Machine learning is a field of artificial intelligence."},
				TopN:      lo.ToPtr(2),
			}
		case constant.EndpointTypeOpenAIResponse:
			// 返回 OpenAIResponsesRequest
			return &dto.OpenAIResponsesRequest{
				Model:  model,
				Input:  testResponsesInput,
				Stream: lo.ToPtr(isStream),
			}
		case constant.EndpointTypeOpenAIResponseCompact:
			// 返回 OpenAIResponsesCompactionRequest
			return &dto.OpenAIResponsesCompactionRequest{
				Model: model,
				Input: testResponsesInput,
			}
		case constant.EndpointTypeAnthropic, constant.EndpointTypeGemini, constant.EndpointTypeOpenAI:
			// 返回 GeneralOpenAIRequest
			maxTokens := uint(16)
			if constant.EndpointType(endpointType) == constant.EndpointTypeGemini {
				maxTokens = 3000
			}
			req := &dto.GeneralOpenAIRequest{
				Model:  model,
				Stream: lo.ToPtr(isStream),
				Messages: []dto.Message{
					{
						Role:    "user",
						Content: testCase,
					},
				},
				MaxTokens: lo.ToPtr(maxTokens),
			}
			if isStream {
				req.StreamOptions = &dto.StreamOptions{IncludeUsage: true}
			}
			return req
		}
	}

	// 自动检测逻辑（保持原有行为）
	if strings.Contains(strings.ToLower(model), "rerank") {
		return &dto.RerankRequest{
			Model:     model,
			Query:     "What is Deep Learning?",
			Documents: []any{"Deep Learning is a subset of machine learning.", "Machine learning is a field of artificial intelligence."},
			TopN:      lo.ToPtr(2),
		}
	}

	// 先判断是否为 Embedding 模型
	if strings.Contains(strings.ToLower(model), "embedding") ||
		strings.HasPrefix(model, "m3e") ||
		strings.Contains(model, "bge-") {
		// 返回 EmbeddingRequest
		return &dto.EmbeddingRequest{
			Model: model,
			Input: []any{embeddingTestCase},
		}
	}

	// Responses compaction models (must use /v1/responses/compact)
	if strings.HasSuffix(model, ratio_setting.CompactModelSuffix) {
		return &dto.OpenAIResponsesCompactionRequest{
			Model: model,
			Input: testResponsesInput,
		}
	}

	// Responses-only models (e.g. codex series)
	if strings.Contains(strings.ToLower(model), "codex") {
		return &dto.OpenAIResponsesRequest{
			Model:  model,
			Input:  testResponsesInput,
			Stream: lo.ToPtr(isStream),
		}
	}

	// Chat/Completion 请求 - 返回 GeneralOpenAIRequest
	testRequest := &dto.GeneralOpenAIRequest{
		Model:  model,
		Stream: lo.ToPtr(isStream),
		Messages: []dto.Message{
			{
				Role:    "user",
				Content: testCase,
			},
		},
	}
	if isStream {
		testRequest.StreamOptions = &dto.StreamOptions{IncludeUsage: true}
	}

	if strings.HasPrefix(model, "o") {
		testRequest.MaxCompletionTokens = lo.ToPtr(uint(16))
	} else if strings.Contains(model, "thinking") {
		if !strings.Contains(model, "claude") {
			testRequest.MaxTokens = lo.ToPtr(uint(50))
		}
	} else if strings.Contains(model, "gemini") {
		testRequest.MaxTokens = lo.ToPtr(uint(3000))
	} else {
		testRequest.MaxTokens = lo.ToPtr(uint(16))
	}

	return testRequest
}

func TestChannel(c *gin.Context) {
	channelId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	channel, err := model.CacheGetChannel(channelId)
	if err != nil {
		channel, err = model.GetChannelById(channelId, true)
		if err != nil {
			common.ApiError(c, err)
			return
		}
	}
	testModel := c.Query("model")
	endpointType := c.Query("endpoint_type")
	isStream, _ := strconv.ParseBool(c.Query("stream"))
	tik := time.Now()
	result := testChannel(channel, testModel, endpointType, isStream)
	if result.localErr != nil {
		resp := gin.H{
			"success": false,
			"message": result.localErr.Error(),
			"time":    0.0,
		}
		if result.newAPIError != nil {
			resp["error_code"] = result.newAPIError.GetErrorCode()
		}
		c.JSON(http.StatusOK, resp)
		return
	}
	tok := time.Now()
	milliseconds := tok.Sub(tik).Milliseconds()
	go channel.UpdateResponseTime(milliseconds)
	consumedTime := float64(milliseconds) / 1000.0
	if result.newAPIError != nil {
		c.JSON(http.StatusOK, gin.H{
			"success":    false,
			"message":    result.newAPIError.Error(),
			"time":       consumedTime,
			"error_code": result.newAPIError.GetErrorCode(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"time":    consumedTime,
	})
}

func TestChannelStream(c *gin.Context) {
	channelId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	channel, err := model.CacheGetChannel(channelId)
	if err != nil {
		channel, err = model.GetChannelById(channelId, true)
		if err != nil {
			common.ApiError(c, err)
			return
		}
	}
	testModel := c.Query("model")
	tik := time.Now()
	result := testChannelStream(channel, testModel)
	if result.localErr != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": result.localErr.Error(),
			"time":    0.0,
		})
		return
	}

	// Extract first token latency from context
	firstTokenLatencyMs := 0
	if result.context != nil {
		firstTokenLatencyMs = result.context.GetInt("first_token_latency_ms")
	}

	tok := time.Now()
	milliseconds := tok.Sub(tik).Milliseconds()

	// Use first token latency if available, otherwise use total time
	if firstTokenLatencyMs > 0 {
		milliseconds = int64(firstTokenLatencyMs)
	}

	go channel.UpdateResponseTime(milliseconds)
	consumedTime := float64(milliseconds) / 1000.0

	if result.newAPIError != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": result.newAPIError.Error(),
			"time":    consumedTime,
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"time":    consumedTime,
	})
}

var testAllChannelsLock sync.Mutex
var testAllChannelsRunning bool = false

func testAllChannels(notify bool) error {

	testAllChannelsLock.Lock()
	if testAllChannelsRunning {
		testAllChannelsLock.Unlock()
		return errors.New("测试已在运行中")
	}
	testAllChannelsRunning = true
	testAllChannelsLock.Unlock()
	channels, getChannelErr := model.GetAllChannels(0, 0, true, false)
	if getChannelErr != nil {
		return getChannelErr
	}
	var disableThreshold = int64(common.ChannelDisableThreshold * 1000)
	if disableThreshold == 0 {
		disableThreshold = 10000000 // a impossible value
	}
	gopool.Go(func() {
		// 使用 defer 确保无论如何都会重置运行状态，防止死锁
		defer func() {
			testAllChannelsLock.Lock()
			testAllChannelsRunning = false
			testAllChannelsLock.Unlock()
		}()

		for _, channel := range channels {
			if channel.Status == common.ChannelStatusManuallyDisabled {
				continue
			}
			isChannelEnabled := channel.Status == common.ChannelStatusEnabled
			dynamicBreakerEnabled := channel.IsDynamicCircuitBreakerEnabled()
			tik := time.Now()
			result := testChannel(channel, "", "", false)
			tok := time.Now()
			milliseconds := tok.Sub(tik).Milliseconds()

			shouldBanChannel := false
			newAPIError := result.newAPIError
			// request error disables the channel
			if newAPIError != nil {
				shouldBanChannel = service.ShouldDisableChannel(channel.Type, result.newAPIError)
			}

			// 当错误检查通过，才检查响应时间
			if common.AutomaticDisableChannelEnabled && !shouldBanChannel {
				if milliseconds > disableThreshold {
					err := fmt.Errorf("响应时间 %.2fs 超过阈值 %.2fs", float64(milliseconds)/1000.0, float64(disableThreshold)/1000.0)
					newAPIError = types.NewOpenAIError(err, types.ErrorCodeChannelResponseTimeExceeded, http.StatusRequestTimeout)
					shouldBanChannel = true
				}
			}

			// For dynamic breaker channels, channel-test failures should affect soft constraints only.
			// Keep hard status gating unchanged unless user explicitly manual-disables the channel.
			if shouldBanChannel && channel.GetAutoBan() && dynamicBreakerEnabled && newAPIError != nil {
				service.RecordChannelRelayFailure(channel, nil, newAPIError)
			}

			// disable channel (hard constraint path)
			if isChannelEnabled && shouldBanChannel && channel.GetAutoBan() && !dynamicBreakerEnabled {
				processChannelError(result.context, *types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey, common.GetContextKeyString(result.context, constant.ContextKeyChannelKey), channel.GetAutoBan()), newAPIError)
			}

			// enable channel
			if !dynamicBreakerEnabled && !isChannelEnabled && service.ShouldEnableChannel(newAPIError, channel.Status) {
				service.EnableChannel(channel.Id, common.GetContextKeyString(result.context, constant.ContextKeyChannelKey), channel.Name)
			}

			channel.UpdateResponseTime(milliseconds)
			time.Sleep(common.RequestInterval)
		}

		if notify {
			service.NotifyRootUser(dto.NotifyTypeChannelTest, "通道测试完成", "所有通道测试已完成")
		}
	})
	return nil
}

func TestAllChannels(c *gin.Context) {
	err := testAllChannels(true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

var autoTestChannelsOnce sync.Once

func AutomaticallyTestChannels() {
	// 只在Master节点定时测试渠道
	if !common.IsMasterNode {
		return
	}
	autoTestChannelsOnce.Do(func() {
		for {
			if !operation_setting.GetMonitorSetting().AutoTestChannelEnabled {
				time.Sleep(1 * time.Minute)
				continue
			}
			for {
				frequency := operation_setting.GetMonitorSetting().AutoTestChannelMinutes
				time.Sleep(time.Duration(int(math.Round(frequency))) * time.Minute)
				common.SysLog(fmt.Sprintf("automatically test channels with interval %f minutes", frequency))
				common.SysLog("automatically testing all channels")
				_ = testAllChannels(false)
				common.SysLog("automatically channel test finished")
				if !operation_setting.GetMonitorSetting().AutoTestChannelEnabled {
					break
				}
			}
		}
	})
}

// ScheduledTestChannels 独立定时测试渠道
var scheduledTestChannelsOnce sync.Once
var hasRecentLLMRequestForScheduledTest = common.HasRecentLLMRequest
var scheduledTestDispatchFunc = func(channel *model.Channel) {
	gopool.Go(func() {
		testScheduledChannel(channel)
	})
}

func dispatchScheduledTestsCycle(channels []*model.Channel, nextTestTimes map[int]int64, now int64) {
	if len(channels) == 0 {
		return
	}

	if !hasRecentLLMRequestForScheduledTest() {
		common.SysLog(fmt.Sprintf("scheduled tests skipped: system idle (>1 hour), channels_count=%d", len(channels)))
		return
	}

	for _, channel := range channels {
		interval := channel.GetScheduledTestInterval()
		if interval <= 0 {
			continue
		}

		nextTestTime, exists := nextTestTimes[channel.Id]

		// 如果是第一次或者到了测试时间
		if !exists || now >= nextTestTime {
			common.SysLog(fmt.Sprintf(
				"scheduled test dispatch: channel_id=%d, channel_name=%s, interval_min=%d",
				channel.Id,
				channel.Name,
				interval,
			))
			scheduledTestDispatchFunc(channel)

			// 更新下次测试时间
			nextTestTimes[channel.Id] = now + int64(interval*60)
		}
	}
}

func ScheduledTestChannels() {
	scheduledTestChannelsOnce.Do(func() {
		// 存储每个渠道的下次测试时间
		nextTestTimes := make(map[int]int64)

		for {
			// 每分钟检查一次
			time.Sleep(1 * time.Minute)

			if !common.AutomaticDisableChannelEnabled {
				continue
			}

			channels, err := model.GetChannelsWithScheduledTest()
			if err != nil {
				common.SysLog(fmt.Sprintf("failed to get channels with scheduled test: %s", err.Error()))
				continue
			}

			dispatchScheduledTestsCycle(channels, nextTestTimes, time.Now().Unix())
		}
	})
}

func testScheduledChannel(channel *model.Channel) {
	if channel == nil {
		return
	}

	if !common.AutomaticDisableChannelEnabled {
		return
	}

	// 如果渠道未启用自动禁用，跳过定时测试
	if !channel.GetAutoBan() {
		return
	}

	channelcache.Remember(channel.Id, channel.Name)
	channelLabel := channelcache.Label(channel.Id, channel.Name)
	startAt := time.Now()
	resultTag := "unknown"
	resultDetail := ""
	common.SysLog(fmt.Sprintf("scheduled test started: %s", channelLabel))
	defer func() {
		elapsed := time.Since(startAt).Round(time.Millisecond)
		if resultTag == "unknown" {
			resultTag = "panic_or_unexpected"
		}
		if resultDetail == "" {
			resultDetail = "-"
		}
		common.SysLog(fmt.Sprintf(
			"scheduled test finished: %s, result=%s, detail=%s, elapsed=%s",
			channelLabel,
			resultTag,
			resultDetail,
			elapsed,
		))
	}()

	defer func() {
		if r := recover(); r != nil {
			resultTag = "panic"
			resultDetail = fmt.Sprintf("%v", r)
			common.SysLog(fmt.Sprintf("scheduled test %s panic: %v", channelLabel, r))
		}
	}()

	maxLatency := channel.GetMaxFirstTokenLatency()
	if maxLatency <= 0 {
		resultTag = "skipped"
		resultDetail = "max_first_token_latency_not_configured"
		// 如果没有设置最大首Token延迟，则记录跳过日志并退出
		// testModel := ""
		// if channel.TestModel != nil {
		// 	testModel = *channel.TestModel
		// }
		// model.RecordScheduledTestLog(model.ScheduledTestLogParams{
		// 	ChannelID:   channel.Id,
		// 	ChannelName: channel.Name,
		// 	ModelName:   testModel,
		// 	Result:      "skipped",
		// 	Message:     fmt.Sprintf("Scheduled test skipped: max_first_token_latency not configured for channel \"%s\" (#%d)", channel.Name, channel.Id),
		// 	Group:       "default",
		// 	IsStream:    true,
		// })
		return
	}

	// common.SysLog(fmt.Sprintf("scheduled testing channel #%d (%s)", channel.Id, channel.Name))

	// 如果动态熔断已启用且渠道当前处于冷却期，跳过本次测试以节省 token。
	// 仅当渠道进入"待探测"阶段（awaiting_probe）时才允许执行探测请求。
	if latest, err := model.GetChannelById(channel.Id, true); err == nil && latest != nil {
		if latest.IsBreakerCoolingAt(time.Now().Unix()) {
			resultTag = "skipped"
			resultDetail = "channel_in_breaker_cooldown"
			return
		}
		channel = latest
	}

	// 执行流式渠道测试以测量首Token延迟
	testModel := ""
	if channel.TestModel != nil {
		testModel = *channel.TestModel
	}
	result := testChannelStream(channel, testModel)

	promptTokens := 0
	completionTokens := 0
	groupValue := "default"
	useTimeSeconds := 0
	isStream := true
	if result.context != nil {
		if g := strings.TrimSpace(result.context.GetString("group")); g != "" {
			groupValue = g
		}
		promptTokens = result.context.GetInt("scheduled_test_prompt_tokens")
		completionTokens = result.context.GetInt("scheduled_test_completion_tokens")
		durationMs := result.context.GetInt("scheduled_test_duration_ms")
		if durationMs > 0 {
			useTimeSeconds = (durationMs + 999) / 1000
		}
		if val, exists := result.context.Get("scheduled_test_is_stream"); exists {
			if streamFlag, ok := val.(bool); ok {
				isStream = streamFlag
			}
		} else if result.context.GetBool("scheduled_test_is_stream") {
			isStream = true
		}
	}

	baseParams := model.ScheduledTestLogParams{
		ChannelID:        channel.Id,
		ChannelName:      channel.Name,
		ModelName:        testModel,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		UseTimeSeconds:   useTimeSeconds,
		Group:            groupValue,
		IsStream:         isStream,
	}
	if latest, err := model.GetChannelById(channel.Id, true); err == nil && latest != nil {
		channel = latest
	}
	dynamicBreakerEnabled := channel.IsDynamicCircuitBreakerEnabled()

	if result.localErr != nil {
		// 测试失败
		resultTag = "failure"
		resultDetail = result.localErr.Error()
		autoAction := ""
		if dynamicBreakerEnabled {
			autoAction = "breaker_penalized"
			breakerErr := result.newAPIError
			if breakerErr == nil {
				breakerErr = types.NewErrorWithStatusCode(
					fmt.Errorf("scheduled probe failed: %w", result.localErr),
					types.ErrorCodeBadResponse,
					http.StatusBadGateway,
				)
			}
			service.RecordChannelProbeFailure(channel, breakerErr)
		} else if channel.Status == common.ChannelStatusEnabled && channel.GetAutoBan() {
			// 如果渠道当前是启用状态，则禁用它
			autoAction = "auto_disabled"
			service.DisableChannel(*types.NewChannelError(
				channel.Id,
				channel.Type,
				channel.Name,
				channel.ChannelInfo.IsMultiKey,
				"",
				channel.GetAutoBan(),
			), fmt.Sprintf("定时测试失败: %s", result.localErr.Error()))
		}
		errMsg := result.localErr.Error()
		params := baseParams
		params.Result = "failure"
		params.Message = fmt.Sprintf("Scheduled test failed: %s", errMsg)
		params.Error = errMsg
		params.AutoAction = autoAction
		// model.RecordScheduledTestLog(params)
		return
	}

	// 检查首Token延迟
	if result.context != nil {
		if firstTokenLatencyMs, hasFirstTokenLatency := scheduledTestFirstTokenLatencyMs(result.context); hasFirstTokenLatency {
			// record first token latency to keep channel response_time in sync with latest scheduled test
			channel.UpdateResponseTime(int64(firstTokenLatencyMs))

			// Convert maxLatency from seconds to milliseconds
			maxLatencyMs := maxLatency * 1000
			latency := firstTokenLatencyMs
			threshold := maxLatencyMs
			if firstTokenLatencyMs > maxLatencyMs {
				// 延迟超过阈值
				resultTag = "failure"
				resultDetail = fmt.Sprintf("first_token_latency_exceeded: %dms > %dms", firstTokenLatencyMs, maxLatencyMs)
				if dynamicBreakerEnabled {
					autoAction := "breaker_penalized"
					service.RecordChannelProbeFailure(
						channel,
						types.NewErrorWithStatusCode(
							fmt.Errorf("first token latency %dms exceeds threshold %dms", firstTokenLatencyMs, maxLatencyMs),
							types.ErrorCodeChannelFirstTokenLatencyExceeded,
							http.StatusGatewayTimeout,
						),
					)
					params := baseParams
					params.Result = "failure"
					//params.Message = fmt.Sprintf("Scheduled test latency %dms exceeds threshold %dms", firstTokenLatencyMs, maxLatencyMs)
					params.LatencyMs = &latency
					params.ThresholdMs = &threshold
					params.AutoAction = autoAction
					// model.RecordScheduledTestLog(params)
				} else if channel.Status == common.ChannelStatusEnabled && channel.GetAutoBan() {
					// 如果渠道当前是启用状态，则禁用它
					autoAction := "auto_disabled"
					service.DisableChannel(*types.NewChannelError(
						channel.Id,
						channel.Type,
						channel.Name,
						channel.ChannelInfo.IsMultiKey,
						"",
						channel.GetAutoBan(),
					), fmt.Sprintf("首Token延迟 %dms 超过最大值 %dms", firstTokenLatencyMs, maxLatencyMs))
					params := baseParams
					params.Result = "failure"
					//params.Message = fmt.Sprintf("Scheduled test latency %dms exceeds threshold %dms", firstTokenLatencyMs, maxLatencyMs)
					params.LatencyMs = &latency
					params.ThresholdMs = &threshold
					params.AutoAction = autoAction
					// model.RecordScheduledTestLog(params)
				} else {
					params := baseParams
					params.Result = "failure"
					//params.Message = fmt.Sprintf("Scheduled test latency %dms exceeds threshold %dms", firstTokenLatencyMs, maxLatencyMs)
					params.LatencyMs = &latency
					params.ThresholdMs = &threshold
					// model.RecordScheduledTestLog(params)
				}
			} else {
				// 延迟在阈值内
				resultTag = "success"
				resultDetail = fmt.Sprintf("first_token_latency_ok: %dms <= %dms", firstTokenLatencyMs, maxLatencyMs)
				if dynamicBreakerEnabled {
					autoAction := ""
					if service.RecordChannelProbeSuccess(channel) {
						autoAction = "breaker_probation"
						if channel.Status != common.ChannelStatusEnabled {
							service.EnableChannel(channel.Id, "", channel.Name)
							autoAction = "auto_enabled_probation"
						}
					}
					params := baseParams
					params.Result = "success"
					params.Message = fmt.Sprintf("Scheduled test latency %dms within threshold %dms", firstTokenLatencyMs, maxLatencyMs)
					params.LatencyMs = &latency
					params.ThresholdMs = &threshold
					if autoAction != "" {
						params.AutoAction = autoAction
					}
					// model.RecordScheduledTestLog(params)
				} else if channel.Status != common.ChannelStatusEnabled {
					// 如果渠道当前是禁用状态，则重新启用它
					autoAction := "auto_enabled"
					service.EnableChannel(channel.Id, "", channel.Name)
					params := baseParams
					params.Result = "success"
					params.Message = fmt.Sprintf("Scheduled test latency %dms within threshold %dms", firstTokenLatencyMs, maxLatencyMs)
					params.LatencyMs = &latency
					params.ThresholdMs = &threshold
					params.AutoAction = autoAction
					// model.RecordScheduledTestLog(params)
				} else {
					params := baseParams
					params.Result = "success"
					params.Message = fmt.Sprintf("Scheduled test latency %dms within threshold %dms", firstTokenLatencyMs, maxLatencyMs)
					params.LatencyMs = &latency
					params.ThresholdMs = &threshold
					// model.RecordScheduledTestLog(params)
				}
			}
		} else {
			resultDetail = "first_token_latency_not_measured"
			params := baseParams
			if dynamicBreakerEnabled {
				resultTag = "failure"
				autoAction := "breaker_penalized"
				breakerErr := types.NewErrorWithStatusCode(
					fmt.Errorf("scheduled probe failed: first token latency not measured"),
					types.ErrorCodeBadResponseBody,
					http.StatusBadGateway,
				)
				service.RecordChannelProbeFailure(channel, breakerErr)
				params.Result = "failure"
				params.Message = "Scheduled probe failed: first token latency not measured"
				params.Error = breakerErr.Error()
				params.AutoAction = autoAction
			} else {
				resultTag = "warning"
				params.Result = "warning"
				params.Message = "Scheduled test could not measure first token latency"
			}
			// model.RecordScheduledTestLog(params)
		}
	} else {
		resultDetail = "scheduled_test_context_missing"
		params := baseParams
		if dynamicBreakerEnabled {
			resultTag = "failure"
			autoAction := "breaker_penalized"
			breakerErr := types.NewErrorWithStatusCode(
				fmt.Errorf("scheduled probe failed: context missing"),
				types.ErrorCodeBadResponseBody,
				http.StatusBadGateway,
			)
			service.RecordChannelProbeFailure(channel, breakerErr)
			params.Result = "failure"
			params.Message = "Scheduled probe failed: context missing"
			params.Error = breakerErr.Error()
			params.AutoAction = autoAction
		} else {
			resultTag = "warning"
			params.Result = "warning"
			params.Message = "Scheduled test completed without context information"
		}
		// model.RecordScheduledTestLog(params)
	}
}

// testChannelStream 专门用于定时测试的流式测试函数，测量首Token延迟
func testChannelStream(channel *model.Channel, testModel string) testResult {
	startTime := time.Now()

	if lo.Contains(unsupportedTestChannelTypes, channel.Type) {
		channelTypeName := constant.GetChannelTypeName(channel.Type)
		return testResult{
			localErr: fmt.Errorf("%s channel test is not supported", channelTypeName),
		}
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	testModel = resolveTestModel(channel, testModel)
	requestPath := resolveTestRequestPath(channel, testModel, "")
	c.Request = &http.Request{
		Method: "POST",
		URL:    &url.URL{Path: requestPath},
		Body:   nil,
		Header: make(http.Header),
	}

	cache, err := model.GetUserCache(1)
	if err != nil {
		return testResult{
			localErr:    err,
			newAPIError: nil,
		}
	}
	cache.WriteContext(c)

	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("channel", channel.Type)
	c.Set("base_url", channel.GetBaseURL())
	group, _ := model.GetUserGroup(1, false)
	c.Set("group", group)

	newAPIError := middleware.SetupContextForSelectedChannel(c, channel, testModel)
	if newAPIError != nil {
		return testResult{
			context:     c,
			localErr:    newAPIError,
			newAPIError: newAPIError,
		}
	}

	request := buildTestRequest(testModel, "", channel, true)
	if generalReq, ok := request.(*dto.GeneralOpenAIRequest); ok {
		generalReq.Stream = lo.ToPtr(true)
		if strings.HasPrefix(testModel, "o") {
			generalReq.MaxCompletionTokens = lo.ToPtr(uint(10))
			generalReq.MaxTokens = lo.ToPtr(uint(0))
		} else if lo.FromPtr(generalReq.MaxTokens) == 0 && lo.FromPtr(generalReq.MaxCompletionTokens) == 0 {
			generalReq.MaxTokens = lo.ToPtr(uint(10))
		}
	}

	relayFormat := detectRelayFormat("", requestPath)
	info, err := relaycommon.GenRelayInfo(c, relayFormat, request, nil)
	if err != nil {
		return testResult{
			context:     c,
			localErr:    err,
			newAPIError: types.NewError(err, types.ErrorCodeGenRelayInfoFailed),
		}
	}

	info.InitChannelMeta(c)

	if info.ChannelMeta != nil {
		if maxLatency := info.ChannelMeta.MaxFirstTokenLatencySeconds; maxLatency > 0 {
			helper.EnsureFirstTokenWatchdog(c, info, maxLatency, nil)
		}
	}

	if meta := request.GetTokenCountMeta(); meta != nil {
		tokens, countErr := service.EstimateRequestToken(c, meta, info)
		if countErr == nil {
			info.SetEstimatePromptTokens(tokens)
			c.Set("scheduled_test_prompt_tokens", tokens)
		}
	}
	c.Set("scheduled_test_is_stream", info.IsStream)

	err = helper.ModelMappedHelper(c, info, request)
	if err != nil {
		return testResult{
			context:     c,
			localErr:    err,
			newAPIError: types.NewError(err, types.ErrorCodeChannelModelMappedError),
		}
	}

	testModel = info.UpstreamModelName
	request.SetModelName(testModel)

	apiType, _ := common.ChannelType2APIType(channel.Type)
	adaptor := relay.GetAdaptor(apiType)
	if adaptor == nil {
		return testResult{
			context:     c,
			localErr:    fmt.Errorf("invalid api type: %d, adaptor is nil", apiType),
			newAPIError: types.NewError(fmt.Errorf("invalid api type: %d, adaptor is nil", apiType), types.ErrorCodeInvalidApiType),
		}
	}

	adaptor.Init(info)

	convertedRequest, err := convertRequestForAdaptor(c, info, adaptor, request)
	if err != nil {
		return testResult{
			context:     c,
			localErr:    err,
			newAPIError: types.NewError(err, types.ErrorCodeConvertRequestFailed),
		}
	}

	if channel.Type == constant.ChannelTypeGemini {
		if geminiReq, ok := convertedRequest.(*dto.GeminiChatRequest); ok {
			modelName := strings.ToLower(info.UpstreamModelName)
			if strings.Contains(modelName, "gemini-3-pro") ||
				strings.Contains(modelName, "gemini-3-flash") ||
				strings.Contains(modelName, "gemini-2.5-pro") ||
				strings.Contains(modelName, "gemini-2.5-flash") {
				if geminiReq.GenerationConfig.ThinkingConfig == nil {
					geminiReq.GenerationConfig.ThinkingConfig = &dto.GeminiThinkingConfig{}
				}
				if strings.Contains(modelName, "gemini-3-pro") {
					geminiReq.GenerationConfig.ThinkingConfig.ThinkingLevel = "low"
				}
				if strings.Contains(modelName, "gemini-3-flash") {
					geminiReq.GenerationConfig.ThinkingConfig.ThinkingLevel = "minimal"
				}
				if strings.Contains(modelName, "gemini-2.5-pro") {
					budget := 128
					geminiReq.GenerationConfig.ThinkingConfig.ThinkingBudget = &budget
				}
				if strings.Contains(modelName, "gemini-2.5-flash") {
					budget := 0
					geminiReq.GenerationConfig.ThinkingConfig.ThinkingBudget = &budget
				}
			}
		}
	}

	jsonData, err := json.Marshal(convertedRequest)
	if err != nil {
		return testResult{
			context:     c,
			localErr:    err,
			newAPIError: types.NewError(err, types.ErrorCodeJsonMarshalFailed),
		}
	}

	requestBody := bytes.NewBuffer(jsonData)
	c.Request.Body = io.NopCloser(requestBody)

	resp, err := adaptor.DoRequest(c, info, requestBody)
	if err != nil {
		return testResult{
			context:     c,
			localErr:    err,
			newAPIError: types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError),
		}
	}

	var httpResp *http.Response
	if resp != nil {
		httpResp = resp.(*http.Response)
		if httpResp.StatusCode != http.StatusOK {
			err := service.RelayErrorHandler(c.Request.Context(), httpResp, true)
			return testResult{
				context:     c,
				localErr:    err,
				newAPIError: types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError),
			}
		}
	}

	watchdog, _ := common.GetContextKeyType[*helper.FirstTokenWatchdog](c, constant.ContextKeyFirstTokenWatchdog)
	defer func() {
		if watchdog != nil {
			watchdog.Stop("scheduled test finished")
			common.SetContextKey(c, constant.ContextKeyFirstTokenWatchdog, nil)
		}
	}()

	// 读取流式响应并测量首Token延迟
	firstTokenTime := time.Duration(0)
	scanner := bufio.NewScanner(httpResp.Body)
	maxBufferSize := 64 << 20 // 64MB default SSE buffer size
	scanner.Buffer(make([]byte, helper.InitialScannerBufferSize), maxBufferSize)
	scanner.Split(bufio.ScanLines)

	gotFirstToken := false
	checkExpectedAnswer := channel != nil && channel.ExpectedAnswer != nil && strings.TrimSpace(*channel.ExpectedAnswer) != ""
	var contentBuilder strings.Builder
	for scanner.Scan() {
		rawLine := scanner.Text()
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		if strings.EqualFold(line, "[DONE]") {
			break
		}

		// ignore SSE comments like ": ping"
		if strings.HasPrefix(line, ":") {
			continue
		}

		var normalized string
		switch {
		case len(line) >= 6 && strings.EqualFold(line[:6], "data: "):
			normalized = strings.TrimSpace(line[6:])
		case len(line) >= 5 && strings.EqualFold(line[:5], "data:"):
			normalized = strings.TrimSpace(line[5:])
		default:
			continue
		}

		if normalized == "" {
			continue
		}

		if strings.EqualFold(normalized, "[DONE]") {
			break
		}

		if checkExpectedAnswer {
			var streamResponse map[string]interface{}
			if err := json.Unmarshal([]byte(normalized), &streamResponse); err == nil {
				if choices, ok := streamResponse["choices"].([]interface{}); ok && len(choices) > 0 {
					if choice, ok := choices[0].(map[string]interface{}); ok {
						if delta, ok := choice["delta"].(map[string]interface{}); ok {
							if content, ok := delta["content"].(string); ok {
								contentBuilder.WriteString(content)
							}
						}
					}
				}

				if candidates, ok := streamResponse["candidates"].([]interface{}); ok && len(candidates) > 0 {
					if candidate, ok := candidates[0].(map[string]interface{}); ok {
						if content, ok := candidate["content"].(map[string]interface{}); ok {
							if parts, ok := content["parts"].([]interface{}); ok {
								for _, part := range parts {
									if partMap, ok := part.(map[string]interface{}); ok {
										if text, ok := partMap["text"].(string); ok {
											contentBuilder.WriteString(text)
										}
									}
								}
							}
						}
					}
				}
			}
		}

		if !gotFirstToken && normalized != "" {
			firstTokenTime = time.Since(startTime)
			durationMs := int(firstTokenTime.Milliseconds())
			c.Set("scheduled_test_duration_ms", durationMs)
			completionTokens := 1
			var streamResp dto.ChatCompletionsStreamResponse
			if err := common.Unmarshal([]byte(normalized), &streamResp); err == nil {
				if len(streamResp.Choices) > 0 && streamResp.Choices[0].Delta.GetContentString() != "" {
					contentTokens := service.CountTextToken(streamResp.Choices[0].Delta.GetContentString(), testModel)
					if contentTokens > 0 {
						completionTokens = contentTokens
					}
				}
			}
			c.Set("scheduled_test_completion_tokens", completionTokens)
			gotFirstToken = true
			if watchdog != nil {
				watchdog.Stop("scheduled test first token received")
			}
			if !checkExpectedAnswer {
				break // 只需要测量首Token，不需要读取全部响应
			}
		}
	}

	if httpResp.Body != nil {
		httpResp.Body.Close()
	}

	if err := scanner.Err(); err != nil && !gotFirstToken {
		return testResult{
			context:     c,
			localErr:    err,
			newAPIError: types.NewError(err, types.ErrorCodeReadResponseBodyFailed),
		}
	}

	if !gotFirstToken {
		return testResult{
			context:     c,
			localErr:    errors.New("no first token received"),
			newAPIError: types.NewError(errors.New("no first token received"), types.ErrorCodeBadResponseBody),
		}
	}

	if checkExpectedAnswer {
		expectedAnswer := strings.TrimSpace(*channel.ExpectedAnswer)
		responseContent := contentBuilder.String()
		if !strings.Contains(strings.ToLower(responseContent), strings.ToLower(expectedAnswer)) {
			errMsg := fmt.Sprintf("expected answer not found in response: expected '%s'", expectedAnswer)
			return testResult{
				context:     c,
				localErr:    errors.New(errMsg),
				newAPIError: types.NewError(errors.New(errMsg), types.ErrorCodeBadResponseBody),
			}
		}
	}

	// 将首Token延迟存储到context中（以毫秒为单位）
	c.Set("first_token_latency_ms", int(firstTokenTime.Milliseconds()))

	return testResult{
		context:     c,
		localErr:    nil,
		newAPIError: nil,
	}
}
