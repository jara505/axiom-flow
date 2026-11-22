# Guion de demostración — Saga de pedidos

Cinco escenarios ejecutables con el `FakePaymentProvider` para validar el flujo
Comando → Reserva → Pago → Envío y sus compensaciones.

> Requisitos previos: base de datos con las tablas definidas en `03-dominio` y los contratos de `04-casos-de-uso`.

---

## Escenario 1 — Flujo feliz (happy path)

**Objetivo:** validar que un pedido completo avanza por los cinco pasos sin fallos.

| Paso | Acción | Resultado esperado |
|------|--------|--------------------|
| 1 | `POST /pedidos` con `idempotency-key: demo-001` | Pedido creado, `estado_pedido = PENDIENTE_PAGO` |
| 2 | `Reservar inventario` (paso `RESERVAR_INVENTARIO`) | Reserva creada, stock reducido |
| 3 | `Pagar` con `FakePaymentProvider` (resultado `APROBADO`) | Pago `APROBADO`, pedido `PAGADO` |
| 4 | `Enviar` (paso `CREAR_ENVIO`) | Envío `CREADO`, pedido `ENVIADO` |
| 5 | `GET /pedidos/{id}/saga` | Saga `COMPLETADA`, todos los pasos `COMPLETADO` |

**Criterio de éxito:** pedido en `ENVIADO`, pago `APROBADO`, reserva `RESERVADO`, envío `CREADO`.

---

## Escenario 2 — Pago rechazado

**Objetivo:** validar que un rechazo de pago cancela el pedido y libera la reserva.

| Paso | Acción | Resultado esperado |
|------|--------|--------------------|
| 1 | `POST /pedidos` con `idempotency-key: demo-002` | Pedido `PENDIENTE_PAGO` |
| 2 | Reservar inventario | Reserva `RESERVADO` |
| 3 | Pagar con `FakePaymentProvider` forzado a `RECHAZADO` | Pago `RECHAZADO` |
| 4 | Orquestador cancela | Pedido `CANCELADO` |
| 5 | Compensación: liberar reserva | Reserva `LIBERADO` |
| 6 | `GET /pedidos/{id}/saga` | Saga con paso `PROCESAR_PAGO = FALLIDO`, compensación `COMPLETADA` |

**Criterio de éxito:** pedido `CANCELADO`, reserva `LIBERADO`, sin envío creado.

---

## Escenario 3 — Pago duplicado (idempotencia)

**Objetivo:** validar que el mismo `idempotency-key` no produce un cobro doble.

| Paso | Acción | Resultado esperado |
|------|--------|--------------------|
| 1 | `POST /pedidos` con `idempotency-key: demo-003` | Pedido creado |
| 2 | Primera llamada `charge` con `demo-003` | Pago `APROBADO`, se cobra una vez |
| 3 | Segunda llamada `charge` con `demo-003` (mismo clave) | Se devuelve el resultado cacheado, **no se cobra dos veces** |
| 4 | `GET /pedidos/{id}/saga` | Un solo paso `PROCESAR_PAGO = COMPLETADO` |

**Criterio de éxito:** un único registro de pago, monto cobrado una sola vez.

---

## Escenario 4 — Fallo de envío tras pago aprobado (compensación)

**Objetivo:** validar compensaciones cuando el pago ya se consumó pero el envío falla.

| Paso | Acción | Resultado esperado |
|------|--------|--------------------|
| 1 | `POST /pedidos` con `idempotency-key: demo-004` | Pedido `PENDIENTE_PAGO` |
| 2 | Reservar inventario | Reserva `RESERVADO` |
| 3 | Pagar `APROBADO` | Pago `APROBADO`, pedido `PAGADO` |
| 4 | Crear envío forzado a `FALLIDO` | Envío `FALLIDO` |
| 5 | Compensación: reembolso + `REEMBOLSAR_PAGO` | Pago `REEMBOLSADO` |
| 6 | Compensación: `LIBERAR_RESERVA` | Reserva `LIBERADO` |
| 7 | Pedido pasa a `CANCELADO` | Pedido `CANCELADO` |
| 8 | `GET /pedidos/{id}/saga` | Saga con compensaciones `COMPLETADA` |

**Criterio de éxito:** pago reembolsado, reserva liberada, pedido `CANCELADO`, ningún envío efectivo.

---

## Escenario 5 — Resultado desconocido / interrupción

**Objetivo:** validar que una operación con estado `DESCONOCIDO` no se repite ciegamente y se recupera consultando el proveedor.

| Paso | Acción | Resultado esperado |
|------|--------|--------------------|
| 1 | `POST /pedidos` con `idempotency-key: demo-005` | Pedido creado |
| 2 | Reservar inventario | Reserva `RESERVADO` |
| 3 | Pagar con `FakePaymentProvider` sin resultado definido | Estado `DESCONOCIDO` (proceso interrumpido) |
| 4 | Orquestador consulta al proveedor con la misma `idempotency-key` | Se obtiene el resultado real (`APROBADO` o `RECHAZADO`) |
| 5 | Orquestador decide continuar o compensar según el resultado consultado | Saga avanza o compensa sin cobro duplicado |
| 6 | `GET /pedidos/{id}/saga` | Paso `PROCESAR_PAGO` resuelto, no duplicado |

**Criterio de éxito:** la saga se resuelve basándose en el resultado consultado, no en una suposición; no hay cobro duplicado.

---

## Cómo ejecutar con el fake

```typescript
import { FakePaymentProvider } from './04-casos-de-uso';

const pago = new FakePaymentProvider('APROBADO');

// Escenario 2: forzar rechazo
pago.setOutcome('demo-002', 'RECHAZADO');

// Escenario 4: envío falla (se maneja en el orquestador de envíos)
// Escenario 5: dejar sin definir para forzar DESCONOCIDO, luego consultar
```
