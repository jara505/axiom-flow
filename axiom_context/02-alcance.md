# Alcance

- Crear una orden.
- Ejecutar una Saga.
- Procesar las operaciones del flujo.
- Simular fallos en diferentes etapas.
- Ejecutar compensaciones.
- Registrar el estado de la ejecución.
- Evitar operaciones duplicadas.
- Consultar el resultado de una ejecución.
- Reproducir escenarios exitosos y fallidos.

## Fuera de alcance

- Kubernetes.
- Kafka / brokers.
- Microservicios independientes desplegados.
- Integraciones reales con proveedores de pago o logística.
- Alta disponibilidad.
- Escalamiento horizontal.
- Event Sourcing.
- CQRS.
- Autenticación compleja.
- Frontend.

## Requisitos funcionales

### Gestión de pedidos

El sistema permitirá realizar funciones como:

- Crear pedidos.
- Consultar pedidos.
- Cancelar pedidos.

Estas operaciones estarán disponibles mediante una API.

### Gestión de inventario

El sistema permitirá:

- Consultar el inventario.
- Reservar inventario.
- Liberar una reserva.

### Gestión de pagos

El sistema permitirá:

- Procesar pagos asociados a un pedido.
- Consultar el estado de un pago.
- Reembolsar un pago cuando la Saga lo requiera.

### Gestión de envíos

El sistema permitirá registrar envíos asociados a un pedido.

El seguimiento de envíos se contempla como una funcionalidad futura.

### Persistencia

El sistema deberá registrar la información necesaria para mantener el estado de las operaciones.

## Requisitos no funcionales

### Rendimiento de la API

La API expuesta deberá responder en un tiempo inferior a **2 segundos**.

### Disponibilidad

La API deberá estar disponible al menos el **95 % del tiempo**.

### Seguridad

La API deberá implementar mecanismos de seguridad adecuados para proteger las operaciones y los datos.

Debido a que el proyecto se ejecutará actualmente en un entorno local, no se contempla el uso de **HTTPS/SSL** en esta etapa.
