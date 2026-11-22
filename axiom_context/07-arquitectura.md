# Arquitectura: ¿qué debería cubrir?

> [!WARNING]
> La informacion de esto la puedes encontrar en [[03-dominio]] este documento actual solamente expner una guia
> para abarcar la arquitectura nada mas. analisis_dominio contiene lo que le corresponde de aca.

> [!TIP]
> Primero se conocen las entidades, casos de usos, interfaces o contratos y luego la relacion que tendran.

## 1. Estructura del sistema

- ¿Qué componentes principales tendrá el sistema?
- ¿Qué responsabilidad tendrá cada componente?
- ¿Cómo se relacionan entre sí?
- ¿Qué estilo arquitectónico encaja con el problema? Por ejemplo, monolito modular o microservicios, cuando corresponda.

## 2. Interacciones y comunicación

- ¿Cómo colaboran los componentes para ejecutar los casos de uso?
- Orden de las llamadas y quién orquesta.
- Contrato en la frontera entre componentes (qué pide, qué devuelve, qué error).
- Qué pasa en el fallo intermedio (punto de compensación).
- ¿Cómo intercambian información?
- ¿Qué interfaces o contratos necesitan?
- ¿Cómo se relaciona el sistema con servicios externos?

## 3. Datos y persistencia

- ¿Dónde se almacenan los datos?
- ¿Qué componentes son responsables de consultarlos y modificarlos?
- ¿Qué restricciones de integridad y consistencia deben mantenerse?
- ¿Qué decisiones importantes existen sobre transacciones y acceso a datos?

## 4. Atributos de calidad

Definir cómo la arquitectura responde a las necesidades relevantes del sistema:

- Seguridad.
- Rendimiento.
- Disponibilidad y recuperación.
- Escalabilidad, si se necesita.
- Mantenibilidad y capacidad de evolución.

No todos requieren el mismo nivel de atención. Se priorizan según los requisitos y riesgos identificados.

## 5. Despliegue y entorno

(por el momento no)

- ¿Dónde y cómo se ejecutará el sistema?
- ¿Qué dependencias necesita?
- ¿Cómo se configuran los entornos?
- ¿Qué aspectos de operación son relevantes, como registros, monitorización y recuperación?

El detalle dependerá del alcance del proyecto.

## 6. Validación de la arquitectura

- ¿La estructura propuesta permite cumplir los requisitos?
- ¿Resuelve los escenarios y riesgos prioritarios?
- ¿Qué compromisos introduce?
- ¿Qué aspectos necesitan un prototipo o una prueba técnica antes de continuar?
