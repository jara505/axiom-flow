# Propuesta de soluciones

Las propuestas todavía deben evaluarse y seleccionarse según los objetivos y el alcance del MVP.

## Patrón de análisis

Cada escenario se analizará mediante los siguientes elementos:

| Elemento | Propósito |
|---|---|
| **Solución propuesta** | Mecanismo para abordar el problema. |
| **Trade-offs** | Costes, ventajas y limitaciones de la solución. |
| **Fallback** | Alternativa cuando la solución principal no sea suficiente o aplicable. |

---

## 1. Fallos parciales

**Problema:** Ver fallo 1 de [[01-contexto]].

### Solución propuesta: Rollback de la transacción local

Utilizar transacciones locales para garantizar que los cambios de cada operación se confirmen íntegramente o se reviertan ante un fallo dentro de esa transacción.

### Trade-offs

- El rollback local no revierte las operaciones anteriores que ya fueron confirmadas.
- No garantiza la coherencia de toda la Saga por sí solo.

### Fallback

Pendiente de definir.

> **Consideración:** Para gestionar fallos entre varias operaciones de una Saga, es necesario evaluar mecanismos de recuperación y compensación, ya que no existe un rollback global automático.

---

## 2. Operaciones duplicadas

**Problema:** Ver fallo 2 de [[01-contexto]].

### Solución propuesta: Idempotencia

Diseñar las operaciones para que la repetición de una misma solicitud no produzca efectos adicionales, utilizando identificadores únicos y verificando si ya fue procesada.

### Trade-offs

| Aspecto | Limitación |
|---|---|
| **Latencia** | Verificar solicitudes anteriores añade trabajo. |
| **Almacenamiento** | Es necesario conservar identificadores y resultados. |
| **Complejidad** | Hay que controlar solicitudes repetidas y concurrentes. |

### Fallback o controles complementarios

Utilizar restricciones de base de datos, como `UNIQUE` y `CHECK`, para impedir determinados datos duplicados o inválidos.

Estos controles complementan la idempotencia, pero no la sustituyen: una restricción de unicidad no evita por sí sola todos los efectos duplicados de una operación.

---

### 2.1. Pagos duplicados

**Problema:** Ver fallo 2 de [[01-contexto]] (pago duplicado).

**Solución propuesta:** Idempotencia (ver 2) y control del estado del pago.

Identificar cada solicitud de pago de forma única y registrar su estado para evitar repetir el cobro sin verificar previamente el resultado de la operación anterior.

**Trade-offs**

- Almacenar y gestionar identificadores de operaciones.
- Gestionar estados pendientes, fallidos y de resultado desconocido.
- Verificar resultados con el proveedor cuando sea posible.

**Fallback**

Si no se puede confirmar el resultado, no iniciar automáticamente un nuevo cobro. Consultar el estado con el proveedor cuando sea posible y mantener la operación pendiente de resolución si persiste la incertidumbre.

---

## 3. Concurrencia en el inventario

**Problema:** Ver fallo 3 de [[01-contexto]].

### Solución propuesta: Bloqueo pesimista (*Exclusive Lock*) — solo intra-servicio

Utilizar un bloqueo sobre el recurso que se está modificando para serializar las reservas concurrentes **dentro de un mismo servicio**. Las solicitudes que necesiten el mismo recurso deberán esperar a que se libere el bloqueo. Entre servicios autónomos con bases de datos independientes, `SELECT FOR UPDATE` no puede serializar reservas entre servicios.

### Trade-offs

| Aspecto | Limitación |
|---|---|
| **Esperas** | Algunas solicitudes pueden experimentar mayor latencia. |
| **Deadlocks** | Los bloqueos pueden generar interbloqueos si se adquieren en un orden incompatible. |
| **Contención** | Muchas solicitudes sobre el mismo inventario pueden reducir el rendimiento. |
| **Alcance** | Solo efectivo dentro de una misma base de datos; no protege contra carreras entre servicios separados. |

### Fallback o mecanismo complementario

Aplicar restricciones en la base de datos y actualizaciones atómicas que comprueben la disponibilidad al realizar la reserva. Esto puede evitar reservas superiores al inventario sin depender exclusivamente de bloqueos explícitos.

---

## 4. Interrupciones y fallos durante la ejecución

Durante la ejecución de un pedido, pueden producirse fallos que impidan completar operaciones, interrumpan el proceso o impidan conocer el resultado de una operación. Estas situaciones pueden dejar la Saga incompleta o generar inconsistencias entre los recursos involucrados.

### Solución propuesta: Persistencia y recuperación de la Saga

Registrar el estado de cada operación y persistir el progreso de la Saga para que el orquestador pueda identificar las operaciones completadas y reanudar la ejecución desde el punto adecuado.

