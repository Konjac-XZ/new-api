package router

import (
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/monitor"

	"github.com/gin-gonic/gin"
)

func SetMonitorRouter(router *gin.Engine) {
	// REST API endpoints with full AdminAuth (requires New-Api-User header)
	monitorRouter := router.Group("/api/monitor")
	monitorRouter.Use(middleware.AdminAuth())
	{
		monitorRouter.GET("/requests", monitor.GetRequests())
		monitorRouter.GET("/requests/active", monitor.GetActiveRequests())
		monitorRouter.GET("/requests/:id", monitor.GetRequest())
		monitorRouter.GET("/requests/:id/body/:type", monitor.GetRequestBody())
		monitorRouter.GET("/stats", monitor.GetStats())
		monitorRouter.POST("/requests/:id/interrupt", monitor.InterruptRequest())
	}

	// WebSocket endpoint on separate group with session-only auth
	// (browsers cannot set custom headers for WebSocket connections)
	wsRouter := router.Group("/api/monitor")
	wsRouter.GET("/ws", middleware.AdminAuthForWebSocket(), monitor.WebSocketHandler())
}
