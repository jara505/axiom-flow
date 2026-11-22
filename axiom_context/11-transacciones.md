# 11 - Transacciones: dudas y problemas abiertos

## Propósito

Documentar las dudas, vacíos y problemas abiertos a nivel de transacciones y consistencia en el sistema Axiom Flow. Este documento es un repositorio de preguntas abiertas que deben resolverse antes de considerar la implementación de transacciones como sólida.

## Estado actual documentado

### Lo que sí está declarado

| Origen | Declaración | Limitación observada |
|--------|-------------|----------------------|
| `03-dominio.md` | `ReservaInventario` comenta: *"Regla de update atómico: el estado solo puede transitar RESERVADO → LIBERADO dentro de la misma transacción local que valida stock y afecta la fila."* | No se especifica el nivel de aislamiento (`READ COMMITTED`, `REPEATABLE READ`, `SERIALIZABLE`). |
| `03-dominio.md` | `Pago` comenta: *"Soporta reintentos si una transacción SAGA falla".* | No se define cómo se detecta una transacción SAGA fallida ni cómo se coordina el reintento entre servicios. |
| `03-dominio.md` | `saga_steps` declara `UNIQUE(idempotency_key)`, `UNIQUE(operation_key)`, `UNIQUE(order_id, step_name)`. | Estas restricciones aplican solo dentro de la BD del servicio orquestador; no protegen contra carreras entre servicios autónomos. |
| `04-casos-de-uso.md` | Enumera fallos: pedido creado pero BD inconsistente, pago duplicado, reserva parcial sin estado conocido, duplicación silenciosa. | No especifica aislamiento de transacción ni mecanismo de bloqueo para evitar condiciones de carrera en la reserva de inventario. |
| `04-casos-de-uso.md` | Regla: *"NUNCA compensar un paso en estado DESCONOCIDO sin consultar primero al dueño".* | No define el nivel de aislamiento ni el mecanismo de bloqueo para la consulta + decisión de compensación como unidad atómica. |
| `05-propuestas.md` | Propone *bloqueo pesimista (Exclusive Lock)* para concurrencia en inventario. | No especifica si es `SELECT FOR UPDATE`, `SELECT FOR NO KEY UPDATE`, ni el nivel de aislamiento requerido para que sea efectivo. |
| `05-propuestas.md` | Propone: *"Cuando el efecto y su registro se encuentren en la misma base de datos, se procurará proteger ambos mediante una única transacción local."* | Solo aplica dentro de un mismo servicio. Entre servicios autónomos no existe transacción distribuida ACID; se requiere compensación Saga. |
| `05-propuestas.md` | Propone *idempotencia* para operaciones duplicadas y `UNIQUE` en BD como restricción complementaria. | No aclara si el nivel de aislamiento por defecto (`READ COMMITTED`) es suficiente para la verificación de idempotencia bajo concurrencia, ni si se necesita `SELECT FOR UPDATE`. |

---

## Dudas abiertas (a resolver en este documento)

### Transacciones y niveles de aislamiento

1. **¿Qué nivel de aislamiento se usa actualmente?**  
   El nivel de aislamiento dependera del contexto asi que no hay un aislamiento universal en las transacciones.
   
2. **¿Es suficiente `READ COMMITTED` para la reserva de inventario bajo concurrencia?**  
   Se propone bloqueo pesimista, pero no se especifica si `READ COMMITTED` + `SELECT FOR UPDATE` es suficiente, o si se requiere `REPEATABLE READ` o `SERIALIZABLE`.


   respuesta:
| Elemento | Decisión | Por qué |
|---|---|---|
| **Mecanismo** | Lock pesimista por fila (`SELECT FOR UPDATE`) | Escritura + no puede fallar + lost update |
| **Orden de adquisición** | **Determinista (`id_producto` ascendente)** | Varios productos → deadlock si no se ordena |
| **Duración** | Solo validar + descontar + commit | Lock largo = cuello y más deadlock |
| **Red de seguridad** | `CHECK (cantidad_disponible >= 0)` | Si el lock falla por bug, el constraint evita stock negativo |
| **MVCC / paginación** | **No ahora** | No resuelven escritura; optimización prematura |

3. **¿Cómo se detecta y maneja una `serialization failure` (código SQLSTATE 40001)?**  
   No hay lógica de reintento automático documentada ni implementada.

