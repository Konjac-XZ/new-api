package monitor

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// GetRequests returns all stored requests
func GetRequests() gin.HandlerFunc {
	return func(c *gin.Context) {
		records := GetManager().GetStore().GetAll()
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    records,
		})
	}
}

// GetRequest returns a single request by ID
func GetRequest() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		record := GetManager().GetStore().Get(id)
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
		stats := GetManager().GetStore().GetStats()

		connections := 0
		if GetManager().GetHub() != nil {
			connections = GetManager().GetHub().ClientCount()
		}

		c.JSON(http.StatusOK, gin.H{
			"success":     true,
			"data":        stats,
			"connections": connections,
		})
	}
}

// WebSocketHandler handles WebSocket connections for real-time updates
func WebSocketHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		GetManager().GetHub().ServeWs(c, GetManager().GetStore())
	}
}

// GetActiveRequests returns only currently processing requests
func GetActiveRequests() gin.HandlerFunc {
	return func(c *gin.Context) {
		records := GetManager().GetStore().GetActive()
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    records,
		})
	}
}

// GetRequestBody returns stored body content for a request (downstream, upstream, response)
func GetRequestBody() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		bodyType := strings.ToLower(c.Param("type"))

		record := GetManager().GetStore().Get(id)
		if record == nil {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Request not found"})
			return
		}

		switch bodyType {
		case "downstream":
			c.JSON(http.StatusOK, gin.H{
				"success":   true,
				"type":      bodyType,
				"body":      record.Downstream.Body,
				"size":      record.Downstream.BodySize,
				"truncated": record.Downstream.BodyTruncated,
			})
		case "upstream":
			if record.Upstream == nil {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Upstream body not recorded"})
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"success":   true,
				"type":      bodyType,
				"body":      record.Upstream.Body,
				"size":      record.Upstream.BodySize,
				"truncated": record.Upstream.BodyTruncated,
			})
		case "response":
			if record.Response == nil {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Response body not recorded"})
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"success":   true,
				"type":      bodyType,
				"body":      record.Response.Body,
				"size":      record.Response.BodySize,
				"truncated": record.Response.BodyTruncated,
			})
		default:
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid body type"})
		}
	}
}

// InterruptRequest cancels an in-flight request if a cancel func is registered
func InterruptRequest() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if id == "" {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Missing request id"})
			return
		}

		registry := GetRegistry()
		if registry == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "Cancellation registry not available"})
			return
		}

		if registry.CancelRequest(id) {
			c.JSON(http.StatusOK, gin.H{"success": true, "message": "Request interrupted"})
			return
		}

		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Request not cancellable or not found"})
	}
}
