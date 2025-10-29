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

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/samber/lo"

	"github.com/gin-gonic/gin"
)

type testResult struct {
	context     *gin.Context
	localErr    error
	newAPIError *types.NewAPIError
}

func testChannel(channel *model.Channel, testModel string, endpointType string) testResult {
	tik := time.Now()
	var unsupportedTestChannelTypes = []int{
		constant.ChannelTypeMidjourney,
		constant.ChannelTypeMidjourneyPlus,
		constant.ChannelTypeSunoAPI,
		constant.ChannelTypeKling,
		constant.ChannelTypeJimeng,
		constant.ChannelTypeDoubaoVideo,
		constant.ChannelTypeVidu,
	}
	if lo.Contains(unsupportedTestChannelTypes, channel.Type) {
		channelTypeName := constant.GetChannelTypeName(channel.Type)
		return testResult{
			localErr: fmt.Errorf("%s channel test is not supported", channelTypeName),
		}
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	testModel = strings.TrimSpace(testModel)
	if testModel == "" {
		if channel.TestModel != nil && *channel.TestModel != "" {
			testModel = strings.TrimSpace(*channel.TestModel)
		} else {
			models := channel.GetModels()
			if len(models) > 0 {
				testModel = strings.TrimSpace(models[0])
			}
			if testModel == "" {
				testModel = "gpt-4o-mini"
			}
		}
	}

	requestPath := "/v1/chat/completions"

	// 如果指定了端点类型，使用指定的端点类型
	if endpointType != "" {
		if endpointInfo, ok := common.GetDefaultEndpointInfo(constant.EndpointType(endpointType)); ok {
			requestPath = endpointInfo.Path
		}
	} else {
		// 如果没有指定端点类型，使用原有的自动检测逻辑
		// 先判断是否为 Embedding 模型
		if strings.Contains(strings.ToLower(testModel), "embedding") ||
			strings.HasPrefix(testModel, "m3e") || // m3e 系列模型
			strings.Contains(testModel, "bge-") || // bge 系列模型
			strings.Contains(testModel, "embed") ||
			channel.Type == constant.ChannelTypeMokaAI { // 其他 embedding 模型
			requestPath = "/v1/embeddings" // 修改请求路径
		}

		// VolcEngine 图像生成模型
		if channel.Type == constant.ChannelTypeVolcEngine && strings.Contains(testModel, "seedream") {
			requestPath = "/v1/images/generations"
		}
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

	//c.Request.Header.Set("Authorization", "Bearer "+channel.Key)
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
	var relayFormat types.RelayFormat
	if endpointType != "" {
		// 根据指定的端点类型设置 relayFormat
		switch constant.EndpointType(endpointType) {
		case constant.EndpointTypeOpenAI:
			relayFormat = types.RelayFormatOpenAI
		case constant.EndpointTypeOpenAIResponse:
			relayFormat = types.RelayFormatOpenAIResponses
		case constant.EndpointTypeAnthropic:
			relayFormat = types.RelayFormatClaude
		case constant.EndpointTypeGemini:
			relayFormat = types.RelayFormatGemini
		case constant.EndpointTypeJinaRerank:
			relayFormat = types.RelayFormatRerank
		case constant.EndpointTypeImageGeneration:
			relayFormat = types.RelayFormatOpenAIImage
		case constant.EndpointTypeEmbeddings:
			relayFormat = types.RelayFormatEmbedding
		default:
			relayFormat = types.RelayFormatOpenAI
		}
	} else {
		// 根据请求路径自动检测
		relayFormat = types.RelayFormatOpenAI
		if c.Request.URL.Path == "/v1/embeddings" {
			relayFormat = types.RelayFormatEmbedding
		}
		if c.Request.URL.Path == "/v1/images/generations" {
			relayFormat = types.RelayFormatOpenAIImage
		}
		if c.Request.URL.Path == "/v1/messages" {
			relayFormat = types.RelayFormatClaude
		}
		if strings.Contains(c.Request.URL.Path, "/v1beta/models") {
			relayFormat = types.RelayFormatGemini
		}
		if c.Request.URL.Path == "/v1/rerank" || c.Request.URL.Path == "/rerank" {
			relayFormat = types.RelayFormatRerank
		}
		if c.Request.URL.Path == "/v1/responses" {
			relayFormat = types.RelayFormatOpenAIResponses
		}
	}

	request := buildTestRequest(testModel, endpointType)

	info, err := relaycommon.GenRelayInfo(c, relayFormat, request, nil)

	if err != nil {
		return testResult{
			context:     c,
			localErr:    err,
			newAPIError: types.NewError(err, types.ErrorCodeGenRelayInfoFailed),
		}
	}

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
	adaptor := relay.GetAdaptor(apiType)
	if adaptor == nil {
		return testResult{
			context:     c,
			localErr:    fmt.Errorf("invalid api type: %d, adaptor is nil", apiType),
			newAPIError: types.NewError(fmt.Errorf("invalid api type: %d, adaptor is nil", apiType), types.ErrorCodeInvalidApiType),
		}
	}

	//// 创建一个用于日志的 info 副本，移除 ApiKey
	//logInfo := info
	//logInfo.ApiKey = ""
	common.SysLog(fmt.Sprintf("testing channel %d with model %s , info %+v ", channel.Id, testModel, info.ToString()))

	priceData, err := helper.ModelPriceHelper(c, info, 0, request.GetTokenCountMeta())
	if err != nil {
		return testResult{
			context:     c,
			localErr:    err,
			newAPIError: types.NewError(err, types.ErrorCodeModelPriceError),
		}
	}

	adaptor.Init(info)

	var convertedRequest any
	// 根据 RelayMode 选择正确的转换函数
	switch info.RelayMode {
	case relayconstant.RelayModeEmbeddings:
		// Embedding 请求 - request 已经是正确的类型
		if embeddingReq, ok := request.(*dto.EmbeddingRequest); ok {
			convertedRequest, err = adaptor.ConvertEmbeddingRequest(c, info, *embeddingReq)
		} else {
			return testResult{
				context:     c,
				localErr:    errors.New("invalid embedding request type"),
				newAPIError: types.NewError(errors.New("invalid embedding request type"), types.ErrorCodeConvertRequestFailed),
			}
		}
	case relayconstant.RelayModeImagesGenerations:
		// 图像生成请求 - request 已经是正确的类型
		if imageReq, ok := request.(*dto.ImageRequest); ok {
			convertedRequest, err = adaptor.ConvertImageRequest(c, info, *imageReq)
		} else {
			return testResult{
				context:     c,
				localErr:    errors.New("invalid image request type"),
				newAPIError: types.NewError(errors.New("invalid image request type"), types.ErrorCodeConvertRequestFailed),
			}
		}
	case relayconstant.RelayModeRerank:
		// Rerank 请求 - request 已经是正确的类型
		if rerankReq, ok := request.(*dto.RerankRequest); ok {
			convertedRequest, err = adaptor.ConvertRerankRequest(c, info.RelayMode, *rerankReq)
		} else {
			return testResult{
				context:     c,
				localErr:    errors.New("invalid rerank request type"),
				newAPIError: types.NewError(errors.New("invalid rerank request type"), types.ErrorCodeConvertRequestFailed),
			}
		}
	case relayconstant.RelayModeResponses:
		// Response 请求 - request 已经是正确的类型
		if responseReq, ok := request.(*dto.OpenAIResponsesRequest); ok {
			convertedRequest, err = adaptor.ConvertOpenAIResponsesRequest(c, info, *responseReq)
		} else {
			return testResult{
				context:     c,
				localErr:    errors.New("invalid response request type"),
				newAPIError: types.NewError(errors.New("invalid response request type"), types.ErrorCodeConvertRequestFailed),
			}
		}
	default:
		// Chat/Completion 等其他请求类型
		if generalReq, ok := request.(*dto.GeneralOpenAIRequest); ok {
			convertedRequest, err = adaptor.ConvertOpenAIRequest(c, info, generalReq)
		} else {
			return testResult{
				context:     c,
				localErr:    errors.New("invalid general request type"),
				newAPIError: types.NewError(errors.New("invalid general request type"), types.ErrorCodeConvertRequestFailed),
			}
		}
	}

	if err != nil {
		return testResult{
			context:     c,
			localErr:    err,
			newAPIError: types.NewError(err, types.ErrorCodeConvertRequestFailed),
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
	usageA, respErr := adaptor.DoResponse(c, httpResp, info)
	if respErr != nil {
		return testResult{
			context:     c,
			localErr:    respErr,
			newAPIError: respErr,
		}
	}
	if usageA == nil {
		return testResult{
			context:     c,
			localErr:    errors.New("usage is nil"),
			newAPIError: types.NewOpenAIError(errors.New("usage is nil"), types.ErrorCodeBadResponseBody, http.StatusInternalServerError),
		}
	}
	usage := usageA.(*dto.Usage)
	result := w.Result()
	respBody, err := io.ReadAll(result.Body)
	if err != nil {
		return testResult{
			context:     c,
			localErr:    err,
			newAPIError: types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError),
		}
	}
	info.PromptTokens = usage.PromptTokens

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

func buildTestRequest(model string, endpointType string) dto.Request {
	// 根据端点类型构建不同的测试请求
	if endpointType != "" {
		switch constant.EndpointType(endpointType) {
		case constant.EndpointTypeEmbeddings:
			// 返回 EmbeddingRequest
			return &dto.EmbeddingRequest{
				Model: model,
				Input: []any{"hello world"},
			}
		case constant.EndpointTypeImageGeneration:
			// 返回 ImageRequest
			return &dto.ImageRequest{
				Model:  model,
				Prompt: "a cute cat",
				N:      1,
				Size:   "1024x1024",
			}
		case constant.EndpointTypeJinaRerank:
			// 返回 RerankRequest
			return &dto.RerankRequest{
				Model:     model,
				Query:     "What is Deep Learning?",
				Documents: []any{"Deep Learning is a subset of machine learning.", "Machine learning is a field of artificial intelligence."},
				TopN:      2,
			}
		case constant.EndpointTypeOpenAIResponse:
			// 返回 OpenAIResponsesRequest
			return &dto.OpenAIResponsesRequest{
				Model: model,
				Input: json.RawMessage("\"hi\""),
			}
		case constant.EndpointTypeAnthropic, constant.EndpointTypeGemini, constant.EndpointTypeOpenAI:
			// 返回 GeneralOpenAIRequest
			maxTokens := uint(10)
			if constant.EndpointType(endpointType) == constant.EndpointTypeGemini {
				maxTokens = 3000
			}
			return &dto.GeneralOpenAIRequest{
				Model:  model,
				Stream: false,
				Messages: []dto.Message{
					{
						Role:    "user",
						Content: "hi",
					},
				},
				MaxTokens: maxTokens,
			}
		}
	}

	// 自动检测逻辑（保持原有行为）
	// 先判断是否为 Embedding 模型
	if strings.Contains(strings.ToLower(model), "embedding") ||
		strings.HasPrefix(model, "m3e") ||
		strings.Contains(model, "bge-") {
		// 返回 EmbeddingRequest
		return &dto.EmbeddingRequest{
			Model: model,
			Input: []any{"hello world"},
		}
	}

	// Chat/Completion 请求 - 返回 GeneralOpenAIRequest
	testRequest := &dto.GeneralOpenAIRequest{
		Model:  model,
		Stream: false,
		Messages: []dto.Message{
			{
				Role:    "user",
				Content: "hi",
			},
		},
	}

	if strings.HasPrefix(model, "o") {
		testRequest.MaxCompletionTokens = 10
	} else if strings.Contains(model, "thinking") {
		if !strings.Contains(model, "claude") {
			testRequest.MaxTokens = 50
		}
	} else if strings.Contains(model, "gemini") {
		testRequest.MaxTokens = 3000
	} else {
		testRequest.MaxTokens = 10
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
	//defer func() {
	//	if channel.ChannelInfo.IsMultiKey {
	//		go func() { _ = channel.SaveChannelInfo() }()
	//	}
	//}()
	testModel := c.Query("model")
	endpointType := c.Query("endpoint_type")
	tik := time.Now()
	result := testChannel(channel, testModel, endpointType)
	if result.localErr != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": result.localErr.Error(),
			"time":    0.0,
		})
		return
	}
	tok := time.Now()
	milliseconds := tok.Sub(tik).Milliseconds()
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
			isChannelEnabled := channel.Status == common.ChannelStatusEnabled
			tik := time.Now()
			result := testChannel(channel, "", "")
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

			// disable channel
			if isChannelEnabled && shouldBanChannel && channel.GetAutoBan() {
				processChannelError(result.context, *types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey, common.GetContextKeyString(result.context, constant.ContextKeyChannelKey), channel.GetAutoBan()), newAPIError)
			}

			// enable channel
			if !isChannelEnabled && service.ShouldEnableChannel(newAPIError, channel.Status) {
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

func isWithinTestTime() bool {
	now := time.Now()
	hour := now.Hour()
	minute := now.Minute()

	// 8:00 - 11:30
	if hour >= 8 && hour < 12 {
		if hour == 11 && minute > 30 {
			return false
		}
		return true
	}

	// 14:00 - 21:00
	if hour >= 14 && hour <= 21 {
		return true
	}

	return false
}

var autoTestChannelsOnce sync.Once

func AutomaticallyTestChannels() {
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

				// Check if current time is within allowed testing hours
				if !isWithinTestTime() {
					continue
				}

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

func ScheduledTestChannels() {
	scheduledTestChannelsOnce.Do(func() {
		// 存储每个渠道的下次测试时间
		nextTestTimes := make(map[int]int64)
		var mu sync.Mutex

		for {
			// 每分钟检查一次
			time.Sleep(1 * time.Minute)

			channels, err := model.GetChannelsWithScheduledTest()
			if err != nil {
				common.SysLog(fmt.Sprintf("failed to get channels with scheduled test: %s", err.Error()))
				continue
			}

			if len(channels) == 0 {
				continue
			}

			now := time.Now().Unix()

			for _, channel := range channels {
				interval := channel.GetScheduledTestInterval()
				if interval <= 0 {
					continue
				}

				mu.Lock()
				nextTestTime, exists := nextTestTimes[channel.Id]
				mu.Unlock()

				// 如果是第一次或者到了测试时间
				if !exists || now >= nextTestTime {
					// 异步测试渠道
					gopool.Go(func() {
						testScheduledChannel(channel)
					})

					// 更新下次测试时间
					mu.Lock()
					nextTestTimes[channel.Id] = now + int64(interval*60)
					mu.Unlock()
				}
			}
		}
	})
}

func testScheduledChannel(channel *model.Channel) {
	defer func() {
		if r := recover(); r != nil {
			common.SysLog(fmt.Sprintf("scheduled test channel #%d panic: %v", channel.Id, r))
		}
	}()

	maxLatency := channel.GetMaxFirstTokenLatency()
	if maxLatency <= 0 {
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

	if result.localErr != nil {
		// 测试失败
		common.SysLog(fmt.Sprintf("scheduled test channel #%d failed: %s", channel.Id, result.localErr.Error()))
		autoAction := ""
		// 如果渠道当前是启用状态，则禁用它
		if channel.Status == 1 {
			autoAction = "auto_disabled"
			service.DisableChannel(*types.NewChannelError(
				channel.Id,
				channel.Type,
				channel.Name,
				channel.ChannelInfo.IsMultiKey,
				"",
				true,
			), fmt.Sprintf("定时测试失败: %s", result.localErr.Error()))
			common.SysLog(fmt.Sprintf("channel #%d disabled due to scheduled test failure", channel.Id))
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
		firstTokenLatencyMs := result.context.GetInt("first_token_latency_ms")
		if firstTokenLatencyMs > 0 {
			// Convert maxLatency from seconds to milliseconds
			maxLatencyMs := maxLatency * 1000
			latency := firstTokenLatencyMs
			threshold := maxLatencyMs
			if firstTokenLatencyMs > maxLatencyMs {
				// 延迟超过阈值
				common.SysLog(fmt.Sprintf("channel #%d first token latency %dms exceeds max %dms",
					channel.Id, firstTokenLatencyMs, maxLatencyMs))

				// 如果渠道当前是启用状态，则禁用它
				if channel.Status == 1 {
					autoAction := "auto_disabled"
					service.DisableChannel(*types.NewChannelError(
						channel.Id,
						channel.Type,
						channel.Name,
						channel.ChannelInfo.IsMultiKey,
						"",
						true,
					), fmt.Sprintf("首Token延迟 %dms 超过最大值 %dms", firstTokenLatencyMs, maxLatencyMs))
					common.SysLog(fmt.Sprintf("channel #%d disabled due to high latency", channel.Id))
					params := baseParams
					params.Result = "failure"
					params.Message = fmt.Sprintf("Scheduled test latency %dms exceeds threshold %dms", firstTokenLatencyMs, maxLatencyMs)
					params.LatencyMs = &latency
					params.ThresholdMs = &threshold
					params.AutoAction = autoAction
					// model.RecordScheduledTestLog(params)
				} else {
					params := baseParams
					params.Result = "failure"
					params.Message = fmt.Sprintf("Scheduled test latency %dms exceeds threshold %dms", firstTokenLatencyMs, maxLatencyMs)
					params.LatencyMs = &latency
					params.ThresholdMs = &threshold
					// model.RecordScheduledTestLog(params)
				}
			} else {
				// 延迟在阈值内
				common.SysLog(fmt.Sprintf("channel #%d first token latency %dms is within limit %dms",
					channel.Id, firstTokenLatencyMs, maxLatencyMs))

				// 如果渠道当前是禁用状态，则重新启用它
				if channel.Status != 1 {
					autoAction := "auto_enabled"
					service.EnableChannel(channel.Id, "", channel.Name)
					common.SysLog(fmt.Sprintf("channel #%d re-enabled due to acceptable latency", channel.Id))
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
			// 如果无法测量首Token延迟，记录警告
			common.SysLog(fmt.Sprintf("channel #%d: unable to measure first token latency", channel.Id))
			params := baseParams
			params.Result = "warning"
			params.Message = "Scheduled test could not measure first token latency"
			// model.RecordScheduledTestLog(params)
		}
	} else {
		params := baseParams
		params.Result = "warning"
		params.Message = "Scheduled test completed without context information"
		// model.RecordScheduledTestLog(params)
	}
}

// testChannelStream 专门用于定时测试的流式测试函数，测量首Token延迟
func testChannelStream(channel *model.Channel, testModel string) testResult {
	startTime := time.Now()

	var unsupportedTestChannelTypes = []int{
		constant.ChannelTypeMidjourney,
		constant.ChannelTypeMidjourneyPlus,
		constant.ChannelTypeSunoAPI,
		constant.ChannelTypeKling,
		constant.ChannelTypeJimeng,
		constant.ChannelTypeDoubaoVideo,
		constant.ChannelTypeVidu,
	}
	if lo.Contains(unsupportedTestChannelTypes, channel.Type) {
		channelTypeName := constant.GetChannelTypeName(channel.Type)
		return testResult{
			localErr: fmt.Errorf("%s channel test is not supported", channelTypeName),
		}
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	testModel = strings.TrimSpace(testModel)
	if testModel == "" {
		models := channel.GetModels()
		if len(models) > 0 {
			testModel = strings.TrimSpace(models[0])
		}
		if testModel == "" {
			testModel = "gpt-4o-mini"
		}
	}

	requestPath := "/v1/chat/completions"
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

	// 构建流式请求
	testRequest := &dto.GeneralOpenAIRequest{
		Model:  testModel,
		Stream: true, // 关键：设置为流式
		Messages: []dto.Message{
			{
				Role:    "user",
				Content: "hi",
			},
		},
	}

	// 设置合理的token限制
	if strings.HasPrefix(testModel, "o") {
		testRequest.MaxCompletionTokens = 10
	} else {
		testRequest.MaxTokens = 10
	}

	info, err := relaycommon.GenRelayInfo(c, types.RelayFormatOpenAI, testRequest, nil)
	if err != nil {
		return testResult{
			context:     c,
			localErr:    err,
			newAPIError: types.NewError(err, types.ErrorCodeGenRelayInfoFailed),
		}
	}

	info.InitChannelMeta(c)

	if meta := testRequest.GetTokenCountMeta(); meta != nil {
		tokens, countErr := service.CountRequestToken(c, meta, info)
		if countErr != nil {
			common.SysLog(fmt.Sprintf("scheduled test token counting failed: %v", countErr))
		} else {
			info.SetPromptTokens(tokens)
			c.Set("scheduled_test_prompt_tokens", tokens)
		}
	}
	c.Set("scheduled_test_is_stream", info.IsStream)

	err = helper.ModelMappedHelper(c, info, testRequest)
	if err != nil {
		return testResult{
			context:     c,
			localErr:    err,
			newAPIError: types.NewError(err, types.ErrorCodeChannelModelMappedError),
		}
	}

	testModel = info.UpstreamModelName
	testRequest.SetModelName(testModel)

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

	convertedRequest, err := adaptor.ConvertOpenAIRequest(c, info, testRequest)
	if err != nil {
		return testResult{
			context:     c,
			localErr:    err,
			newAPIError: types.NewError(err, types.ErrorCodeConvertRequestFailed),
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

	// 读取流式响应并测量首Token延迟
	firstTokenTime := time.Duration(0)
	scanner := bufio.NewScanner(httpResp.Body)
	scanner.Split(bufio.ScanLines)

	gotFirstToken := false
	for scanner.Scan() {
		data := scanner.Text()
		if len(data) < 6 || !strings.HasPrefix(data, "data: ") {
			continue
		}
		data = strings.TrimPrefix(data, "data: ")
		data = strings.TrimSpace(data)

		if data == "[DONE]" {
			break
		}

		if !gotFirstToken && data != "" {
			firstTokenTime = time.Since(startTime)
			durationMs := int(firstTokenTime.Milliseconds())
			c.Set("scheduled_test_duration_ms", durationMs)
			completionTokens := 1
			var streamResp dto.ChatCompletionsStreamResponse
			if err := common.Unmarshal([]byte(data), &streamResp); err == nil {
				if tokens := service.CountTokenStreamChoices(streamResp.Choices, testModel); tokens > 0 {
					completionTokens = tokens
				}
			}
			c.Set("scheduled_test_completion_tokens", completionTokens)
			gotFirstToken = true
			// common.SysLog(fmt.Sprintf("channel #%d first token received after %dms", channel.Id, firstTokenTime.Milliseconds()))
			break // 只需要测量首Token，不需要读取全部响应
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

	// 将首Token延迟存储到context中（以毫秒为单位）
	c.Set("first_token_latency_ms", int(firstTokenTime.Milliseconds()))

	return testResult{
		context:     c,
		localErr:    nil,
		newAPIError: nil,
	}
}
