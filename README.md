# Axiom Flow

Orquestación de transacciones distribuidas con el patrón Saga para flujos de orden, inventario, pago y envío.

![Go](https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white)
![Estado](https://img.shields.io/badge/Estado-MVP-orange)

## Instalación

Requisitos:

- Go 1.27
- PostgreSQL

```bash
git clone https://github.com/jara505/axiom-flow.git
cd axiom-flow
go mod download
```

## Uso

```bash
cp .env.example .env
go run cmd/server/main.go
```

## Configuración

El proyecto requiere variables de entorno. Consulta el archivo [`.env.example`](.env.example) para ver la plantilla completa.

Variables principales:

**App**
- `APP_PORT`
- `APP_ENV`

**Base de datos**
- `DB_HOST`
- `DB_PORT`
- `DB_USER`
- `DB_PASSWORD`
- `DB_NAME`
- `DB_SSL_MODE`

**PayPal Sandbox**
- `PAYPAL_CLIENT_ID`
- `PAYPAL_CLIENT_SECRET`
- `PAYPAL_BASE_URL`

## Licencia

Copyright (c) 2026. Todos los derechos reservados. Este software es de propiedad privada. No se concede licencia para uso, copia, modificación o distribución sin autorización explícita por escrito del autor.
