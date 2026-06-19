package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConditionalMonitorResponseBodyDropsCapturedTextWhenDisabled(t *testing.T) {
	enabled := true
	body := NewConditionalMonitorResponseBody(func() bool {
		return enabled
	})

	n, err := body.WriteString("streamed text")
	require.NoError(t, err)
	assert.Equal(t, len("streamed text"), n)
	assert.Equal(t, "streamed text", body.String())

	enabled = false
	n, err = body.WriteString("more text")
	require.NoError(t, err)
	assert.Equal(t, len("more text"), n)
	assert.Zero(t, body.Len())
	assert.Empty(t, body.String())

	enabled = true
	n, err = body.Write([]byte("after recovery"))
	require.NoError(t, err)
	assert.Equal(t, len("after recovery"), n)
	assert.Equal(t, "after recovery", body.String())
}
