package httpapi

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
)

const paymentSignatureHeader = "X-Webhook-Signature" // #nosec G101 -- header name, not a secret value.

func requireBodyHMACSignature(secret string, headerName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if secret == "" {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "webhook secret is not configured"})
			c.Abort()
			return
		}

		body, err := c.GetRawData()
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "could not read webhook body"})
			c.Abort()
			return
		}
		c.Request.Body = io.NopCloser(bytes.NewReader(body))

		expected := computeBodyHMAC(body, secret)
		if !constantTimeStringEqual(c.GetHeader(headerName), expected) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "webhook signature is invalid"})
			c.Abort()
			return
		}

		c.Next()
	}
}

func computeBodyHMAC(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}
