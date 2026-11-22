# Stack tecnológico — Axiom

Este documento define la propuesta de stack para la implementación del walking skeleton de la Saga de pedidos.

> Estado: propuesta de arquitectura. No hay código fuente implementado aún en el repositorio `axiom/`.

---

## 1. Lenguaje de programación

| Componente | Decisión | Justificación |
|------------|----------|---------------|
| Lenguaje base | **Go 1.27** | Estándar para servicios de backend, buen rendimiento, concurrencia nativa con goroutines, tipado estático y binario estático. |
| Contratos de documentación | TypeScript | Los contratos en `04-casos-de-uso.md` están en TypeScript como especificación, pero la implementación será en Go. |

---

## 2. Base de datos

| Componente | Decisión | Justificación |
|------------|----------|---------------|
| Motor | **PostgreSQL** | Coherente con el DDL de `03-dominio.md` (`SERIAL`, `TIMESTAMP`, `FOREIGN KEY`, `CHECK`, `UNIQUE`). Soporta transacciones ACID necesarias para la Saga y bloqueos pesimistas. |
| Driver | `github.com/jackc/pgx/v5` | Driver nativo de PostgreSQL para Go, alto rendimiento, soporte nativo para tipos de PostgreSQL y contexto. |
| Capa de acceso | `github.com/jmoiron/sqlx` | Extiende `database/sql` con mapeo estructos (`struct`) y consultas nombradas, sin abstraer completamente el SQL (keeps control sobre restricciones de idempotencia y update atómico). |

---

## 3. Capa web / API

| Componente | Decisión | Justificación |
|------------|----------|---------------|
| Framework HTTP | `github.com/gin-gonic/gin` | Framework ligero y popular para Go, buen rendimiento, enrutamiento sencillo, middlewares y manejo de JSON. |
| Alternativa considerada | `net/http` (estándar) | Suficiente para un walking skeleton, pero Gin acelera el desarrollo de rutas y middlewares. |
| Puerto | `8080` (configurable via `PORT`) | Puerto común para contenedores y servicios Cloud. |
| Variables de entorno | `github.com/joho/godotenv` | Carga `.env` en desarrollo local. |

---

## 4. Identificadores y utilidades

| Componente | Decisión | Justificación |
|------------|----------|---------------|
| UUID | `github.com/google/uuid` | Generación de `idempotency_key` y `operation_key` para la Saga y los pasos. |
| Logging | `log/slog` (estándar Go 1.27) | Logging estructurado integrado en la librería estándar. |
| Validación | Manual / `go-playground/validator` | Validación de cuerpos de request y reglas de negocio. Para el walking skeleton se empieza con validación manual y se migra a `validator` si crece. |

---

## 5. Proveedor de pagos (fake)

| Componente | Decisión | Justificación |
|------------|----------|---------------|
| Fake de pago | Implementación propia en Go | Implementa la interfaz `PaymentProvider` definida en `04-casos-de-uso.md`. Permite forzar resultados (`APROBADO`, `RECHAZADO`, `DESCONOCIDO`), simular latencia y consultar por `operation_key` para reconciliación. |
| Proveedor real | Fuera de alcance del MVP | Según `02-alcance.md`, integraciones reales con proveedores de pago quedan fuera del alcance inicial. |

---

## 6. Testing

| Componente | Decisión | Justificación |
|------------|----------|---------------|
| Framework de tests | `github.com/stretchr/testify` | Assertions y mocks estándar en el ecosistema Go. |
| Pruebas de integración | Testcontainers para PostgreSQL | Permite ejecutar pruebas contra una instancia real de PostgreSQL sin depender de una base instalada localmente. |
| Prueba del crash | Caso específico documentado en `04-casos-de-uso.md` (sección "Prueba del crash") | Se implementará como test de integración que mata el proceso entre commit del pago y persistencia en `SagaPaso`. |

---

## 7. Resumen de dependencias de Go

Módulo propuesto: `axiom/implementation`

```
require (
    github.com/gin-gonic/gin v1.10.0
    github.com/jackc/pgx/v5 v5.5.4
    github.com/jmoiron/sqlx v1.3.5
    github.com/joho/godotenv v1.5.1
    github.com/google/uuid v1.6.0
    github.com/stretchr/testify v1.9.0
)
```

Dependencias indirectas gestionadas por `go mod tidy`.

---

## 8. Estructura de directorios propuesta

```
axiom/
├── implementation/
│   ├── go.mod
│   ├── go.sum
│   ├── cmd/
│   │   └── server/
│   │       └── main.go
│   ├── internal/
│   │   ├── domain/         # Modelos, errores, interfaces
│   │   ├── db/             # Conexión a PostgreSQL
│   │   ├── repository/     # Acceso a datos (Pedido, Reserva, Pago, Envio, SagaPaso)
│   │   ├── service/        # Orquestador de Saga, FakePaymentProvider
│   │   └── handler/        # Handlers HTTP (POST /pedidos, GET /pedidos/{id}/saga)
│   └── migrations/
│       └── 001_initial.sql
```

---

## 9. Decisiones no tomadas (fuera de alcance inicial)

- **Mensajería:** No se usa Kafka ni brokers (`02-alcance.md` lo excluye).
- **Microservicios:** Monolito modular en un solo binario.
- **Contenedores:** Docker se deja para una etapa posterior.
- **Autenticación:** No se contempla en el MVP.
- **Frontend:** Queda fuera de alcance (`02-alcance.md`).

---

## 10. Relación con documentos existentes

- `03-dominio.md`: Modelo de datos SQL que se ejecuta en PostgreSQL.
- `04-casos-de-uso.md`: Contratos TypeScript que se traducen a interfaces y structs de Go.
- `08-guion-demo.md`: Escenarios que se ejecutan contra el FakePaymentProvider.
- `07-arquitectura.md`: Guía de preguntas que este stack busca responder.
