package ratelimit

import "time"

// RateLimiter define el contrato para cualquier estrategia de rate limiting.
// Permite cambiar de in-memory a Redis (u otro backend) sin modificar el middleware
// ni los handlers HTTP. Solo se inyecta una implementación distinta en el servidor.
type RateLimiter interface {
	// Allow verifica si la clave puede ejecutar la petición.
	// allowed: true si pasa el límite
	// remaining: cuántas peticiones le quedan en la ventana actual
	// resetAt: momento en que se reinicia el contador
	// retryAfter: cuánto debe esperar el cliente si fue rechazado (0 si fue permitido)
	Allow(key string) (allowed bool, remaining int, resetAt time.Time, retryAfter time.Duration)

	// Limit devuelve el máximo de peticiones por ventana (útil para headers de respuesta)
	Limit() int
}
