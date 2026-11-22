# Axiom Flow - Decisiones Arquitectónicas

Sistema de procesamiento de órdenes con transacciones distribuidas basadas en **Saga Pattern**. Documenta decisiones de diseño clave y estructura del sistema.

## 1. Arquitectura Base

### Patrón Saga (Orquestado)
- Orquestador central en `internal/service/orchestrator.go`
- Transacciones distribuidas con compensación en orden inverso
- Estados registrados en tabla `saga_steps` para recuperación
- Sin dependencia de una sola base de datos; uso de PostgreSQL por consistencia transaccional y queries de recovery conocidas

## 2. Arquitectura Monolito con Extracción de Servicios

### Estructura actual de servicios containerizados
- `services/inventory/` - Directorio de servicio de inventario con propio Dockerfile y migraciones
- `services/shipment/` - Directorio de servicio de envío con propia configuración
- Cada directorio puede compilarse e ejecutarse como contenedor independiente
- Orquestación local mediante `docker-compose.yml` para desarrollo/testing

**Decisión:** Mantener wiring principal en `cmd/server/main.go` mientras los servicios individuales son extractables para containerización. Trade-off: simplicidad en deployment inicial vs. pureza de microservicios puros. El sistema opera como monolito con módulos containerizados que pueden extraerse independientemente cuando sea necesario.

### Justificación de la estructura actual
- Permite testing isolado de componentes sin afectar todo el sistema
- Facilita deployment escalado cuando el crecimiento lo requiera
- Mantiene coherencia transaccional mediante el orchestrator central
- Reduce complejidad operativa en fases iniciales del proyecto

## 3. Circuit Breaker (Implementación Propia)

### `internal/service/circuit_breaker.go`
- Lógica Open/Close/Half-Open controlando tiempo de espera
- **Closed:** Llamadas normales al servicio remoto
- **Open:** Después de N fallos consecutivos, bloquea llamadas y retorna error inmediatamente
- **Half-Open:** Permite pruebas limitadas para verificar recuperación

**Justificación:** Necesitábamos control fin sobre timeouts y comportamiento específico del negocio. Librerías externas no ofrecían el fallback automático a implementación local que requería el patrón Saga.

## 4. Protecciones Transaccionales y de Consistencia

### Transacciones atómicas por servicio
- `services/inventory/main.go` — Transacción `BEGIN`/`COMMIT` en `POST /inventory/reserve` y `POST /inventory/release`. `SELECT FOR UPDATE` bloquea fila por `idempotency_key` y por `(order_id, product_id)` con `status='RESERVED'` (bloqueo pesimista).
- `services/shipment/main.go` — Transacción `BEGIN`/`COMMIT` en `POST /shipments`. `SELECT FOR UPDATE` verifica existencia previa por `idempotency_key`.
- Cada transacción escrita atómicamente junto con su evento en tabla `outbox` (event sourcing outbox pattern).

### Idempotencia por servicio
- `inventory_reservations.idempotency_key` con constraint `UNIQUE`. Rechaza reservas duplicadas con misma clave.
- `shipments.idempotency_key` con constraint `UNIQUE`. Rechaza envíos duplicados con misma clave.
- `orders.idempotency_key` (monolito) con constraint `UNIQUE`.

### Outbox
- `services/inventory/` y `services/shipment/` tienen tabla `outbox` con columnas: `aggregate_type`, `aggregate_id`, `event_type`, `payload JSONB`, `status` (`PENDING`/`published_at`).
- Eventos escritos dentro de la misma transacción del negocio (`inventory.reserved`, `inventory.released`, `shipment.created`).


## 5. Rate Limiting y Idempotencia (desde el diseño)

### Rate Limiting
- `internal/handler/ratelimit/` - Limitador en-memory configurable
- Rango: de 5 req/s a 60 req/min por IP
- Responde 429 antes de que el sistema se colapse
- **Nota:** Arquitectura preparada para migrar a Redis cuando se multipliquen instancias

### Idempotencia
- Cada orden incluye `idempotency_key` (string único)
- Almacenado en tabla `orders` con constraint unique
- Previne procesamiento duplicado si circuit breaker retries llamadas
- Crítico para consistencia cuando fallos provocan retries automáticas

## Estructura Actual

```text
axiom-flow/
├── cmd/server/main.go           # Entry point y wiring de servicios
├── internal/service/           # Lógica de negocio y adaptadores
│   ├── circuit_breaker.go      # Circuit breaker propio
│   ├── shipment_remote.go      # Adaptador HTTP shipment
│   └── inventory_remote.go     # Adaptador HTTP inventory
├── services/                   # Servicios containerizados extraídos (autónomos, cada uno con su PostgreSQL)
│   ├── inventory/main.go       # Reservas con transacción atómica, SELECT FOR UPDATE, idempotency key, outbox
│   ├── inventory/migrations/001_initial.sql  # Migración independiente (reservas + outbox)
│   ├── shipment/main.go        # Envíos con transacción atómica, idempotency key, outbox
│   └── payment/main.go         # Pagos con HTTP API (PostgreSQL, idempotente)
├── migrations/                 # Migraciones SQL
└── docker-compose.yml          # Orquestación de contenedores
