# Test Manual - Axiom Flow Walking Skeleton

## Objetivo
Validar el funcionamiento del sistema end-to-end y casos de fallo.
El sistema requiere `idempotency_key` en cada petición (campo obligatorio).

---### Modo Ejecución
1. Asegurarse de tener PostgreSQL corriendo
2. Ejecutar el servidor: `go run cmd/server/main.go`
3. Utilizar `curl` o Postman para hacer las peticiones

### 📋 Comandos curl (copiar y pegar según caso):

#### **TC-001 - Flujo Complejo (Happy Path)** - Crear orden:
```bash
curl -X POST http://localhost:8080/orders \
  -H "Content-Type: application/json" \
  -d '{"customer_id":1,"order_number":"ORD-001","idempotency_key":"key-tc001","subtotal":99.99,"total":99.99,"product_id":1,"quantity":1}'
```
#### **TC-001 - Flujo Complejo (Happy Path)** - Consultar saga:
```bash
curl http://localhost:8080/orders/1/saga
```
#### **TC-002 - Idempotencia de Pago** - Primera orden:
```bash
curl -X POST http://localhost:8080/orders \
  -H "Content-Type: application/json" \
  -d '{"customer_id":1,"order_number":"ORD-002","idempotency_key":"key-tc002","subtotal":99.99,"total":99.99,"product_id":1,"quantity":1}'
```
#### **TC-002 - Idempotencia de Pago** - Segunda orden (mismo key):
```bash
curl -X POST http://localhost:8080/orders \
  -H "Content-Type: application/json" \
  -d '{"customer_id":1,"order_number":"ORD-003","idempotency_key":"key-tc002","subtotal":49.99,"total":49.99,"product_id":1,"quantity":1}'
```
#### **TC-003 - Flujo con Pago Fallido**:
```bash
curl -X POST http://localhost:8080/orders \
  -H "Content-Type: application/json" \
  -d '{"customer_id":1,"order_number":"ORD-003","idempotency_key":"key-tc003","subtotal":99.99,"total":99.99,"product_id":1,"quantity":1}'
```
#### **TC-004 - Consulta de Saga por ID**:
```bash
curl http://localhost:8080/orders/1/saga
```
#### **TC-005 - Creación de Envío (Shipment)** - Verificar en BD:
```bash
# Ejecutar TC-001 primero, luego consultar tabla:
echo 'SELECT * FROM shipments WHERE order_id = 1;'
```

---### Casos de Prueba (Checkbox)

#### [x] **TC-001: Flujo Complejo (Happy Path)**
   - **Descripción**: Crear una orden y verificar que la saga completa los 4 pasos (CREATE_ORDER → RESERVE_INVENTORY → PROCESS_PAYMENT → CREATE_SHIPMENT).
   - **Petición**: Usar comando "Flujo Complejo (Happy Path)" arriba
   - **Esperado**: Los 4 pasos aparecen con status `COMPLETED` en la respuesta JSON
   - **Tipo de Prueba**: Integración / Funcional
   - **Observaciones**: Verificar que el `idempotency_key` en la respuesta tiene formato `order-1-STEP-attempt-1`

#### [x] **TC-002: Idempotencia / Re-entrada**
   - **Descripción**: Verificar comportamiento al crear orders repetidas con la misma clave de idempotencia.
   - **Petición**: Usar los comandos "Idempotencia de Pago" arriba (primera y segunda petición)
   - **Esperado**: La segunda petición con el mismo `idempotency_key` no debe crear un nuevo step duplicado; la saga debe ser inteligente ante entradas repetidas.
   - **Tipo de Prueba**: Idempotencia / Re-entrada
   - **Observaciones**: El sistema valida el campo `idempotency_key` como obligatorio. Anotar si la segunda petición falla o tiene comportamiento inesperado.

#### [x] **TC-003: Flujo con Pago Fallido**
   - **Descripción**: Forzar que el pago falle y verificar la compensación (que los pasos anteriores se mantengan COMPLETED y CREATE_SHIPMENT no se ejecuta o falla).
   - **Petición**: Usar comando "Flujo con Pago Fallido" arriba
   - **Esperado**: La saga falla en `PROCESS_PAYMENT`; los pasos anteriores (`CREATE_ORDER`, `RESERVE_INVENTORY`) deben mantenerse `COMPLETED`; el paso `CREATE_SHIPMENT` no debe ejecutarse o debe marcarse `FAILED`.
   - **Tipo de Prueba**: Compensación / Fallo
   - **Observaciones**: 

