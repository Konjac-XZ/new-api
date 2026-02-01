package monitor

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// RequireInitialized is a middleware that ensures the monitor system is initialized
func RequireInitialized() gin.HandlerFunc {
	return func(c *gin.Context) {
		if GetManager().GetStore() == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"success": false,
				"message": "Monitor not initialized",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}
