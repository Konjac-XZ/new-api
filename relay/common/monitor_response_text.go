package common

import "strings"

// MonitorResponseText preserves the boundary between reasoning and visible
// content while a streamed response is being assembled for Monitor.
type MonitorResponseText struct {
	body         strings.Builder
	thinkingOpen bool
}

func (text *MonitorResponseText) WriteThinking(value string) {
	if value == "" {
		return
	}
	if !text.thinkingOpen {
		text.body.WriteString("<thinking>\n")
		text.thinkingOpen = true
	}
	text.body.WriteString(value)
}

func (text *MonitorResponseText) WriteContent(value string) {
	if value == "" {
		return
	}
	text.closeThinking()
	text.body.WriteString(value)
}

func (text *MonitorResponseText) String() string {
	if !text.thinkingOpen {
		return text.body.String()
	}
	return text.body.String() + "\n</thinking>\n"
}

func (text *MonitorResponseText) closeThinking() {
	if !text.thinkingOpen {
		return
	}
	text.body.WriteString("\n</thinking>\n")
	text.thinkingOpen = false
}