#### [x] **TC-004: Consulta de Saga por ID**
   - **Descripción**: Verificar que `GET /orders/{id}/saga` devuelve la información correcta de la saga ejecutada.
   - **Petición**: Usar comando "Consulta de Saga por ID" arriba
   - **Esperado**: JSON con estructura `{order_id, steps[]}` donde cada step tiene `step_name`, `status`, `idempotency_key`, `operation_key`.
   - **Tipo de Prueba**: Consulta / API
   - **Observaciones**: Los `idempotency_key` en la respuesta deben seguir el patrón `order-{ID}-{stepName}-attempt-{attempt}`

#### [x] **TC-005: Creación de Envío (Shipment)**
   - **Descripción**: Verificar que se crea el registro de envío en la base de datos.
   - **Petición**: Ejecutar TC-001 primero, luego usar comando de verificación en BD
   - **Esperado**: En la base de datos, la tabla `shipments` debe tener un registro con `order_id` correspondiente y status `SHIPPED`.
   - **Tipo de Prueba**: Base de datos / Almacenamiento
   - **Observaciones**: 

---### Instrucciones Generales
1. **Limpiar Base de Datos entre pruebas**:
   ```bash
   echo 'TRUNCATE shipments, saga_steps, payments, inventory_reservations, orders RESTART IDENTITY CASCADE;' | psql -U postgres -d axiom_flow
   ```
2. **Verificar que el servidor está corriendo**: `go run cmd/server/main.go`
3. **Usar `jq` para formatear respuestas JSON si es necesario** (ej: `curl http://localhost:8080/orders/1/saga | jq .`)
4. **Llenar las "Observaciones" después de cada prueba**

---### Estructura de Datos Esperada (Ejemplo real de salida)
```json
{
  "order_id": 1,
  "steps": [
    {
      "id": 1,
      "order_id": 1,
      "step_name": "CREATE_ORDER",
      "status": "COMPLETED",
      "attempt": 1,
      "idempotency_key": "order-1-CREATE_ORDER-attempt-1",
      "operation_key": "order-1-CREATE_ORDER",
      "started_at": "2026-10-31T17:05:35.153306Z",
      "finished_at": "2026-10-31T17:05:35.153306Z"
    },
    {
      "id": 4,
      "order_id": 1,
      "step_name": "CREATE_SHIPMENT",
      "status": "COMPLETED",
      "attempt": 1,
      "idempotency_key": "order-1-CREATE_SHIPMENT-attempt-1",
      "operation_key": "order-1-CREATE_SHIPMENT",
      "started_at": "2026-10-31T17:05:35.153306Z",
      "finished_at": "2026-10-31T17:05:35.153306Z"
    },
    {
      "id": 3,
      "order_id": 1,
      "step_name": "PROCESS_PAYMENT",
      "status": "COMPLETED",
      "attempt": 1,
      "idempotency_key": "order-1-PROCESS_PAYMENT-attempt-1",
      "operation_key": "order-1-PROCESS_PAYMENT",
      "started_at": "2026-10-31T17:05:35.153306Z",
      "finished_at": "2026-10-31T17:05:35.153306Z"
    },
    {
      "id": 2,
      "order_id": 1,
      "step_name": "RESERVE_INVENTORY",
      "status": "COMPLETED",
      "attempt": 1,
      "idempotency_key": "order-1-RESERVE_INVENTORY-attempt-1",
      "operation_key": "order-1-RESERVE_INVENTORY",
      "started_at": "2026-10-31T17:05:35.153306Z",
      "finished_at": "2026-10-31T17:05:35.153306Z"
    }
  ]
}
```
#### **TC-002 - Idempotencia de Pago** - Primera orden:
```bash
curl -X POST http://localhost:8080/orders \
  -H "Content-Type: application/json" \
  -d '{"customer_id":1,"order_number":"ORD-002","idempotency_key":"key-tc002","subtotal":99.99,"total":99.99,"product_id":1,"quantity":1}'
```
#### **TC-002 - Idempotencia de Pago** - Segunda orden (mismo key):
```bash
curl -X POST http://localhost:8080/orders \
  -H "Content-Type: application/json" \
  -d '{"customer_id":1,"order_number":"ORD-003","idempotency_key":"key-tc002","subtotal":49.99,"total":49.99,"product_id":1,"quantity":1}'
```
#### **TC-003 - Flujo con Pago Fallido**:
```bash
curl -X POST http://localhost:8080/orders \
  -H "Content-Type: application/json" \
  -d '{"customer_id":1,"order_number":"ORD-003","idempotency_key":"key-tc003","subtotal":99.99,"total":99.99,"product_id":1,"quantity":1}'
```
#### **TC-004 - Consulta de Saga por ID**:
```bash
curl http://localhost:8080/orders/1/saga
```
#### **TC-005 - Creación de Envío (Shipment)** - Verificar en BD:
```bash
# Ejecutar TC-001 primero, luego consultar tabla:
echo 'SELECT * FROM shipments WHERE order_id = 1;'
```
---### Casos de Prueba (Checkbox)

