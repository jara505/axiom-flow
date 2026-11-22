# Axiom Flow

## 1. Descripción

Axiom Flow es un proyecto backend experimental orientado a la orquestación de transacciones distribuidas mediante el **Saga Pattern**.

El proyecto simula un flujo de logística y envíos en el que una operación de negocio requiere ejecutar varias acciones independientes, como reservar inventario, procesar un pago y crear un envío.

El objetivo principal es estudiar cómo gestionar este tipo de operaciones cuando no existe una única transacción ACID que abarque todo el proceso.

Axiom Flow se plantea inicialmente como un **MVP pequeño y autocontenido**, con suficiente complejidad para demostrar conceptos de backend y sistemas distribuidos sin convertirse en una plataforma empresarial completa.

---

## 2. Objetivo

El objetivo del proyecto es construir un pequeño motor capaz de:

- Ejecutar una secuencia de operaciones.
- Detectar cuándo una operación falla.
- Mantener el estado de la ejecución.
- Ejecutar acciones compensatorias cuando sea necesario.
- Evitar ejecuciones duplicadas mediante idempotencia.
- Permitir reproducir distintos escenarios de fallo.
- Persistir la información necesaria para consultar el resultado de una ejecución.

El proyecto está pensado principalmente como una demostración práctica de conocimientos de backend y sistemas distribuidos.

---

## 3. Caso de uso

El escenario utilizado para el MVP será un flujo simplificado de procesamiento de una orden.

Una orden requiere completar tres operaciones:

1. Reservar inventario.
2. Procesar el pago.
3. Crear el envío.

En condiciones normales:

```text
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

Cada una de estas operaciones representa una acción que, en un sistema real, podría pertenecer a un servicio independiente.

Para el MVP, estas operaciones serán simuladas localmente.

---

## 4. Manejo de fallos

El proyecto debe ser capaz de representar fallos parciales.

Por ejemplo, si el inventario y el pago se procesan correctamente pero la creación del envío falla:

```text
Reservar inventario     ✓
Procesar pago           ✓
Crear envío             ✗
```

El sistema deberá ejecutar las acciones necesarias para compensar las operaciones que ya habían sido realizadas:

```text
Crear envío             ✗
Procesar pago           → Reembolso
Reservar inventario     → Liberar inventario
```

El resultado esperado será que la Saga quede identificada como compensada.

También se contemplan escenarios donde una compensación falla. Esto permite representar situaciones en las que el sistema no puede completar automáticamente la recuperación.

---

## 5. Idempotencia

Las operaciones del sistema deben contemplar la posibilidad de recibir solicitudes duplicadas.

El MVP utilizará **Redis** para implementar mecanismos básicos de idempotencia.

El objetivo es evitar que una misma operación pueda ejecutarse accidentalmente más de una vez debido a reintentos o solicitudes duplicadas.



## 6. Simulación de servicios externos

Para mantener el proyecto pequeño y autocontenido, las operaciones que normalmente dependerían de otros sistemas serán simuladas.

El MVP contará con componentes que representen servicios externos como:

- Servicio de inventario.
- Servicio de pagos.
- Servicio de envíos.

Estos componentes podrán responder correctamente o simular diferentes tipos de fallo.

Por ejemplo:

```text
Inventory Service
    └── reservar inventario → éxito / fallo

Payment Service
    └── procesar pago → éxito / fallo

Shipping Service
    └── crear envío → éxito / fallo
```

La simulación permitirá probar diferentes escenarios sin depender de APIs externas reales.

---

## 7. Stack

El MVP utilizará las siguientes tecnologías:

- **Go** — lenguaje principal.
- **PostgreSQL** — persistencia de órdenes y ejecuciones.
- **Redis** — mecanismos de idempotencia.
- **Docker** — ejecución de los componentes del proyecto.
- **Docker Compose** — entorno local de desarrollo.

El stack podrá evolucionar posteriormente si las necesidades del proyecto lo justifican.

---

## 8. Alcance del MVP

El MVP estará limitado a un flujo principal de procesamiento de órdenes.

Debe permitir:

- Crear una orden.
- Ejecutar una Saga.
- Procesar las operaciones del flujo.
- Simular fallos en diferentes etapas.
- Ejecutar compensaciones.
- Registrar el estado de la ejecución.
- Evitar operaciones duplicadas.
- Consultar el resultado de una ejecución.
- Reproducir escenarios exitosos y fallidos.

El objetivo es que el proyecto sea pequeño, ejecutable y fácil de demostrar.

---

## 9. Fuera del alcance inicial

Para mantener el proyecto enfocado, inicialmente no se contemplan:

- Kubernetes.
- Apache Kafka.
- Microservicios desplegados de forma independiente.
- Integraciones reales con proveedores de pago.
- Integraciones reales con empresas de logística.
- Alta disponibilidad.
- Balanceo de carga.
- Escalado horizontal.
- Event Sourcing.
- CQRS.
- Sistemas de autenticación complejos.
- Interfaces frontend.

Estos elementos podrían evaluarse posteriormente si aportan valor al proyecto.

---

## 10. Resultado esperado

Al finalizar el MVP debería ser posible ejecutar una orden y observar claramente qué ocurrió durante todo su ciclo de vida.

Por ejemplo:

```text
Order: ORD-123

1. Inventory reservation   ✓
2. Payment processing      ✓
3. Shipping creation       ✗
4. Payment compensation    ✓
5. Inventory compensation  ✓

Result: COMPENSATED
```

También debería ser posible consultar el estado de una ejecución que haya terminado correctamente:

```text
Order: ORD-456

1. Inventory reservation   ✓
2. Payment processing      ✓
3. Shipping creation       ✓

Result: COMPLETED
```

El resultado debe permitir entender fácilmente cómo respondió el sistema ante cada escenario.

---

## 11. Evolución posterior

Una vez completado el MVP, el proyecto podrá evolucionar de acuerdo con los problemas que aparezcan durante su desarrollo.

Algunas posibles extensiones son:

- Agregar nuevos tipos de operaciones.
- Incorporar reintentos.
- Mejorar el manejo de compensaciones fallidas.
- Agregar timeouts.
- Incorporar observabilidad.
- Ejecutar operaciones de forma asíncrona.
- Separar componentes en servicios independientes.
- Incorporar un broker de mensajes.
- Agregar pruebas de escenarios más complejos.
- Exponer métricas sobre las ejecuciones.

Estas extensiones no forman parte del objetivo inicial y solo se incorporarían cuando exista una razón técnica para hacerlo.

---

## 12. Propósito del proyecto

Axiom Flow es principalmente un proyecto de aprendizaje y demostración enfocado en backend y sistemas distribuidos.

La intención no es construir una plataforma de orquestación empresarial completa, sino desarrollar un MVP pequeño que permita experimentar con problemas reales relacionados con:

- Transacciones distribuidas.
- Fallos parciales.
- Compensaciones.
- Idempotencia.
- Consistencia eventual.
- Recuperación ante errores.
- Estado de procesos de larga duración.

El proyecto servirá como base para explorar posteriormente decisiones de arquitectura más complejas a medida que aparezcan nuevas necesidades.
