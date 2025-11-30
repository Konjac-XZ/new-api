package monitor

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

// GetRequests returns all stored requests
func GetRequests() gin.HandlerFunc {
	return func(c *gin.Context) {
		log.Printf("[Monitor Handler] GetRequests called: globalStore=%p", globalStore)
		if globalStore == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"success": false,
				"message": "Monitor not initialized",
			})
			return
		}

		records := globalStore.GetAll()
		log.Printf("[Monitor Handler] GetRequests returning %d records", len(records))
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    records,
		})
	}
}

// GetRequest returns a single request by ID
func GetRequest() gin.HandlerFunc {
	return func(c *gin.Context) {
		log.Printf("[Monitor Handler] GetRequest called: globalStore=%p", globalStore)
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
		log.Printf("[Monitor Handler] GetStats called: globalStore=%p", globalStore)
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
		log.Printf("[Monitor Handler] WebSocketHandler called: globalHub=%p, globalStore=%p", globalHub, globalStore)
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
		log.Printf("[Monitor Handler] GetActiveRequests called: globalStore=%p", globalStore)
		if globalStore == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"success": false,
				"message": "Monitor not initialized",
			})
			return
		}

		records := globalStore.GetActive()
		log.Printf("[Monitor Handler] GetActiveRequests returning %d records", len(records))
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    records,
		})
	}
}
