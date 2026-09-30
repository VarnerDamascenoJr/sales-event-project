package httpapi

import "github.com/gin-gonic/gin"

func publicRateLimited(deps RouterDeps, handler gin.HandlerFunc) gin.HandlerFunc {
	if deps.PublicRateLimiter == nil {
		return handler
	}
	return func(c *gin.Context) {
		deps.PublicRateLimiter.Middleware()(c)
		if c.IsAborted() {
			return
		}
		handler(c)
	}
}