4. **¿Existe bloqueo pesimista (`SELECT ... FOR UPDATE`) implementado en alguna consulta crítica?**  
   No se encuentra en el código ni en la documentación. Solo se menciona como propuesta en `05-propuestas.md`.

   repuesta: Actualmente no esta implementando

### Transacciones distribuidas y SAGA

5. **Cuando el efecto y su registro están en bases de datos diferentes (servicios autónomos), ¿qué mecanismo garantiza atomicidad?**  
   Ninguna transacción ACID transversal puede abarcar ambos servicios. Cada servicio confirma su propia transacción local. La atomicidad de la Saga se logra mediante **compensación en orden inverso**: si un paso confirma su efecto y un paso posterior falla, el orquestador invoca la operación de compensación del paso anterior (por ejemplo, `refund` en pagos, `release` en inventario, `cancel` en envíos). La consistencia es eventual, no inmediata.

6. **¿Cómo se maneja una transacción que confirma en el servicio de pagos pero el orquestador muere antes de persistir el estado en `saga_steps`?**  
   El `saga_steps` vive en la BD del servicio orquestador. Al reiniciar, el orquestador debe buscar pasos en estado `IN_PROGRESS` o `UNKNOWN` sin `finished_at` y **consultar al servicio propietario** el estado real de esa operación antes de decidir si continúa o inicia la compensación. Requiere un worker de reconciliación que se ejecute al arranque y que use claves de idempotencia para evitar duplicados al reintentar.

### Consistencia y compensación

7. **Si se compensa una reserva de inventario, ¿la liberación está en la misma transacción que la marca de compensación en `saga_steps`?**  
   No. La liberación ocurre en la BD del servicio de inventario y la marca de compensación en la BD del orquestador. Son dos transacciones locales separadas, por lo que existe una ventana de inconsistencia. Se requiere idempotencia en la liberación y un job de limpieza/reconciliación que corrija pasos huérfanos.

8. **¿Qué sucede si dos procesos intentan reservar el mismo producto al mismo tiempo y ambos pasan la validación de stock antes de escribir?**  
   La restricción `UNIQUE(order_id, product_id)` evita duplicados exactos, pero no evita que dos reservas válidas para diferentes `order_id` excedan el stock disponible sin un bloqueo sobre la fila de inventario.

---

### Transacciones en servicios autónomos (Saga distribuida)

Al separar los servicios en bases de datos independientes, desaparece la transacción única que abarque varios pasos. Cada servicio gestiona su propia transacción local y su propio nivel de aislamiento. La consistencia de la Saga se construye sobre:

- **Compensación en orden inverso**: cada paso completado tiene una operación de compensación idempotente (liberar reserva, reembolsar pago, cancelar envío).
- **Idempotencia por clave**: el orquestador envía la misma `idempotency_key` en reintentos para evitar efectos duplicados.
- **Reconciliación al arranque**: un worker consulta el estado real a cada servicio propietario para pasos en `IN_PROGRESS` o `UNKNOWN` sin `finished_at`.
- **Bloqueo pesimista solo intra-servicio**: `SELECT FOR UPDATE` solo tiene efecto dentro de la BD de un mismo servicio; no serializa reservas entre servicios distintos.

---

## Decisiones pendientes (pendiente de llenar)

| # | Pregunta | Decisión adoptada | Justificación |
|---|----------|-------------------|---------------|
| 1 | ¿Nivel de aislamiento por defecto de transacciones? | Por servicio | Cada servicio autónomo define su propio nivel de aislamiento en su propia BD. |
| 2 | ¿Se usa `SELECT FOR UPDATE` en reservas de inventario? | Solo intra-servicio | El bloqueo pesimista solo serializa dentro de la BD de un mismo servicio; no entre servicios autónomos. |
| 3 | ¿Cómo se manejan errores de serialización (40001)? | Por servicio | Cada servicio gestiona sus propios reintentos de serialización; el orquestador no los ve directamente. |
| 4 | ¿Cómo se coordina una transacción entre servicios separados? | Compensación Saga | No existe 2PC; se usa orquestación con compensaciones en orden inverso e idempotencia. |
| 5 | ¿Cómo se limpian reservas huérfanas (reservadas pero pago nunca confirmado)? | Worker de reconciliación | Al arranque, el orquestador consulta a cada servicio propietario y corrige pasos en estado intermedio. |

---

**Nota:** Este documento es un repositorio de dudas abiertas. Se rellenará a medida que se tomen decisiones de arquitectura e implementación. No contiene código ni implementación aún.

