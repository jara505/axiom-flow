# Analisis del dominio

## Entidades

```sql
-- Entidades y sus atributos (beta)

CREATE TABLE Cliente (
    id_cliente SERIAL PRIMARY KEY,
    nombre VARCHAR(150) NOT NULL,
    email VARCHAR(150) NOT NULL UNIQUE,
    fecha_registro TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE Producto (
    id_producto SERIAL PRIMARY KEY,
    sku VARCHAR(50) NOT NULL UNIQUE,
    nombre VARCHAR(100) NOT NULL,
    descripcion TEXT,
    marca VARCHAR(50),
    precio_base DECIMAL(10, 2) NOT NULL,
    is_active BOOLEAN DEFAULT TRUE NOT NULL
);

-- Gestión logística y disponibilidad física (1:1 con Producto para el MVP)
CREATE TABLE Inventario (
    id_inventario SERIAL PRIMARY KEY,
    id_producto INTEGER NOT NULL UNIQUE,
    cantidad_disponible INTEGER NOT NULL DEFAULT 0,
    stock_minimo INTEGER NOT NULL DEFAULT 0,
    stock_maximo INTEGER NOT NULL DEFAULT 1000,
    FOREIGN KEY (id_producto) REFERENCES Producto(id_producto) ON DELETE RESTRICT
);

CREATE TABLE Pedido (
    id_pedido SERIAL PRIMARY KEY,
    id_cliente INTEGER NOT NULL,
    numero_pedido VARCHAR(50) NOT NULL UNIQUE,
    idempotency_key VARCHAR(100) NOT NULL UNIQUE,
    estado_pedido VARCHAR(30) NOT NULL, -- PENDIENTE_PAGO, PAGADO, CANCELADO, ENVIADO
    monto_subtotal DECIMAL(10, 2) NOT NULL,
    monto_total DECIMAL(10, 2) NOT NULL,
    fecha_creacion TIMESTAMP DEFAULT CURRENT_TIMESTAMP NOT NULL,
    FOREIGN KEY (id_cliente) REFERENCES Cliente(id_cliente) ON DELETE RESTRICT
);

-- Mantenemos el detalle de la orden con el precio congelado
CREATE TABLE DetallesProducto (
    id_detalles_producto SERIAL PRIMARY KEY,
    id_pedido INTEGER NOT NULL,
    id_producto INTEGER NOT NULL,
    cantidad INTEGER NOT NULL CHECK (cantidad > 0),
    precio_unitario DECIMAL(10, 2) NOT NULL, -- Congela el precio base al momento de la compra
    FOREIGN KEY (id_pedido) REFERENCES Pedido(id_pedido) ON DELETE CASCADE,
    FOREIGN KEY (id_producto) REFERENCES Producto(id_producto) ON DELETE RESTRICT
);

CREATE TABLE ReservaInventario (
    id_reserva SERIAL PRIMARY KEY,
    id_pedido INTEGER NOT NULL,
    id_producto INTEGER NOT NULL,
    cantidad INTEGER NOT NULL CHECK (cantidad > 0),
    estado VARCHAR(20) NOT NULL DEFAULT 'RESERVADO' CHECK (estado IN ('RESERVADO', 'LIBERADO')),
    fecha_reserva TIMESTAMP DEFAULT CURRENT_TIMESTAMP NOT NULL,
    fecha_liberacion TIMESTAMP,
    FOREIGN KEY (id_pedido) REFERENCES Pedido(id_pedido) ON DELETE RESTRICT,
    FOREIGN KEY (id_producto) REFERENCES Producto(id_producto) ON DELETE RESTRICT,
    UNIQUE (id_pedido, id_producto),
    -- Regla de update atómico: el estado solo puede transitar RESERVADO → LIBERADO
    -- dentro de la misma transacción local que valida stock y afecta la fila.
);

-- Transaccionalidad de pagos (Soporta reintentos si una transacción SAGA falla)
CREATE TABLE Pago (
    id_pago SERIAL PRIMARY KEY,
    id_pedido INTEGER NOT NULL,
    idempotency_key VARCHAR(100) NOT NULL UNIQUE,
    monto DECIMAL(10, 2) NOT NULL,
    moneda VARCHAR(3) DEFAULT 'USD' NOT NULL,
    metodo_pago VARCHAR(30) NOT NULL,
    proveedor_pago VARCHAR(50) NOT NULL, -- Stripe, PayPal, Simulación
    estado_pago VARCHAR(20) NOT NULL,    -- PENDIENTE, APROBADO, RECHAZADO, REEMBOLSADO, DESCONOCIDO
    fecha_pago TIMESTAMP DEFAULT CURRENT_TIMESTAMP NOT NULL,
    FOREIGN KEY (id_pedido) REFERENCES Pedido(id_pedido) ON DELETE CASCADE
);

CREATE TABLE Envio (
    id_envio SERIAL PRIMARY KEY,
    id_pedido INTEGER NOT NULL UNIQUE,
    estado_envio VARCHAR(20) NOT NULL DEFAULT 'PENDIENTE' CHECK (estado_envio IN ('PENDIENTE', 'CREADO', 'FALLIDO')),
    direccion VARCHAR(255) NOT NULL,
    fecha_creacion TIMESTAMP DEFAULT CURRENT_TIMESTAMP NOT NULL,
    fecha_envio TIMESTAMP,
    FOREIGN KEY (id_pedido) REFERENCES Pedido(id_pedido) ON DELETE RESTRICT
);

CREATE TABLE SagaPaso (
    id_saga_paso SERIAL PRIMARY KEY,
    id_pedido INTEGER NOT NULL,
    nombre_paso VARCHAR(50) NOT NULL CHECK (nombre_paso IN ('CREAR_PEDIDO', 'RESERVAR_INVENTARIO', 'PROCESAR_PAGO', 'CREAR_ENVIO', 'REEMBOLSAR_PAGO', 'LIBERAR_RESERVA')),
    estado VARCHAR(20) NOT NULL DEFAULT 'PENDIENTE' CHECK (estado IN ('PENDIENTE', 'EN_CURSO', 'COMPLETADO', 'FALLIDO', 'DESCONOCIDO')),
    intento INTEGER NOT NULL DEFAULT 1,
    idempotency_key VARCHAR(100) NOT NULL UNIQUE,
    operation_key VARCHAR(100) NOT NULL UNIQUE,
    fecha_inicio TIMESTAMP DEFAULT CURRENT_TIMESTAMP NOT NULL,
    fecha_fin TIMESTAMP,
    FOREIGN KEY (id_pedido) REFERENCES Pedido(id_pedido) ON DELETE RESTRICT,
    UNIQUE (id_pedido, nombre_paso)
);
```

## Casos de uso y como interactuan

> Podemos encontrar mas info en [[04-casos-de-uso|use_cases.md]]
