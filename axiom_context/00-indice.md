# Axiom — Índice del proyecto

Documentación de arquitectura del flujo de pedidos: Saga, idempotencia y compensaciones.

## Orden de lectura

1. [[01-contexto]] — problema y fallos posibles.
2. [[02-alcance]] — alcance, requisitos funcionales y no funcionales.
3. [[03-dominio]] — entidades del modelo de datos.
4. [[04-casos-de-uso]] — escenarios de fallo y contratos de componentes.
5. [[05-propuestas]] — soluciones, trade-offs y fallbacks por fallo.
6. [[06-justificacion]] — justificación de las soluciones y alternativas descartadas.
7. [[07-arquitectura]] — guía de preguntas para cerrar la arquitectura.
8. [[08-guion-demo]] — guion de demostración con cinco escenarios de la Saga.
9. [[09-stack-tecnologico]] — propuesta de stack tecnológico (Go, PostgreSQL, dependencias).

## Mapa fallo → propuesta → justificación

| Fallo ([[01-contexto]]) | Propuesta ([[05-propuestas]]) | Justificación ([[06-justificacion]]) |
|---|---|---|
| 1. Fallos parciales | 1. Rollback local · 4. Persistencia y recuperación | 1. Estado y reanudación |
| 2. Operaciones duplicadas (incluye pago duplicado) | 2. Idempotencia · 2.1 Control del estado del pago | — |
| 3. Concurrencia en inventario | 3. Lock pesimista | — |
| 4. Cancelación con efectos | Sin propuesta propia; cubierto por compensaciones | 2. Acciones compensatorias |
| 5. Pedido sin condiciones | 5. Validación + reserva atómica | 5. Validación y reserva |
| 6. Interrupción | 4. Persistencia y recuperación | 3. Recuperación · 4. Identificación |

## Pendientes

- El fallo 4 (cancelación después de producir efectos) no tiene propuesta propia.
- La tabla de compensaciones tiene 3 filas: confirmar si está completa.
- [[07-arquitectura]] es una guía metodológica, no documentación del proyecto.
