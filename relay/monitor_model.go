package relay

import (
	"strings"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/monitor"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

func recordMonitorUpstreamModel(c *gin.Context, info *relaycommon.RelayInfo) {
	monitorID := c.GetString("monitor_id")
	if monitorID == "" || info == nil {
		return
	}

	upstreamModel := strings.TrimSpace(info.UpstreamModelName)
	if requestModel := strings.TrimSpace(c.GetString("request_model")); info.ChannelType == constant.ChannelTypeVertexAi && requestModel != "" {
		upstreamModel = requestModel
	}
	if upstreamModel == "" {
		return
	}

	originModel := strings.TrimSpace(c.GetString("original_model"))
	if originModel == "" {
		originModel = strings.TrimSpace(info.OriginModelName)
	}
	isRedirected := info.IsModelMapped || (originModel != "" && upstreamModel != originModel)
	monitor.UpdateModelMapping(monitorID, upstreamModel, isRedirected)
}
