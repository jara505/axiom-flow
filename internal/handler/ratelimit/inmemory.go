package ratelimit

import (
	"sync"
	"time"
)

// bucketWindow almacena el estado de una clave dentro de una ventana de tiempo fija (Fixed Window).
type bucketWindow struct {
	count     int
	windowEnd time.Time
}

// InMemoryRateLimiter es una implementación en memoria del RateLimiter.
// No depende de Redis ni de ninguna base de datos externa.
// Incluye limpieza periódica para evitar fugas de memoria por claves expiradas.
type InMemoryRateLimiter struct {
	mu           sync.RWMutex
	buckets      map[string]*bucketWindow
	limit        int
	windowDuration time.Duration
}

// NewInMemoryRateLimiter crea un limitador con la cantidad máxima de peticiones
// y la duración de la ventana. Inicia la limpieza periódica para evitar memory leaks.
func NewInMemoryRateLimiter(limit int, windowDuration time.Duration) *InMemoryRateLimiter {
	rl := &InMemoryRateLimiter{
		buckets:      make(map[string]*bucketWindow),
		limit:        limit,
		windowDuration: windowDuration,
	}

	// Limpieza programada: elimina ventanas expiradas cada 5 minutos.
	// Esto evita que el mapa crezca sin control por cada IP nueva que llega.
	go rl.cleanup(5 * time.Minute)

	return rl
}

// Allow implementa RateLimiter.Allow usando algoritmo Fixed Window.
func (rl *InMemoryRateLimiter) Allow(key string) (allowed bool, remaining int, resetAt time.Time, retryAfter time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()

	w, exists := rl.buckets[key]
	if !exists || !now.Before(w.windowEnd) {
		// Nueva ventana o primera vez para esta clave
		w = &bucketWindow{
			count:     1,
			windowEnd: now.Add(rl.windowDuration),
		}
		rl.buckets[key] = w
		return true, rl.limit - 1, w.windowEnd, 0
	}

	if w.count >= rl.limit {
		// Límite excedido dentro de la ventana vigente
		return false, 0, w.windowEnd, w.windowEnd.Sub(now)
	}

	w.count++
	return true, rl.limit - w.count, w.windowEnd, 0
}

// Limit implementa RateLimiter.Limit.
func (rl *InMemoryRateLimiter) Limit() int {
	rl.mu.RLock()
	defer rl.mu.RUnlock()
	return rl.limit
}

// cleanup borra periódicamente las claves cuya ventana ya expiró.
// Sin esto, cada IP única dejaría un registro para siempre (memory leak).
func (rl *InMemoryRateLimiter) cleanup(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for key, w := range rl.buckets {
			if now.After(w.windowEnd) {
				delete(rl.buckets, key)
			}
		}
		rl.mu.Unlock()
	}
}
