# Axiom Flow

Orquestación de transacciones distribuidas con el patrón **Saga** para flujos de orden, inventario, pago y envío. Sistema backend en Go con PostgreSQL, consistencia fuerte por servicio y compensación automática ante fallos.

![Go](https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white)
![PostgreSQL](https://img.shields.io/badge/PostgreSQL-15-336791?logo=postgresql&logoColor=white)
![Docker](https://img.shields.io/badge/Docker-Ready-2496ED?logo=docker&logoColor=white)
![Patrón Saga](https://img.shields.io/badge/Patr%C3%B3n-Saga-ff6b35)

---

## Descripción

Axiom Flow procesa pedidos mediante una Saga orquestada: cada orden atraviesa los pasos de pago, reserva de inventario y envío. Si alguno falla, el orquestador ejecuta compensaciones en orden inverso (liberar inventario, reembolsar pago, cancelar envío) para mantener la consistencia del sistema.

Cada microservicio es autónomo y posee su propia base de datos PostgreSQL, eliminando acoplamiento a nivel de datos y permitiendo extracción independiente en el futuro.

---

## Arquitectura

[diagram.png](diagram.png)

---

## Decisiones de diseño y consistencia

Las protecciones de consistencia implementadas (transacciones atómicas, `SELECT FOR UPDATE`, `idempotency_key` con constraint `UNIQUE`, patrón Outbox y circuit breaker) están documentadas en detalle en [`architectural_decisions/README.md`](architectural_decisions/README.md#4-protecciones-transaccionales-y-de-consistencia).

---

## Stack tecnológico

- **Lenguaje:** Go 1.27
- **Base de datos:** PostgreSQL (una por servicio)
- **Monolito HTTP:** Gin
- **Microservicios:** `net/http` estándar
- **Driver SQL:** `lib/pq`, `sqlx`
- **Variables de entorno:** `godotenv`
- **Contenedores:** Docker

---

## API Documentada

Cada servicio expone Swagger UI listo para explorar sin instalar nada:

| Servicio | Puerto | Swagger UI |
|----------|--------|------------|
| Monolito (órdenes + Saga) | 8080 | `http://localhost:8080/docs` |
| Inventario | 8082 | `http://localhost:8082/docs` |
| Envíos | 8081 | `http://localhost:8081/docs` |

La especificación OpenAPI 3.0 completa está en `api/openapi.yaml` y puede importarse en Postman, Insomnia o Swagger Editor.

---

## Cómo ejecutar

```bash
# Clonar el repositorio
git clone https://github.com/jara505/axiom-flow.git
cd axiom-flow

# Configurar variables de entorno
cp .env.example .env
# Editar .env con tus credenciales de PostgreSQL

# Ejecutar el monolito (puerto 8080)
go run ./cmd/server/

# En terminales separadas, ejecutar microservicios:
go run ./services/inventory/main.go   # puerto 8082
go run ./services/shipment/main.go    # puerto 8081
```

Cada servicio inicializa su esquema automáticamente al arrancar (`initSchema`), por lo que no es necesario aplicar migraciones manuales en primera ejecución.

---

## Variables de entorno

| Variable | Descripción | Valor por defecto |
|----------|-------------|-------------------|
| `PORT` / `APP_PORT` | Puerto del servicio | 8080 / 8081 / 8082 |
| `DB_HOST` | Host de PostgreSQL | `localhost` |
| `DB_PORT` | Puerto de PostgreSQL | `5432` |
| `DB_USER` | Usuario de PostgreSQL | `postgres` |
| `DB_PASSWORD` | Contraseña de PostgreSQL | `postgres` |
| `DB_NAME` | Nombre de la base de datos | `axiom_flow` |
| `DB_SSL_MODE` | Modo SSL de PostgreSQL | `disable` |
| `PAYMENT_SERVICE_URL` | URL del servicio de pago | `http://localhost:8083` |
| `SHIPMENT_REMOTE_URL` | URL del servicio de envíos | `http://localhost:8081` |
| `INVENTORY_REMOTE_URL` | URL del servicio de inventario | `http://localhost:8082` |

Consulta `.env.example` para la plantilla completa.

---

## Licencia

Copyright (c) 2026. Todos los derechos reservados. Este software es de propiedad privada. No se concede licencia para uso, copia, modificación o distribución sin autorización explícita por escrito del autor.
