# Entender el problema

## Problema

Tenemos un flujo de logística que contiene procesos base como:

* Creación de pedidos.
* Procesamiento de pagos.
* Reserva de inventario.
* Creación de envíos.

Estas operaciones forman parte de un mismo proceso de negocio, pero pueden ocurrir fallos durante su ejecución. Un fallo parcial puede provocar que algunas operaciones se completen mientras otras no, dejando el proceso en un estado incompleto o inconsistente.

Cuando una operación falla después de que otras hayan producido efectos, pueden quedar efectos parciales que no corresponden con el resultado final del proceso.

---

## Fallos posibles

### 1. Fallos parciales

Una operación puede ejecutarse correctamente, pero producirse un error posteriormente durante el proceso.

```text
Crear pedido        ✓
Reservar inventario ✓
Procesar pago       ✓
Crear envío         ✗
```

El proceso queda incompleto a pesar de que algunas operaciones ya fueron ejecutadas.

### 2. Operaciones duplicadas

El mismo proceso u operación puede ejecutarse dos o más veces.

Esto puede provocar efectos duplicados sobre los recursos involucrados.

#### Pago duplicado

Un pago puede procesarse más de una vez, provocando que se cobre un monto superior al que correspondía originalmente.

### 3. Concurrencia en inventario

Varios usuarios pueden intentar reservar el mismo inventario al mismo tiempo.

Esto puede provocar inconsistencias en el estado de disponibilidad del inventario.

### 4. Cancelación después de producir efectos

Un pedido puede producir efectos sobre otros recursos y posteriormente ser cancelado.

Esto puede dejar efectos que ya no corresponden con el estado final del pedido.

### 5. Pedido sin condiciones necesarias

Un pedido puede intentar procesarse cuando el producto no existe o no existe suficiente inventario disponible.

Esto puede provocar que el proceso represente una operación que realmente no puede cumplirse.

### 6. Interrupción durante la ejecución

Puede producirse un fallo del servidor, una interrupción del proceso o un apagón mientras el flujo está siendo ejecutado.

Esto puede provocar que el estado final de una operación quede desconocido y que el proceso permanezca incompleto.
