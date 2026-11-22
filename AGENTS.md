# AGENTS.md

Contexto general del proyecto **Axiom Flow**: sistema de procesamiento de pedidos con transacciones distribuidas basadas en el **patrón Saga**. Incluye creación de pedidos, reserva de inventario, procesamiento de pagos y envío, con compensación en orden inverso ante fallos. Implementado en **Go** con **PostgreSQL**.

Cada documento de `./axiom_context` tiene un propósito específico. Esta tabla sirve como índice rápido para evitar lecturas innecesarias y ahorrar tokens.

| Documento | Propósito |
|-----------|-----------|
| `00-indice.md` | Índice del proyecto, orden de lectura y mapa fallo → propuesta → justificación. |
| `01-contexto.md` | Contexto del negocio, definición del problema y lista de fallos posibles. |
| `02-alcance.md` | Alcance del proyecto: requisitos funcionales y no funcionales. |
| `03-dominio.md` | Modelo de dominio: entidades SQL, atributos, restricciones y contratos TypeScript. |
| `04-casos-de-uso.md` | Escenarios de uso y de fallo, contratos de componentes y diagrama de flujo de la Saga. |
| `05-propuestas.md` | Soluciones propuestas por fallo, trade-offs y fallbacks. Incluye bloqueo pesimista e idempotencia. |
| `06-justificacion.md` | Justificación de las soluciones elegidas y alternativas descartadas. |
| `07-arquitectura.md` | Guía metodológica de preguntas para definir la arquitectura (no contiene respuestas definitivas). |
| `08-guion-demo.md` | Guion de demostración con cinco escenarios ejecutables de la Saga. |
| `09-stack-tecnologico.md` | Stack tecnológico propuesto: Go, PostgreSQL, Gin, pgx, sqlx, uuid, testify, etc. |
| `10-alineamiento.md` | Alineamiento arquitectura → implementación (operacional, estructural y de restricción). |
| `11-transacciones.md` | Dudas, problemas abiertos y decisiones pendientes a nivel de transacciones, niveles de aislamiento, bloqueo pesimista y compensación en SAGA. |

> **Nota:** Los documentos futuros se agregan aquí cuando se creen.

---
## 0. Precedence

When rules conflict: Correctness & Safety > Context > Authorization > Execution > Output.

## 1. Context First

Before any change:
- Read the entry point of the task. Expand only to direct dependencies. Stop at low marginal relevance.
- List files read and justify relevance.
- Check environment/config files: `.env.example`, `package.json`, `pyproject.toml`, `go.mod`, `Cargo.toml`, `Makefile`, CI configs.
- Identify affected modules and shared dependencies.

If the task is ambiguous in a way that blocks correctness, ask. Do not guess.

## 2. Authorization

No implicit permission to mutate state. Require explicit approval before:
- File system changes (create, edit, delete)
- Git operations (branch, commit, push, reset, rebase)
- Script execution
- External service interaction

Flow: analyze → propose plan → wait for "Approved" / "Proceed".

If a `git-workflow` skill is available, defer to it for branch, commit, and PR rules.

## 3. Dry Run (visible)

Before modifying, state in one sentence: what changes, why, and what it may affect.
No fixed template. Do not restate obvious steps.

## 4. Code

- Minimal diffs; avoid full rewrites unless explicitly requested.
- Comments only for non-obvious logic, workarounds, or perf optimizations.
- Explicit error handling. No silent failures. No empty catch blocks.
- SOLID, small focused functions, descriptive naming, no global state.
- Match the existing style of the file you are editing. Do not reformat unrelated code.

## 5. Testing

- TDD required for bug fixes, business logic, and non-trivial features. Optional otherwise.
- Tests must fail before the fix and pass after. Do not write tests that pass trivially.
- Update docs when logic changes.
- Warn explicitly on breaking changes affecting env vars, DB schemas, external APIs, or public interfaces.

## 6. Uncertainty

If confidence is insufficient: ask, or state "verification required". Do not speculate.
Do not invent APIs, file paths, config keys, or behavior. If unsure, read the code.

## 7. Output

- Result first, explanation only if non-obvious.
- No filler, no repetition, no restating the input.
- Do not expose step-by-step reasoning. Show conclusions and critical insights only.
- Compress similar ideas. Output only new information.

## 8. Reading & Search

Prefer searching to reading. Before reading a file, estimate its size
(`wc -l` or first 50 lines):
- < 200 lines → read fully.
- 200–1000 lines → grep for the relevant symbol, then read around it.
- > 1000 lines → never read fully. Extract only the relevant slice
  (`rg -n`, `sed -n 'A,Bp'`). State which lines you read.

