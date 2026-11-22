## APrendizaje que se obtuvo en el proyecto

1.  la higuera estranguladora (strangler fig)

El sistema viejo sigue funcionando y atendiendo tráfico.

El sistema nuevo se construye alrededor, empezando por una funcionalidad.

Una fachada/proxy redirige el tráfico gradualmente del viejo al nuevo.

Cuando el nuevo cubre toda la funcionalidad, el viejo se apaga.

```text

Esto se aprendio debido a que se migro al uso de docker para servicios independientes, llevandonos a uno de los problemas clasicos
"Como migro sin romper lo que tengo"

ademas que entre menos dependencia/acoplamiento exista entre servicio ese se le puede aplicar migraciones por que es mas facil de rvertir

```

2. Nunca elegir o ordenar por comlejidad si no por reversibildad , eso quiere decir que elgiamos entre lo mas facil de revertir, luego algo con una estrategia barata y luego lo que es caro de revertir

tecnicas aprendidas: Branch by Abstraction != feature flag


## Herramientas nuevas

vegeta -> para prueba de cargas contra endpoints
toxiproxy -> para fallas de red, caidas, lentitud.
