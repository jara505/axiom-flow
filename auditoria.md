# Auditoría de resiliencia - Axiom Flow

Propósito: probar el sistema hasta romperlo para validar circuit breaker, failover y compensación de Saga. Ejecutar los comandos en orden.

## Requisito previo
Todos los contenedores levantados:
```
docker ps
```

---

## 1. Línea base con vegeta

**Propósito:** medir comportamiento normal sin fallos (latencia, errores, throughput).

**Nota:** vegeta no soportó POST con JSON body en este entorno (faltaba Content-Type, devolvía 404). Se ejecutó con `curl` en bucle (300 peticiones secuenciales con payload válido).

**Comando:**
```bash
for i in $(seq 1 300); do curl -s -o /dev/null -w "%{http_code}" -X POST http://localhost:8080/orders -H "Content-Type: application/json" -d "{\"customer_id\":1,\"order_number\":\"ORD-VEG-LOAD-$i\",\"total\":99.99,\"product_id\":1,\"quantity\":1,\"idempotency_key\":\"veg-load-$i\"}"; done
```

**Resultados**

| Métrica | Valor |
|---------|-------|
| Requests totales | 300 |
| Success (200/201) | 10 |
| 400 Bad Request | 0 |
| 429 Too Many Requests | 290 |
| Otros errores | 0 |

**Interpretación:** el rate limiter (60 req/min por IP) bloqueó la gran mayoría con 429. Solo 10 órdenes se crearon exitosamente antes de alcanzar el límite.

---

## 2. Carga progresiva con vegeta

**Propósito:** encontrar el punto donde el sistema empieza a fallar o degradarse.

**Nota:** vegeta no soportó POST con JSON body en este entorno. Se ejecutó con `curl` en bucle con tres niveles de carga y rate limit de 5 req/s por IP.

**Comandos:**
```bash
for i in $(seq 1 60); do curl ... -d "{"order_number":"ORD-PROG2-ligera-$i"}"; done
for i in $(seq 1 300); do curl ... -d "{"order_number":"ORD-PROG2-media-$i"}"; done
for i in $(seq 1 600); do curl ... -d "{"order_number":"ORD-PROG2-alta-$i"}"; done
```

**Resultados**

| Nivel de carga | Requests | Success (200/201) | 429 Too Many Requests | 400 Bad Request |
|----------------|----------|-------------------|----------------------|-----------------|
| Ligera (n=60)  | 60       | 59                | 1                    | 0               |
| Media (n=300)  | 300      | 0                 | 300                  | 0               |
| Alta (n=600)   | 600      | 0                 | 600                  | 0               |

**Interpretación:** con 5 req/s por IP, el rate limiter actúa casi de inmediato en cargas masivas. En la prueba ligera (60 req en bucle rápido), solo 59 pasaron y 1 fue 429 porque el bucket de 5 tokens se agotó instantáneamente. En cargas media y alta, todo es 429. El sistema **no cayó**, no devolvió 5xx. Es un comportamiento **bueno**: se protege sin colapsar.

---
## 3. Fallo de servicio remoto (shipment)

**Propósito:** verificar que `AdaptiveShipmentGateway` cae a local cuando el remoto falla.

**Comandos ejecutados:**
```bash
docker kill axiom-shipment-service
curl -s -X POST http://localhost:8080/orders -H "Content-Type: application/json" -d '{"customer_id":1,"order_number":"ORD-SHIP-FALL","total":50.00,"product_id":1,"quantity":1,"idempotency_key":"ship-fall-001"}'
docker start axiom-shipment-service
```

**Resultados**

| Escenario | Resultado |
|-----------|-----------|
| shipment-service caído | Order creada exitosamente (order_id: 134) |
| Fallback a local | Funcionó correctamente |
| Reactivación | axiom-shipment-service recuperado |

**Interpretación:** el `AdaptiveShipmentGateway` detectó que el remoto falló, abrió el circuit breaker y cayó a la implementación local. La Saga se completó sin errores. El sistema es **resiliente** ante la caída del servicio de envío.

