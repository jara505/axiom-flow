package ratelimit

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type rateLimitResponse struct {
	Error         string    `json:"error"`
	Message       string    `json:"message"`
	Limit         int       `json:"limit"`
	Remaining     int       `json:"remaining"`
	ResetAt       time.Time `json:"reset_at"`
	RetryAfterSec int       `json:"retry_after_seconds"`
}

// RateLimitMiddleware envuelve un handler de Gin y aplica rate limiting por IP.
// No sabe si el RateLimiter es in-memory, Redis o cualquier otra cosa.
func RateLimitMiddleware(rl RateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.ClientIP() // clave simple: IP del cliente

		allowed, remaining, resetAt, retryAfter := rl.Allow(key)

		c.Header("X-RateLimit-Limit", strconv.Itoa(rl.Limit()))
		c.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))
		c.Header("X-RateLimit-Reset", strconv.FormatInt(resetAt.Unix(), 10))

		if !allowed {
			c.Header("Retry-After", strconv.Itoa(int(retryAfter.Seconds())))
			c.Header("Content-Type", "application/json")
			c.AbortWithStatusJSON(http.StatusTooManyRequests, rateLimitResponse{
				Error:         "rate_limit_exceeded",
				Message:       "demasiadas peticiones, intenta de nuevo más tarde",
				Limit:         rl.Limit(),
				Remaining:     0,
				ResetAt:       resetAt,
				RetryAfterSec: int(retryAfter.Seconds()),
			})
			return
		}

		c.Next()
	}
}