#### [x] **TC-001: Flujo Complejo (Happy Path)**
   - **Descripción**: Crear una orden y verificar que la saga completa los 4 pasos (CREATE_ORDER → RESERVE_INVENTORY → PROCESS_PAYMENT → CREATE_SHIPMENT).
   - **Petición**: Usar comando "Flujo Complejo (Happy Path)" arriba
   - **Esperado**: Los 4 pasos aparecen con status `COMPLETED` en la respuesta JSON
   - **Tipo de Prueba**: Integración / Funcional
   - **Observaciones**: Verificar que el `idempotency_key` en la respuesta tiene formato `order-1-STEP-attempt-1`

#### [x] **TC-002: Idempotencia / Re-entrada**
   - **Descripción**: Verificar comportamiento al crear orders repetidas con la misma clave de idempotencia.
   - **Petición**: Usar los comandos "Idempotencia de Pago" arriba (primera y segunda petición)
   - **Esperado**: La segunda petición con el mismo `idempotency_key` no debe crear un nuevo step duplicado; la saga debe ser inteligente ante entradas repetidas.
   - **Tipo de Prueba**: Idempotencia / Re-entrada
   - **Observaciones**: El sistema valida el campo `idempotency_key` como obligatorio. Anotar si la segunda petición falla o tiene comportamiento inesperado.

#### [x] **TC-003: Flujo con Pago Fallido**
   - **Descripción**: Forzar que el pago falle y verificar la compensación (que los pasos anteriores se mantengan COMPLETED y CREATE_SHIPMENT no se ejecuta o falla).
   - **Petición**: Usar comando "Flujo con Pago Fallido" arriba
   - **Esperado**: La saga falla en `PROCESS_PAYMENT`; los pasos anteriores (`CREATE_ORDER`, `RESERVE_INVENTORY`) deben mantenerse `COMPLETED`; el paso `CREATE_SHIPMENT` no debe ejecutarse o debe marcarse `FAILED`.
   - **Tipo de Prueba**: Compensación / Fallo
   - **Observaciones**: 

#### [x] **TC-004: Consulta de Saga por ID**
   - **Descripción**: Verificar que `GET /orders/{id}/saga` devuelve la información correcta de la saga ejecutada.
   - **Petición**: Usar comando "Consulta de Saga por ID" arriba
   - **Esperado**: JSON con estructura `{order_id, steps[]}` donde cada step tiene `step_name`, `status`, `idempotency_key`, `operation_key`.
   - **Tipo de Prueba**: Consulta / API
   - **Observaciones**: Los `idempotency_key` en la respuesta deben seguir el patrón `order-{ID}-{stepName}-attempt-{attempt}`

#### [x] **TC-005: Creación de Envío (Shipment)**
   - **Descripción**: Verificar que se crea el registro de envío en la base de datos.
   - **Petición**: Ejecutar TC-001 primero, luego usar comando de verificación en BD
   - **Esperado**: En la base de datos, la tabla `shipments` debe tener un registro con `order_id` correspondiente y status `SHIPPED`.
   - **Tipo de Prueba**: Base de datos / Almacenamiento
   - **Observaciones**: 

#### [x] **TC-006: Retry con Backoff Exponencial**
   - **Descripción**: Forzar un fallo transitorio en un paso (ej. pago rechazado temporalmente) y verificar que el sistema programa un reintento con `next_retry_at` en lugar de fallar inmediatamente.
   - **Petición**: Ejecutar TC-001 con un `idempotency_key` nuevo; si el pago falla por timeout simulado, verificar que `saga_steps` tiene `retry_count = 1` y `next_retry_at` con una hora futura.
   - **Esperado**: La saga no falla inmediatamente; el paso queda `IN_PROGRESS` con `next_retry_at` programado. El recovery worker debe reintentarlo automáticamente después del intervalo.
   - **Tipo de Prueba**: Resiliencia / Retry
   - **Observaciones**: Verificar columnas `retry_count` y `next_retry_at` en tabla `saga_steps`.

#### [x] **TC-007: Worker de Recuperación Automática**
   - **Descripción**: Verificar que el worker de recuperación (ejecutándose cada 10s) retoma pasos pendientes de retry y los completa.
   - **Petición**: Crear una orden donde un paso falle y quede programado para retry; esperar al menos 10 segundos; verificar que el paso se ejecutó y pasó a `COMPLETED`.
   - **Esperado**: Después del intervalo del worker, el paso previamente fallido pasa a `COMPLETED` y la saga continúa normalmente.
   - **Tipo de Prueba**: Resiliencia / Recuperación Automática
   - **Observaciones**: El worker se inicia automáticamente al levantar el servidor.

