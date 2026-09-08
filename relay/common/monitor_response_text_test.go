package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMonitorResponseTextPreservesThinkingBoundaries(t *testing.T) {
	var response MonitorResponseText

	response.WriteThinking("step one")
	response.WriteThinking(" and step two")
	response.WriteContent("final answer")

	assert.Equal(t, "<thinking>\nstep one and step two\n</thinking>\nfinal answer", response.String())
}

func TestMonitorResponseTextClosesUnfinishedThinking(t *testing.T) {
	var response MonitorResponseText

	response.WriteThinking("unfinished stream")

	assert.Equal(t, "<thinking>\nunfinished stream\n</thinking>\n", response.String())
}