Use search tools before reads: `rg -n`, `git grep -n`, `fd`/`find`,
`git log --oneline -20 -- path/`. Report what you searched and found —
do not silently grep and act on the result.

If the agent tool exposes ranged reads (offset/limit) or a repo map,
use them. Never paste an entire large file into context; summarize
first if you must, and ask for confirmation.



---

## Skills disponibles

| Skill | Propósito | Requisito |
|-------|-----------|-----------|
| `git-workflow` | Gestión de ramas, commits y flujo de Git | **Obligatoria** para cualquier operación de control de versiones |
| `staff-engineer` | Decisiones de arquitectura, diseño de sistema y revisión de código con visión de staff engineer | Usar cuando se requiera diseño estructural o revisión técnica profunda |
| `architecture-review` | Revisión de arquitectura de software y validación de interfaces y contratos | Usar cuando se evalúe diseño de componentes o interfaces entre módulos |

Otras skills se invocan únicamente cuando el caso específico lo justifique.



## Mapa de navegación (LEER ANTES DE BUSCAR)

### Capas del monolito (`internal/`)
- `internal/domain/`        → **entidades y errores de dominio**. NO lógica de orquestación.
  - `models.go`             → structs compartidos (Order, etc.)
  - `payment.go`, `shipment.go`, `inventory_reservation.go`, `owners.go`
  - `errors.go`             → errores de negocio
- `internal/service/`       → **lógica de negocio / casos de uso / orquestación**
  - `orchestrator.go`       → ⭐ orquestador de la saga (punto de entrada al flujo)
  - `circuit_breaker.go`, `recovery_worker.go`
  - `shipment_remote.go`    → adaptador de envío remoto
  - `payment_remote.go`     → adaptador de pagos remoto
  - `inventory_remote.go`   → adaptador remoto de inventario
  - `fake_payment.go`, `payment_sandbox.go`   → stubs de pago
  - `test/`                 → helpers de test del paquete
- `internal/repository/`    → **acceso a datos (SQL)** por agregado
  - `envio.go`, `payment.go`, `inventory_reservation.go`, `saga_paso.go`
- `internal/handler/`       → **HTTP (handlers, middleware, response)**
  - `orders.go`, `health.go`, `middleware.go`, `response.go`
  - `ratelimit/`            → rate limiting in-memory
- `internal/db/postgres.go` → conexión a DB
- `internal/integration/`   → tests de integración end-to-end (saga)

### Entrypoint
- `cmd/server/main.go`      → wiring: DB, repos, servicios, handlers

### Servicios containerizados (`services/`)
- `services/inventory/`     → Servicio de inventario con propio Dockerfile y migraciones
- `services/shipment/`      → Servicio de envío con propia configuración
- `services/payment/`       → Servicio de pagos con HTTP API (PostgreSQL, idempotente)

**Nota:** Estructura de monolito con módulos containerizados que pueden extraerse independientemente cuando sea necesario. Wiring principal en `cmd/server/main.go`.

### Migraciones
- `migrations/`             → migraciones del monolito (orden por prefijo numérico)
- `services/*/migrations/`  → migraciones propias de cada microservicio

### Documentación de diseño
- `axiom_context/`          → contexto, dominio, casos de uso, arquitectura
  - `03-dominio.md`, `04-casos-de-uso.md`, `07-arquitectura.md`
- `ALINEAMIENTO_ARQUITECTURA_IMPLEMENTACION.md`
- `context.md`, `aprendizaje.md`, `refactorizacion_servicios.md`

## Preguntas frecuentes → dónde mirar

| Pregunta | Archivo |
|---|---|
| ¿Dónde se define un servicio/entidad de dominio? | `internal/domain/<entidad>.go` |
| ¿Dónde está la lógica de orquestación de la saga? | `internal/service/orchestrator.go` |
| ¿Dónde se registra/wirea todo? | `cmd/server/main.go` |
| ¿Dónde está el SQL de X? | `internal/repository/<agregado>.go` |
| ¿Dónde está el schema de la DB? | `migrations/*.sql` |
| ¿Dónde está el endpoint HTTP de órdenes? | `internal/handler/orders.go` |
| ¿Dónde se define el contrato del servicio de envío? | `services/shipment/main.go` o `internal/service/shipment_remote.go` |
| ¿Dónde se define el contrato del servicio de pagos? | `services/payment/main.go` o `internal/service/payment_remote.go` |
| ¿Por qué se tomó decisión X? | `axiom_context/07-arquitectura.md` |

> Si la respuesta NO está en esta tabla → **pregunta al humano antes de buscar**.