#### [x] **TC-008: Health Check**
   - **Descripción**: Verificar que el servicio responde y la base de datos está accesible.
   - **Petición:**
   ```bash
   curl http://localhost:8080/health
   ```
   - **Esperado**: JSON con `status: "up"`, métricas de órdenes creadas/fallidas, y timestamp.
   - **Tipo de Prueba**: Operacional / Infraestructura
   - **Observaciones**: 

#### [x] **TC-010: Rate Limiting (429 Too Many Requests)**
   - **Descripción**: Verificar que el servidor limita peticiones por IP y devuelve `429` cuando se supera el límite, con headers y cuerpo JSON informativos.
   - **Petición:** Realizar más de 60 peticiones rápidas a `http://localhost:8080/health` desde la misma IP.
   - **Esperado**: Después de 60 peticiones en una ventana de 1 minuto, el servidor responde con HTTP `429`, headers `X-RateLimit-Limit`, `X-RateLimit-Remaining`, `X-RateLimit-Reset`, `Retry-After`, y cuerpo JSON con `error: "rate_limit_exceeded"`.
   - **Tipo de Prueba**: Operacional / Protección de API
   - **Observaciones**: Límite configurado: 60 peticiones por minuto por IP (in-memory). Evadir vía VPN está fuera del alcance del MVP.

#### [x] **TC-009: Request ID en logs**
   - **Descripción**: Verificar que cada petición devuelve un header `X-Request-ID` único y que los logs incluyen ese ID.
   - **Petición:**
   ```bash
   curl -v http://localhost:8080/health
   ```
   - **Esperado**: La respuesta incluye `x-request-id` con un UUID. Los logs del servidor muestran ese mismo ID asociado a la petición.
   - **Tipo de Prueba**: Trazabilidad / Observabilidad
   - **Observaciones**: 

---### Instrucciones Generales
1. **Limpiar Base de Datos entre pruebas**:
   ```bash
   echo 'TRUNCATE shipments, saga_steps, payments, inventory_reservations, orders RESTART IDENTITY CASCADE;' | psql -U postgres -d axiom_flow
   ```
2. **Verificar que el servidor está corriendo**: `go run cmd/server/main.go`
3. **Usar `jq` para formatear respuestas JSON si es necesario** (ej: `curl http://localhost:8080/orders/1/saga | jq .`)
4. **Llenar las "Observaciones" después de cada prueba**

---### Estructura de Datos Esperada (Ejemplo real de salida)
```json
{
  "order_id": 1,
  "steps": [
    {
      "id": 1,
      "order_id": 1,
      "step_name": "CREATE_ORDER",
      "status": "COMPLETED",
      "attempt": 1,
      "idempotency_key": "order-1-CREATE_ORDER-attempt-1",
      "operation_key": "order-1-CREATE_ORDER",
      "started_at": "2026-10-31T17:05:35.153306Z",
      "finished_at": "2026-10-31T17:05:35.153306Z"
    },
    {
      "id": 4,
      "order_id": 1,
      "step_name": "CREATE_SHIPMENT",
      "status": "COMPLETED",
      "attempt": 1,
      "idempotency_key": "order-1-CREATE_SHIPMENT-attempt-1",
      "operation_key": "order-1-CREATE_SHIPMENT",
      "started_at": "2026-10-31T17:05:35.153306Z",
      "finished_at": "2026-10-31T17:05:35.153306Z"
    },
    {
      "id": 3,
      "order_id": 1,
      "step_name": "PROCESS_PAYMENT",
      "status": "COMPLETED",
      "attempt": 1,
      "idempotency_key": "order-1-PROCESS_PAYMENT-attempt-1",
      "operation_key": "order-1-PROCESS_PAYMENT",
      "started_at": "2026-10-31T17:05:35.153306Z",
      "finished_at": "2026-10-31T17:05:35.153306Z"
    },
    {
      "id": 2,
      "order_id": 1,
      "step_name": "RESERVE_INVENTORY",
      "status": "COMPLETED",
      "attempt": 1,
      "idempotency_key": "order-1-RESERVE_INVENTORY-attempt-1",
      "operation_key": "order-1-RESERVE_INVENTORY",
      "started_at": "2026-10-31T17:05:35.153306Z",
      "finished_at": "2026-10-31T17:05:35.153306Z"
    }
  ]
}
```
