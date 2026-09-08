package relay

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestForceStreamRequestBodyPreservesPassthroughNumbers(t *testing.T) {
	input := []byte(`{"model":"gpt-test","stream":false,"max_tokens":18446744073686646784,"extra":{"enabled":true}}`)

	output, err := forceStreamRequestBody(input, &dto.StreamOptions{IncludeUsage: true})
	require.NoError(t, err)

	var body map[string]json.RawMessage
	require.NoError(t, common.Unmarshal(output, &body))
	assert.JSONEq(t, `true`, string(body["stream"]))
	assert.JSONEq(t, `{"include_usage":true}`, string(body["stream_options"]))
	assert.Equal(t, "18446744073686646784", string(body["max_tokens"]))
	assert.JSONEq(t, `{"enabled":true}`, string(body["extra"]))
}
