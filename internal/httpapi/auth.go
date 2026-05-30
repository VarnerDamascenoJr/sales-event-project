package httpapi

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

const apiKeyHeader = "X-API-Key" // #nosec G101 -- header name, not a secret value.

const (
	RoleAdmin           = "ADMIN"
	RoleSupport         = "SUPPORT"
	RoleCheckIn         = "CHECK_IN"
	RolePaymentProvider = "PAYMENT_PROVIDER"
)

type AuthStore interface {
	AuthenticateAPIKey(ctx context.Context, key string) (APIKeyPrincipal, error)
}

type APIKeyPrincipal struct {
	ID   string
	Name string
	Role string
}

func requireRoles(authStore AuthStore, roles ...string) gin.HandlerFunc {
	allowedRoles := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		allowedRoles[role] = struct{}{}
	}

	return func(c *gin.Context) {
		if authStore == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "auth store is not configured"})
			c.Abort()
			return
		}

		apiKey := c.GetHeader(apiKeyHeader)
		if apiKey == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "api key is required"})
			c.Abort()
			return
		}

		principal, err := authStore.AuthenticateAPIKey(c.Request.Context(), apiKey)
		if err != nil {
			status := http.StatusUnauthorized
			if !errors.Is(err, errInvalidAPIKey) {
				status = http.StatusInternalServerError
			}
			c.JSON(status, gin.H{"error": err.Error()})
			c.Abort()
			return
		}

		if _, ok := allowedRoles[principal.Role]; !ok {
			c.JSON(http.StatusForbidden, gin.H{"error": "api key role is not allowed"})
			c.Abort()
			return
		}

		c.Set("apiKeyId", principal.ID)
		c.Set("apiKeyRole", principal.Role)
		c.Next()
	}
}

func hashAPIKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

func constantTimeStringEqual(a string, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

var errInvalidAPIKey = errors.New("api key is invalid")
