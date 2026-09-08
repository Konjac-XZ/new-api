package openai

import (
	"net/http"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

type bufferedChatChoice struct {
	role             string
	content          strings.Builder
	reasoningContent strings.Builder
	toolCalls        map[int]*dto.ToolCallResponse
	finishReason     string
}

type bufferedCompletionChunk struct {
	Id      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Text         string  `json:"text"`
		Index        int     `json:"index"`
		Logprobs     any     `json:"logprobs"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *dto.Usage `json:"usage"`
}

type bufferedCompletionChoice struct {
	Text         string `json:"text"`
	Index        int    `json:"index"`
	Logprobs     any    `json:"logprobs"`
	FinishReason string `json:"finish_reason"`
}

type bufferedCompletionResponse struct {
	Id      string                     `json:"id"`
	Object  string                     `json:"object"`
	Created int64                      `json:"created"`
	Model   string                     `json:"model"`
	Choices []bufferedCompletionChoice `json:"choices"`
	Usage   dto.Usage                  `json:"usage"`
}

type bufferedChatResponse struct {
	Id                string                         `json:"id"`
	Object            string                         `json:"object"`
	Created           int64                          `json:"created"`
	Model             string                         `json:"model"`
	SystemFingerprint *string                        `json:"system_fingerprint,omitempty"`
	Choices           []dto.OpenAITextResponseChoice `json:"choices"`
	Usage             dto.Usage                      `json:"usage"`
}

type bufferedStreamErrorEnvelope struct {
	Error any `json:"error"`
}

func bufferedJSONHTTPResponse(resp *http.Response) *http.Response {
	outputResponse := *resp
	outputResponse.Header = resp.Header.Clone()
	outputResponse.Header.Set("Content-Type", "application/json")
	outputResponse.Header.Del("Cache-Control")
	outputResponse.Header.Del("Connection")
	outputResponse.Header.Del("Content-Encoding")
	outputResponse.Header.Del("Transfer-Encoding")
	return &outputResponse
}

// OaiBufferedStreamHandler consumes an OpenAI Completions SSE response and
// returns the equivalent non-streaming JSON response to the downstream client.
func OaiBufferedStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(nil, types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	defer service.CloseResponseBodyGracefully(resp)

	var watchdog *helper.FirstTokenWatchdog
	if stored, ok := common.GetContextKeyType[*helper.FirstTokenWatchdog](c, constant.ContextKeyFirstTokenWatchdog); ok {
		watchdog = stored
	}
	defer func() {
		if watchdog != nil {
			watchdog.Stop("buffered upstream stream finished")
		}
		common.SetContextKey(c, constant.ContextKeyFirstTokenWatchdog, nil)
	}()

	chatChoices := make(map[int]*bufferedChatChoice)
	completionChoices := make(map[int]*bufferedCompletionChoice)
	var responseId string
	var responseObject string
	var responseModel string
	var systemFingerprint *string
	var created int64
	var usage *dto.Usage
	var responseText strings.Builder
	var lastData string

	scanner := helper.NewStreamScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) < 5 || line[:5] != "data:" {
			continue
		}
		data := strings.TrimSpace(line[5:])
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			break
		}

		if watchdog != nil {
			watchdog.Stop("first token received")
		}
		info.SetFirstResponseTime()
		info.ReceivedResponseCount++
		lastData = data
		var errorEnvelope bufferedStreamErrorEnvelope
		if err := common.UnmarshalJsonStr(data, &errorEnvelope); err == nil {
			if openAIError := dto.GetOpenAIError(errorEnvelope.Error); openAIError != nil && openAIError.Type != "" {
				return nil, types.WithOpenAIError(*openAIError, http.StatusInternalServerError)
			}
		}

		if info.RelayMode == relayconstant.RelayModeCompletions {
			var chunk bufferedCompletionChunk
			if err := common.UnmarshalJsonStr(data, &chunk); err != nil {
				return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
			}
			if chunk.Id != "" {
				responseId = chunk.Id
			}
			if chunk.Object != "" {
				responseObject = chunk.Object
			}
			if chunk.Model != "" {
				responseModel = chunk.Model
			}
			if chunk.Created != 0 {
				created = chunk.Created
			}
			if service.ValidUsage(chunk.Usage) {
				usage = chunk.Usage
			}
			for _, choice := range chunk.Choices {
				accumulator := completionChoices[choice.Index]
				if accumulator == nil {
					accumulator = &bufferedCompletionChoice{Index: choice.Index}
					completionChoices[choice.Index] = accumulator
				}
				accumulator.Text += choice.Text
				responseText.WriteString(choice.Text)
				if choice.Logprobs != nil {
					accumulator.Logprobs = choice.Logprobs
				}
				if choice.FinishReason != nil {
					accumulator.FinishReason = *choice.FinishReason
				}
			}
			continue
		}

		var chunk dto.ChatCompletionsStreamResponse
		if err := common.UnmarshalJsonStr(data, &chunk); err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
		if chunk.Id != "" {
			responseId = chunk.Id
		}
		if chunk.Object != "" {
			responseObject = chunk.Object
		}
		if chunk.Model != "" {
			responseModel = chunk.Model
		}
		if chunk.Created != 0 {
			created = chunk.Created
		}
		if chunk.SystemFingerprint != nil {
			systemFingerprint = chunk.SystemFingerprint
		}
		if service.ValidUsage(chunk.Usage) {
			usage = chunk.Usage
		}
		for _, choice := range chunk.Choices {
			accumulator := chatChoices[choice.Index]
			if accumulator == nil {
				accumulator = &bufferedChatChoice{toolCalls: make(map[int]*dto.ToolCallResponse)}
				chatChoices[choice.Index] = accumulator
			}
			if choice.Delta.Role != "" {
				accumulator.role = choice.Delta.Role
			}
			content := choice.Delta.GetContentString()
			accumulator.content.WriteString(content)
			responseText.WriteString(content)
			reasoningContent := choice.Delta.GetReasoningContent()
			accumulator.reasoningContent.WriteString(reasoningContent)
			responseText.WriteString(reasoningContent)
			for toolPosition, toolCall := range choice.Delta.ToolCalls {
				toolIndex := toolPosition
				if toolCall.Index != nil {
					toolIndex = *toolCall.Index
				}
				accumulatedTool := accumulator.toolCalls[toolIndex]
				if accumulatedTool == nil {
					accumulatedTool = &dto.ToolCallResponse{}
					accumulator.toolCalls[toolIndex] = accumulatedTool
				}
				if toolCall.ID != "" {
					accumulatedTool.ID = toolCall.ID
				}
				if toolCall.Type != nil {
					accumulatedTool.Type = toolCall.Type
				}
				accumulatedTool.Function.Name += toolCall.Function.Name
				accumulatedTool.Function.Arguments += toolCall.Function.Arguments
				responseText.WriteString(toolCall.Function.Name)
				responseText.WriteString(toolCall.Function.Arguments)
			}
			if choice.FinishReason != nil {
				accumulator.finishReason = *choice.FinishReason
			}
		}
	}

	if helper.HasFirstTokenTimeout(c) {
		return nil, helper.FirstTokenLatencyError(info)
	}
	if err := scanner.Err(); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	if responseModel == "" {
		responseModel = info.UpstreamModelName
	}
	estimatedUsage := usage == nil
	if estimatedUsage {
		usage = service.ResponseText2Usage(c, responseText.String(), responseModel, info.GetEstimatePromptTokens())
	}

	var responseBody []byte
	var err error
	if info.RelayMode == relayconstant.RelayModeCompletions {
		indexes := make([]int, 0, len(completionChoices))
		for index := range completionChoices {
			indexes = append(indexes, index)
		}
		sort.Ints(indexes)
		choices := make([]bufferedCompletionChoice, 0, len(indexes))
		for _, index := range indexes {
			choices = append(choices, *completionChoices[index])
		}
		if responseObject == "" || responseObject == "text_completion.chunk" {
			responseObject = "text_completion"
		}
		applyUsagePostProcessing(info, usage, common.StringToByteSlice(lastData))
		responseBody, err = common.Marshal(bufferedCompletionResponse{
			Id: responseId, Object: responseObject, Created: created,
			Model: responseModel, Choices: choices, Usage: *usage,
		})
	} else {
		indexes := make([]int, 0, len(chatChoices))
		for index := range chatChoices {
			indexes = append(indexes, index)
		}
		sort.Ints(indexes)
		choices := make([]dto.OpenAITextResponseChoice, 0, len(indexes))
		toolCount := 0
		for _, index := range indexes {
			accumulator := chatChoices[index]
			if accumulator.finishReason == constant.FinishReasonContentFilter {
				common.SetContextKey(c, constant.ContextKeyAdminRejectReason, "openai_finish_reason=content_filter")
			}
			message := dto.Message{Role: accumulator.role}
			if message.Role == "" {
				message.Role = "assistant"
			}
			toolIndexes := make([]int, 0, len(accumulator.toolCalls))
			for toolIndex := range accumulator.toolCalls {
				toolIndexes = append(toolIndexes, toolIndex)
			}
			sort.Ints(toolIndexes)
			toolCalls := make([]dto.ToolCallResponse, 0, len(toolIndexes))
			for _, toolIndex := range toolIndexes {
				toolCall := *accumulator.toolCalls[toolIndex]
				toolCall.Index = nil
				toolCalls = append(toolCalls, toolCall)
				info.CountBillableToolCall(dto.BuildInCallFunctionCall, toolCall.Function.Name)
			}
			toolCount += len(toolCalls)
			if accumulator.content.Len() > 0 || len(toolCalls) == 0 {
				message.SetStringContent(accumulator.content.String())
			} else {
				message.SetNullContent()
			}
			if accumulator.reasoningContent.Len() > 0 {
				message.ReasoningContent = common.GetPointer(accumulator.reasoningContent.String())
			}
			if len(toolCalls) > 0 {
				message.SetToolCalls(toolCalls)
			}
			choices = append(choices, dto.OpenAITextResponseChoice{
				Index: index, Message: message, FinishReason: accumulator.finishReason,
			})
		}
		if estimatedUsage && toolCount > 0 {
			usage.CompletionTokens += toolCount * 7
			usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
		}
		if responseObject == "" || responseObject == "chat.completion.chunk" {
			responseObject = "chat.completion"
		}
		applyUsagePostProcessing(info, usage, common.StringToByteSlice(lastData))
		responseBody, err = common.Marshal(bufferedChatResponse{
			Id: responseId, Object: responseObject, Created: created,
			Model: responseModel, SystemFingerprint: systemFingerprint,
			Choices: choices, Usage: *usage,
		})
	}
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
	}

	if info.MonitorResponseBody != nil {
		info.MonitorResponseBody.Write(responseBody)
	}
	service.IOCopyBytesGracefully(c, bufferedJSONHTTPResponse(resp), responseBody)
	return usage, nil
}
