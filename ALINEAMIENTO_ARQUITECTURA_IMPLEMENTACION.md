# Alineamiento Arquitectura → Implementación

Propósito: verificar que el software refleja las decisiones arquitectónicas establecidas previamente y evitar divergencia entre los tres caminos: operacional, estructural y de restricciones.

---

## 1. Alineamiento Operacional

La implementación se comporta como la arquitectura menciona que debe comportarse.

| Comportamiento arquitectónico | Estado en implementación |
|------------------------------|--------------------------|
| Flujo forward: crear pedido → reservar inventario → procesar pago → crear envío | Implementado en `Orchestrator.ExecuteForward` (con dueños stub para inventario y envío; fake para pago). |
| Compensación en orden inverso al fallar un paso | Pendiente de implementar en `Orchestrator`. |
| Estados de paso: `PENDING → IN_PROGRESS → COMPLETED / FAILED / UNKNOWN` | Implementado en `domain.SagaStatus` y `CHECK` de `saga_steps`. |
| Compensación: `COMPENSATING → COMPENSATED / FAILED` | Definido en dominio; no implementado en orquestador. |
| Regla: no compensar `UNKNOWN` sin consultar primero al proveedor | Documentado en `04-casos-de-uso.md`; pendiente de implementar en Go. |
| Prueba del crash: pago aprobado + proceso muere antes de persistir paso → no duplica cobro, cierra Saga | Pendiente (requiere base de datos real y sandbox de pago). |

---

## 2. Alineamiento Estructural

La estructura del código refleja lo diseñado: elementos y sus relaciones.

| Capa | Archivos implementados | Relación con capas |
|------|----------------------|-------------------|
| Punto de entrada | `cmd/server/main.go` | Inicializa DB, repositorios, servicio, handlers. |
| Dominio | `internal/domain/*.go` | Modelos, errores, interfaces (`InventoryOwner`, `PaymentOwner`, `ShipmentOwner`). |
| Acceso a datos | `internal/db/postgres.go` | Conexión a PostgreSQL con `sqlx` + `pgx`. |
| Repositorios | `internal/repository/*.go` | `SagaStepRepository`, `InventoryReservationRepository`, `PaymentRepository`. |
| Servicio | `internal/service/*.go` | `Orchestrator`, `FakePaymentProvider`, stubs. |
| Handlers HTTP | `internal/handler/orders.go` | `CreateOrder`, `GetSaga`. |
| Esquema SQL | `migrations/000_orders.sql` ... `003_payments.sql` | Tablas `orders`, `saga_steps`, `inventory_reservations`, `payments`. |

Flujo de dependencias: `handler` → `service` → `repository` → `db` → PostgreSQL.

---

## 3. Alineamiento de Restricciones

El software contiene las restricciones que la arquitectura menciona sobre lo que puede y no hacer la implementación.

| Restricción arquitectónica | Dónde está escrita | Implementada en código |
|----------------------------|-------------------|------------------------|
| `UNIQUE(idempotency_key)` en `orders` y `payments` | `000_orders.sql`, `003_payments.sql` | Sí, en SQL. |
| `UNIQUE(order_id, product_id)` en `inventory_reservations` | `002_inventory_reservations.sql` | Sí, en SQL. |
| `UNIQUE(idempotency_key, operation_key)` en `saga_steps` | `001_saga_steps.sql` | Sí, en SQL. |
| `CHECK` de estados válidos en `saga_steps` y `payments` | Migraciones SQL | Sí, en SQL. |
| Tipo fuerte `SagaStatus` en Go (evita strings sueltos) | `internal/domain/models.go` | Sí, en Go. |
| Update atómico `RESERVED → RELEASED` en misma transacción | `002_inventory_reservations.sql` | Parcial: `InventoryReservationRepository.Release` usa `*sqlx.Tx`, pero el orquestador no abre transacciones para agrupar pasos. |
| No compensar `UNKNOWN` sin consultar | `04-casos-de-uso.md` | No implementado en Go (pendiente). |

---

## Matriz de alineamiento resumen

| Área | Arquitectura dice | Implementación actual | Estado |
|------|-------------------|----------------------|--------|
| Flujo Saga | Forward + compensación inversa | Forward implementado; compensación pendiente | Parcial |
| Idempotencia | UNIQUE en claves | UNIQUE en BD; falta validación previa en Go antes de insertar | Parcial |
| Estados Saga | 7 estados con CHECK | CHECK en BD; `type SagaStatus` en Go | Completo |
| Transacciones | Atomicidad local | Release atómico con Tx; resto sin Tx | Parcial |
| Consulta por clave | `consultarPorClave` / `consultarPorPedido` | `PaymentRepository.GetByIdempotencyKey` implementado | Completo (pagos) |

---

## Pendiente para cerrar alineamiento

1. Implementar compensación inversa en `Orchestrator`.
2. Agregar validación de idempotencia en Go antes de `INSERT` (no confiar solo en el error de BD).
3. Abrir transacciones en el orquestador para pasos que las requieran.
4. Implementar regla "no compensar UNKNOWN sin consultar".
5. Conectar pasarela de pago sandbox real (bloqueado por acceso).
6. Ejecutar pruebas de crash y reconciliación.

---

*Este documento es el filtro de alineamiento. Cualquier cambio en la implementación debe verificarse contra estas tres dimensiones antes de integrarse.*
