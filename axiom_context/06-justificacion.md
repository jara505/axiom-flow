# Selección y justificación de soluciones

## 1. Estado y reanudación de la Saga

| Elemento | Descripción |
|---|---|
| **Solución seleccionada** | Ver [[05-propuestas#4. Interrupciones y fallos durante la ejecución]]. |
| **Justificación** | El estado del orquestador puede no reflejar el resultado real de una operación. Registrar el progreso y verificar los resultados permite determinar qué ocurrió antes de reanudar la Saga. |

**Alternativa considerada — Confiar únicamente en los estados del orquestador**

Registrar estados como `PENDIENTE`, `COMPLETADO` o `FALLIDO` para identificar el progreso.

- **Limitación:** una operación puede completarse sin que el orquestador llegue a registrar su resultado. Por tanto, el estado por sí solo no siempre permite conocer lo ocurrido.

## 2. Acciones compensatorias

| Elemento | Descripción |
|---|---|
| **Solución seleccionada** | Ver [[05-propuestas#4. Interrupciones y fallos durante la ejecución]]. |
| **Justificación** | Como no existe una transacción global que revierta todos los efectos, las compensaciones permiten gestionar fallos posteriores a operaciones ya completadas. |

**Consideración:** las compensaciones no garantizan una reversión exacta y también pueden fallar.

## 3. Recuperación ante interrupciones

| Elemento | Descripción |
|---|---|
| **Solución seleccionada** | Ver [[05-propuestas#Interrupciones inesperadas]]. |
| **Justificación** | El MVP busca experimentar con la recuperación de una Saga, no garantizar la continuidad de un servicio de producción. Por ello, la alta disponibilidad queda fuera del alcance inicial. |

## 4. Identificación y verificación de operaciones

| Elemento | Descripción |
|---|---|
| **Solución seleccionada** | Ver [[05-propuestas#Resultados desconocidos]]. |
| **Justificación** | Identificar y consultar cada operación permite tomar decisiones basadas en su resultado, reduciendo el riesgo de duplicar operaciones completadas o compensar las incorrectas. |

## 5. Validación y reserva de inventario

| Elemento | Descripción |
|---|---|
| **Solución seleccionada** | Ver [[05-propuestas#5. Pedido sin condiciones necesarias]]. |
| **Justificación** | La disponibilidad puede cambiar entre la validación inicial y la reserva. Por ello, la reserva debe comprobar y asignar el inventario de forma segura para evitar superar las existencias disponibles. |

### Alternativa considerada — Validar la disponibilidad del servicio

Comprobar si un servicio está disponible antes de invocarlo.

- **Limitación:** el servicio puede fallar después de la comprobación o producir un error durante la operación. La disponibilidad previa no garantiza que esta se complete correctamente.
