package httpapi

import (
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

type RateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*visitorLimiter
	rps      rate.Limit
	burst    int
	ttl      time.Duration
}

type visitorLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

func NewRateLimiter(requestsPerSecond float64, burst int, ttl time.Duration) *RateLimiter {
	if requestsPerSecond <= 0 {
		requestsPerSecond = 1
	}
	if burst <= 0 {
		burst = 1
	}
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}

	return &RateLimiter{
		limiters: make(map[string]*visitorLimiter),
		rps:      rate.Limit(requestsPerSecond),
		burst:    burst,
		ttl:      ttl,
	}
}

func (l *RateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !l.allow(c.ClientIP()) {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "rate limit exceeded"})
			c.Abort()
			return
		}
		c.Next()
	}
}

func (l *RateLimiter) allow(clientIP string) bool {
	now := time.Now()

	l.mu.Lock()
	defer l.mu.Unlock()

	l.prune(now)

	key := normalizedIP(clientIP)
	visitor, ok := l.limiters[key]
	if !ok {
		visitor = &visitorLimiter{
			limiter:  rate.NewLimiter(l.rps, l.burst),
			lastSeen: now,
		}
		l.limiters[key] = visitor
	}

	visitor.lastSeen = now
	return visitor.limiter.Allow()
}

func (l *RateLimiter) prune(now time.Time) {
	for key, visitor := range l.limiters {
		if now.Sub(visitor.lastSeen) > l.ttl {
			delete(l.limiters, key)
		}
	}
}

func normalizedIP(clientIP string) string {
	if ip := net.ParseIP(clientIP); ip != nil {
		return ip.String()
	}
	return clientIP
}
