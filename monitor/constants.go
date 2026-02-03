package monitor

import "time"

const (
	// WebSocket timeouts
	WriteWait  = 10 * time.Second
	PongWait   = 60 * time.Second
	PingPeriod = (PongWait * 9) / 10

	// WebSocket message size
	MaxMessageSize = 512

	// Channel buffer sizes
	BroadcastChanSize  = 256
	ClientSendChanSize = 256

	// Store limits
	MaxRecords  = 100
	MaxBodySize = 10 * 1024
)
