# Features: Consistencia fuerte en pagos

## Alcance
Consistencia fuerte únicamente para el dominio de pagos. El resto de la Saga mantiene consistencia eventual mediante compensación.

## Medidas a implementar

1. **Transacción local atómica**  
   Reservar fondos y escribir el estado del pago (`PENDING` → `COMPLETED` o `FAILED`) en la misma transacción de la base de datos de pagos. Responder `OK` al orquestador solo después del `COMMIT`.

2. **Bloqueo pesimista (`SELECT FOR UPDATE`)**  
   Bloquear la fila de pago/orden dentro de esa transacción para evitar que dos solicitudes cobren la misma orden simultáneamente.

3. **Idempotency key**  
   El orquestador envía una clave de idempotencia en cada petición de pago. El servicio de pagos debe validarla y rechazar cobros duplicados ante reintentos.

4. **Outbox opcional**  
   Escribir el evento de pago confirmado en una tabla `outbox` de la misma base de datos de pagos, dentro de la misma transacción, para que el orquestador lo consuma de forma confiable sin depender solo de la respuesta HTTP.

## Distinción de fallas

- **Falla de transacción / infraestructura** (timeout de red, caída de DB): se maneja con reintentos, circuit breaker y bloqueo pesimista.
- **Falla de negocio** (pago aprobado pero no hay stock, tarjeta rechazada): se maneja con compensación Saga en orden inverso (liberar fondos, revertir reserva, cancelar envío).