#### Fallos parciales de la Saga

**Trade-offs**

- El estado registrado no siempre refleja el resultado real de una operación.
- Es necesario distinguir entre operaciones completadas, fallidas y con resultado desconocido.
- No todos los fallos pueden resolverse automáticamente.

**Fallback:** Ejecutar acciones compensatorias cuando corresponda, como liberar una reserva de inventario o solicitar la devolución de un pago.

El orquestador deberá determinar si corresponde reanudar, compensar o mantener la ejecución pendiente de resolución. Las compensaciones pueden fallar y no garantizan restituir exactamente el estado original.

#### Orden y condiciones de compensación

La compensación se ejecuta en orden inverso a los pasos completados. Si se completaron pago y reserva, primero `REEMBOLSAR_PAGO` y después `LIBERAR_RESERVA`; si solo se completó la reserva, solo `LIBERAR_RESERVA`.

Un paso en estado `DESCONOCIDO` debe resolverse antes de decidir. Se consulta al dueño por la clave de idempotencia o, en la reserva, por `id_pedido`. Si se confirma que se ejecutó, se compensa; si no, se marca como fallido.

Cada compensación es una operación nueva y lleva su propia clave de idempotencia. Si una compensación falla, la Saga permanece en `COMPENSANDO` y no se marca como resuelta. El pedido pasa a `CANCELADO` solo cuando todas las compensaciones necesarias están `COMPLETADO`.

#### Interrupciones inesperadas

**Problema:** Ver fallo 6 de [[01-contexto]].

**Solución propuesta:** Comprobar la disponibilidad de los servicios cuando corresponda, para facilitar la recuperación tras una interrupción.

**Trade-offs**

- Comprobar la disponibilidad no garantiza que el servicio siga disponible al ejecutar la operación.
- La recuperación depende de conservar información suficiente sobre el progreso.
- Los reintentos pueden prolongar el proceso y afectar al usuario.

**Fallback:** Aplicar una política de fallo rápido cuando no convenga seguir intentando la operación y conservar el estado para recuperarlo después.

Un proveedor de pago alternativo podría considerarse en una implementación futura, siempre que se verifique primero el resultado del intento original para evitar cobros duplicados.

**Límite del MVP:** Se probarán interrupciones controladas. La alta disponibilidad y los proveedores de pago alternativos quedan fuera del alcance inicial.

#### Resultados desconocidos

**Problema:** Una operación puede completarse sin que el orquestador reciba o registre el resultado. Esto impide determinar qué ocurrió y puede provocar repeticiones o compensaciones innecesarias.

**Solución propuesta:** Asignar un identificador único a cada operación, persistir su resultado en el componente que la ejecuta y permitir que el orquestador lo consulte antes de decidir cómo continuar.

Cuando el efecto y su registro se encuentren en la misma base de datos (mismo servicio), se procurará proteger ambos mediante una única transacción local. Entre servicios autónomos no existe transacción local compartida; se debe usar compensación Saga con idempotencia.

**Trade-offs**

- Las consultas y verificaciones adicionales pueden aumentar la latencia y la complejidad.
- La recuperación depende de que el ejecutor conserve un registro fiable del resultado.
- Si el efecto ocurre fuera de la transacción local y no puede verificarse, puede ser necesario mantener la operación pendiente de resolución.

**Fallback:** Si no es posible determinar el resultado, no asumir automáticamente que la operación falló. Mantenerla pendiente hasta obtener información suficiente para decidir si se debe reintentar, continuar o compensar.

---

## 5. Pedido sin condiciones necesarias

**Problema:** Ver fallo 5 de [[01-contexto]]. Las existencias pueden cambiar entre la validación inicial y la reserva.

### Solución propuesta: Validación previa y comprobación durante la reserva

Validar previamente la disponibilidad y volver a comprobarla de forma atómica al reservar, garantizando que la cantidad asignada no supere las existencias disponibles.

### Trade-offs

- La validación inicial no garantiza que el inventario siga disponible al reservarlo.
- La concurrencia debe gestionarse para evitar sobreventas.
- La comprobación efectiva debe realizarse de forma segura y atómica.

### Fallback

Si la reserva falla, detener el avance de la Saga y ejecutar las acciones compensatorias necesarias sobre las operaciones que ya se hayan completado.

---

## Tabla de compensaciones

| # | Caso | Naturaleza | Acción |
|---|---|---|---|
| 1 | Reserva de inventario falla | Aborto previo | No se crea pedido / CANCELADO |
| 2 | Pago aprobado + no se persiste PAGADO | Efecto consumado | Compensación: reembolso + CANCELADO |
| 3 | Pago rechazado | Resultado esperado del flujo | CANCELADO + liberar reserva |