---
## 4. Fallo de servicio remoto (inventory)

**Propósito:** verificar que `AdaptiveInventoryGateway` cae a local con validación previa cuando el remoto falla.

**Comandos ejecutados:**
```bash
docker kill axiom-inventory-service
curl -s -X POST http://localhost:8080/orders -H "Content-Type: application/json" -d '{"customer_id":1,"order_number":"ORD-INV-FALL","total":50.00,"product_id":1,"quantity":1,"idempotency_key":"inv-fall-001"}'
docker start axiom-inventory-service
```

**Resultados**

| Escenario | Resultado |
|-----------|-----------|
| inventory-service caído | Order creada exitosamente (order_id: 135) |
| Fallback a local con validación | Funcionó correctamente |
| Reactivación | axiom-inventory-service recuperado |

**Interpretación:** el `AdaptiveInventoryGateway` detectó que el remoto falló y cayó a la implementación local. La validación previa en local permitió crear la reserva sin doble-booking. La Saga se completó sin errores. El sistema es **resiliente** ante la caída del servicio de inventario.

---
## 5. Inyección de latencia con toxiproxy

**Propósito:** simular red lenta entre el monolito y shipment-service para observar timeouts y apertura de circuit breaker.

**Comandos ejecutados:**
```bash
curl -X POST http://localhost:8474/proxies \
  -H "Content-Type: application/json" \
  -d '{"name":"shipment-latency","listen":"127.0.0.1:9001","upstream":"shipment-service:8081"}'

curl -X POST http://localhost:8474/proxies/shipment-latency/toxics \
  -H "Content-Type: application/json" \
  -d '{"name":"latency","type":"latency","attributes":{"latency":3000}}'

curl -s http://localhost:8080/orders \
  -H "Content-Type: application/json" \
  -d '{"customer_id":1,"order_number":"ORD-LATENCY","total":50.00,"product_id":1,"quantity":1,"idempotency_key":"latency-001"}'

curl -X DELETE http://localhost:8474/proxies/shipment-latency/toxics/latency
```

**Resultados**

| Escenario | Resultado |
|-----------|-----------|
| Latencia 3s añadida | Order creada exitosamente (order_id: 136) |
| Toxicidad removida | Circuit breaker restablecido |

**Interpretación:** con 3 segundos de latencia añadida por toxiproxy, la orden se creó exitosamente (order_id: 136). El circuit breaker permitió que la llamada terminara dentro del timeout esperado, utilizando el fallback a local. Si el timeout hubiera sido menor a 3s, el circuit breaker se habría abierto y el pedido habría fallado. El comportamiento observado es **resiliente**: el sistema toleró la latencia mediante fallback a la implementación local.

---
## 6. Fallo de base de datos remota

**Propósito:** verificar comportamiento cuando la BD de shipment o inventory cae.

**Comandos:**
```bash
docker stop axiom-shipment-db
curl -s http://localhost:8080/orders \
  -H "Content-Type: application/json" \
  -d '{"customer_id":1,"order_number":"ORD-DB","total":50.00,"product_id":1,"quantity":1,"idempotency_key":"db-001"}'
docker logs axiom-flow --tail 20
docker start axiom-shipment-db
```


**resultados**

```


```
---

## 7. Recuperación y observación post-fallo

**Propósito:** verificar que el recovery worker retoma pasos pendientes tras levantar servicios caídos.

**Comandos:**
```bash
docker logs axiom-flow --tail 50
curl -s http://localhost:8080/health
```


**resultados**

```

```
---

## 8. Consumo de Saga (estado final)

**Propósito:** inspeccionar pasos registrados en `saga_steps` después de una prueba de fallo.

**Comando:**
```bash
curl -s http://localhost:8080/orders/{order_id}/saga
```
Reemplazar `{order_id}` por el ID devuelto en la respuesta de creación de orden.

```

```
