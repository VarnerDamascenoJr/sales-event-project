package httpapi

import "net/http"

import "github.com/gin-gonic/gin"

const webhookSecretHeader = "X-Webhook-Secret" // #nosec G101 -- header name, not a secret value.

func requireWebhookSecret(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if secret == "" {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "webhook secret is not configured"})
			c.Abort()
			return
		}

		if !constantTimeStringEqual(c.GetHeader(webhookSecretHeader), secret) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "webhook secret is invalid"})
			c.Abort()
			return
		}

		c.Next()
	}
}
