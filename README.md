# Axiom Flow

> Orquestación de transacciones distribuidas con el **Saga Pattern** — MVP de backend y sistemas distribuidos.

[![Go](https://img.shields.io/badge/Go-1.27-00ADD8?style=for-the-badge&logo=go&logoColor=white)](https://golang.org/)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-15+-336791?style=for-the-badge&logo=postgresql&logoColor=white)](https://www.postgresql.org/)
[![Gin](https://img.shields.io/badge/Gin-Web%20Framework-00ADD8?style=for-the-badge)](https://gin-gonic.com/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Status](https://img.shields.io/badge/Status-MVP-orange)](https://github.com/)

---

## 1. ¿Qué es Axiom Flow?

**Axiom Flow** es un proyecto backend experimental diseñado para demostrar cómo gestionar **transacciones distribuidas** cuando no existe una única transacción ACID que abarque todo un proceso de negocio.

Simula un flujo de logística y envíos donde una orden requiere ejecutar varias acciones independientes —reservar inventario, procesar pago y crear envío— que en un sistema real pertenecerían a servicios distintos.

El objetivo principal es estudiar y demostrar el manejo de:

* ⚡ **Fallos parciales** — cuando algunas operaciones se completan y otras no.
* 🔄 **Compensaciones (Saga)** — deshacer efectos de operaciones ya realizadas ante un fallo.
* 🛡️ **Idempotencia** — evitar efectos duplicados ante reintentos o solicitudes repetidas.
* 📊 **Estado de ejecución** — persistir y consultar el progreso de una Saga de larga duración.
* 🧪 **Escenarios de fallo** — reproducir y verificar comportamiento ante interrupciones.


## 2. Caso de uso principal

Una orden debe completar tres operaciones secuenciales:

```
Crear orden
     |
     v
Reservar inventario
     |
     v
Procesar pago
     |
     v
Crear envío
     |
     v
Orden completada
```

Si el pago se aprueba pero el envío falla, la Saga ejecuta compensaciones en orden inverso:

```
Crear envío      ✗
Procesar pago    → Reembolso
Reservar inventario → Liberar inventario
```

Resultado: la Saga queda identificada como **compensada**, sin efectos colaterales duplicados.


## 3. Stack tecnológico

| Capa | Tecnología | Propósito |
|------|-----------|-----------|
| **Lenguaje** | Go 1.27 | Backend, concurrencia con goroutines, binario estático. |
| **Base de datos** | PostgreSQL | Modelo relacional con restricciones de idempotencia (`UNIQUE`), transacciones ACID y update atómico. |
| **Driver SQL** | `pgx/v5` | Driver nativo de PostgreSQL para Go. |
| **Capa de acceso** | `sqlx` | Mapeo de structs y consultas nombradas sobre `database/sql`. |
| **Framework HTTP** | Gin | API REST ligera y de alto rendimiento. |
| **Identificadores** | `google/uuid` | Generación de `idempotency_key` y `operation_key`. |
| **Fake de pago** | Implementación propia | Simulador de proveedor de pagos con resultados forzables y consulta por clave (reconciliación). |
| **Testing** | `testify` | Pruebas unitarias y de integración. |
| **Logging** | `log/slog` | Logging estructurado (estándar de Go 1.27). |

> Nota: el proyecto contempla Docker/Docker Compose para entorno local, pero actualmente se entrega como código fuente listo para compilar y ejecutar.

