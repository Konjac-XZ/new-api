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
	RegisterChanSize   = 64
	UnregisterChanSize = 64

	// Store limits
	MonitorMinRecords          = 100
	MonitorRetentionWindow     = 5 * time.Minute
	MonitorStatsCacheTTL       = time.Second
	MonitorDegradeActiveLimit  = MonitorMinRecords
	MonitorRecoverActiveLimit  = MonitorMinRecords * 8 / 10
	MonitorBodyCaptureMaxBytes = 40 * 1024
)
