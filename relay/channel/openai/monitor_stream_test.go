package openai

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppendMonitorTokenDataPreservesReasoningBoundary(t *testing.T) {
	var response relaycommon.MonitorResponseText

	err := appendMonitorTokenData(constant.RelayModeChatCompletions, `{"choices":[{"delta":{"reasoning_content":"first step"}}]}`, &response)
	require.NoError(t, err)
	err = appendMonitorTokenData(constant.RelayModeChatCompletions, `{"choices":[{"delta":{"reasoning_content":"; second step"}}]}`, &response)
	require.NoError(t, err)
	err = appendMonitorTokenData(constant.RelayModeChatCompletions, `{"choices":[{"delta":{"content":"final answer"}}]}`, &response)
	require.NoError(t, err)

	assert.Equal(t, "<thinking>\nfirst step; second step\n</thinking>\nfinal answer", response.String())
}

func TestAppendResponsesMonitorEventPreservesReasoningBoundary(t *testing.T) {
	var response relaycommon.MonitorResponseText

	appendResponsesMonitorEvent(&dto.ResponsesStreamResponse{
		Type:  "response.reasoning_summary_text.delta",
		Delta: "summary",
	}, &response)
	appendResponsesMonitorEvent(&dto.ResponsesStreamResponse{
		Type:  "response.output_text.delta",
		Delta: "answer",
	}, &response)

	assert.Equal(t, "<thinking>\nsummary\n</thinking>\nanswer", response.String())
}
