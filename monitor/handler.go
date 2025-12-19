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

		records := globalStore.GetAllSnapshot()
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    records,
		})
	}
}

// GetRequest returns a single request by ID
// For bodies exceeding 20KB, the body content is excluded to save bandwidth
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
		record := globalStore.GetSnapshot(id)
		if record == nil {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"message": "Request not found",
			})
			return
		}

		// Exclude body content if it exceeds 20KB to save bandwidth
		// Frontend will check body_size and fetch body separately if needed
		const displayThreshold = 20000

		if record.Downstream.BodySize > displayThreshold {
			record.Downstream.Body = ""
		}
		if record.Upstream != nil && record.Upstream.BodySize > displayThreshold {
			record.Upstream.Body = ""
		}
		if record.Response != nil && record.Response.BodySize > displayThreshold {
			record.Response.Body = ""
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    record,
		})
	}
}

// GetRequestBody returns the body content for a specific request
// bodyType can be: "downstream", "upstream", or "response"
func GetRequestBody() gin.HandlerFunc {
	return func(c *gin.Context) {
		if globalStore == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"success": false,
				"message": "Monitor not initialized",
			})
			return
		}

		id := c.Param("id")
		bodyType := c.Param("type")

		record := globalStore.GetSnapshot(id)
		if record == nil {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"message": "Request not found",
			})
			return
		}

		var body string
		var bodySize int

		switch bodyType {
		case "downstream":
			body = record.Downstream.Body
			bodySize = record.Downstream.BodySize
		case "upstream":
			if record.Upstream != nil {
				body = record.Upstream.Body
				bodySize = record.Upstream.BodySize
			}
		case "response":
			if record.Response != nil {
				body = record.Response.Body
				bodySize = record.Response.BodySize
			}
		default:
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "Invalid body type. Must be: downstream, upstream, or response",
			})
			return
		}

		if bodySize == 0 {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"message": "Body not available",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data": gin.H{
				"body":      body,
				"body_size": bodySize,
			},
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

		records := globalStore.GetActiveSnapshot()
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    records,
		})
	}
}

// InterruptRequest cancels an ongoing request attempt
func InterruptRequest() gin.HandlerFunc {
	return func(c *gin.Context) {
		if globalStore == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"success": false,
				"message": "Monitor not initialized",
			})
			return
		}

		id := c.Param("id")
		if id == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "Request ID is required",
			})
			return
		}

		// Check if request exists and is active
		record := globalStore.GetSnapshot(id)
		if record == nil {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"message": "Request not found",
			})
			return
		}

		// Check if request is in an active state
		if !isActiveStatus(record.Status) {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "Request is not active (already completed or failed)",
			})
			return
		}

		// Attempt to cancel the request
		registry := GetRegistry()
		cancelled := registry.CancelRequest(id)

		if !cancelled {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"message": "No active cancellable operation found for this request",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "Request interrupted successfully",
		})
	}
}
