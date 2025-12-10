package monitor

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// GetRequests returns all stored requests
func GetRequests() gin.HandlerFunc {
	return func(c *gin.Context) {
		if globalStore == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"success": false,
				"message": "Monitor not initialized",
			})
			return
		}

		records := globalStore.GetAll()
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    records,
		})
	}
}

// GetRequest returns a single request by ID
func GetRequest() gin.HandlerFunc {
	return func(c *gin.Context) {
		if globalStore == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"success": false,
				"message": "Monitor not initialized",
			})
			return
		}

		id := c.Param("id")
		record := globalStore.Get(id)
		if record == nil {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"message": "Request not found",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    record,
		})
	}
}

// GetStats returns monitoring statistics
func GetStats() gin.HandlerFunc {
	return func(c *gin.Context) {
		if globalStore == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"success": false,
				"message": "Monitor not initialized",
			})
			return
		}

		stats := globalStore.GetStats()
		stats.TotalRequests = globalStore.count

		c.JSON(http.StatusOK, gin.H{
			"success":     true,
			"data":        stats,
			"connections": globalHub.ClientCount(),
		})
	}
}

// WebSocketHandler handles WebSocket connections for real-time updates
func WebSocketHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		if globalHub == nil || globalStore == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"success": false,
				"message": "Monitor not initialized",
			})
			return
		}

		globalHub.ServeWs(c, globalStore)
	}
}

// GetActiveRequests returns only currently processing requests
func GetActiveRequests() gin.HandlerFunc {
	return func(c *gin.Context) {
		if globalStore == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"success": false,
				"message": "Monitor not initialized",
			})
			return
		}

		records := globalStore.GetActive()
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    records,
		})
	}
}
