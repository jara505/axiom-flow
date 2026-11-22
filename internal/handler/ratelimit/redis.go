package ratelimit

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisRateLimiter es una implementación de RateLimiter usando Redis.
// Usa un contador fijo por ventana con claves que expir automáticamente.
type RedisRateLimiter struct {
	client         *redis.Client
	limit          int
	keyPrefix      string
	windowDuration time.Duration
}

// NewRedisRateLimiter crea un limitador que usa Redis como backend.
func NewRedisRateLimiter(client *redis.Client, limit int, windowDuration time.Duration, keyPrefix string) *RedisRateLimiter {
	return &RedisRateLimiter{
		client:         client,
		limit:          limit,
		keyPrefix:      keyPrefix,
		windowDuration: windowDuration,
	}
}

// Allow implementa RateLimiter.Allow usando algoritmo Fixed Window con Redis.
func (r *RedisRateLimiter) Allow(key string) (allowed bool, remaining int, resetAt time.Time, retryAfter time.Duration) {
	ctx := context.Background()

	// Construir la clave de Redis: prefijo:ip
	redisKey := r.keyPrefix + ":" + key

	// INCR atomically incrementa el contador
	newCount, err := r.client.Incr(ctx, redisKey).Result()
	if err != nil {
		// Si falla Redis, permitimos el request por seguridad (fail-open)
		return true, r.limit, time.Now().Add(r.windowDuration), 0
	}

	// Primera petición de esta clave: establecer expiración si aún no existe
	if newCount == 1 {
		// SET WITH EX para crear la clave con TTL de una sola operación
		err = r.client.Expire(ctx, redisKey, r.windowDuration).Err()
		if err != nil {
			// Si falla el Expire, comunque retornamos el count incrementado
		}
	}

	limitInt64 := int64(r.limit)

	// Calcular remaining y resetAt
	if newCount > limitInt64 {
		// Límite excedido
		remaining = 0
		// Obtener el tiempo restante hasta que la clave expire
		ttl, err := r.client.TTL(ctx, redisKey).Result()
		if err == nil && ttl > 0 {
			resetAt = time.Now().Add(ttl)
			retryAfter = ttl
		} else {
			// Fallback: ventana por defecte
			resetAt = time.Now().Add(r.windowDuration)
			retryAfter = r.windowDuration
		}
		allowed = false
	} else {
		allowed = true
		remaining = r.limit - int(newCount)
		// resetAt es el final de la ventana actual
		ttl, _ := r.client.TTL(ctx, redisKey).Result()
		if ttl < 0 {
			ttl = r.windowDuration
		}
		resetAt = time.Now().Add(ttl)
	}

	return allowed, remaining, resetAt, retryAfter
}

// Limit implementa RateLimiter.Limit.
func (r *RedisRateLimiter) Limit() int {
	return r.limit
}
