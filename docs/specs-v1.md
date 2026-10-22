# Punto de venta

> Estado: resuelto v1.0. Clarificaciones resueltas 2026-10-20 (ver `.ai/DECISIONS.md`).
> Esta spec describe el **qué**, no el **cómo**. El stack elegido se registra en la sección 9 y el diseño (arquitectura y modelo de datos) en `docs/arquitectura.md`.

## 1. Contexto y problema

Punto de venta para administrar una repostería (adaptable a restaurantes y negocios similares) que maneje inventarios, entradas y salidas de productos y de efectivo, costos, comandas y reportes. El diseño debe permitir seguir agregando funciones sin rehacer lo existente.

Hoy el negocio no cuenta con un registro único y confiable de stock, proveedores, caja y personal, lo que dificulta reponer a tiempo y conocer el costo real de lo que se vende.

## 2. Objetivos y no-objetivos

**Objetivos**
- O-01: Facilitar la administración diaria del negocio desde un solo sistema.
- O-02: Tener datos confiables de stock y proveedores.
- O-03: Controlar el efectivo mediante apertura y cierre de caja con cuadre.
- O-04: Conocer el costo de los productos y el margen de venta.
- O-05: Que la configuración (categorías, áreas, métodos de pago) permita adaptar el sistema a otros rubros sin cambiar el código.

**No-objetivos**
- Facturación electrónica o fiscal.
- Tienda en línea o pedidos por internet.
- **Fuera de v1**: delivery, reservas de mesas y programa de fidelidad (no son objetivos de v1).

## 3. Roles y permisos

| Rol | Puede |
|---|---|
| Dueño | Todo lo del administrador; registrar administradores; ver todos los reportes |
| Administrador | Registrar empleados, encargados y proveedores; abrir/cerrar caja; gestionar pagos del personal; ver reportes |
| Encargado de área | Registrar ítems y depósitos de su área; registrar entradas y salidas de inventario |
| Empleado | Realizar ventas; registrar su asistencia; reimprimir comandas y recibos |

- **Roles**: un usuario tiene un **único rol** (exclusivos). No se permite asignar múltiples roles simultáneos.
- **Autorizaciones**: anulaciones, descuentos y devoluciones requieren usuario **administrador o dueño**, con registro obligatorio en el historial (trazable).

## 4. Historias de usuario

**Inventario y proveedores**
- HU-01: Como dueño, quiero ver los productos bajo el mínimo, para reponer a tiempo.
- HU-02: Como encargado de área, quiero registrar ítems en el inventario, para mantener el catálogo actualizado.
- HU-03: Como encargado de área, quiero registrar un depósito, para saber en qué lugar está cada ítem.
- HU-04: Como administrador, quiero registrar a los proveedores de los ítems del inventario, para saber a quién comprar cada producto.

**Usuarios**
- HU-05: Como dueño, quiero registrar a un administrador, para delegar la gestión.
- HU-06: Como administrador, quiero registrar a un empleado, para que pueda operar el sistema.

**Ventas y caja**
- HU-07: Como empleado, quiero realizar una venta y elegir entre efectivo, tarjeta y QR, para cobrar según prefiera el cliente.
- HU-08: Como administrador, quiero abrir y cerrar caja, para controlar el efectivo del turno.
- HU-09: Como empleado, quiero reimprimir una comanda o un recibo, para atender reclamos o pérdidas del papel.

**Personal**
- HU-10: Como empleado, quiero registrar mi hora de llegada, salida y descansos (opcional), para dejar constancia de mi jornada.
- HU-11: Como administrador, quiero gestionar los pagos de todo el personal, para pagar correctamente y con respaldo.

**Costos y reportes** (propuestas, por confirmar)
- HU-12: Como dueño, quiero conocer el costo y el margen de cada producto, para fijar precios.
- HU-13: Como dueño, quiero reportes de ventas, caja, inventario y personal, para decidir con datos.

## 5. Requisitos funcionales

### 5.1 Inventario
- RF-01: El sistema debe permitir crear productos con código único, nombre, stock mínimo y stock actual.
- RF-02: El sistema debe registrar movimientos de entrada y salida con fecha, cantidad y usuario.
- RF-03: El sistema debe advertir salidas que dejen el stock en negativo.
- RF-04: El sistema debe listar los productos con stock igual o menor al mínimo y resumirlos agrupados por proveedor.
- RF-05: El sistema debe permitir registrar depósitos (ubicaciones de almacenamiento) y asignar el stock de cada ítem a un depósito.
- RF-06: El sistema debe permitir registrar traslados de stock entre depósitos con fecha, cantidad y usuario.
- RF-07: El sistema debe asociar cada ítem a uno o más proveedores.

### 5.2 Proveedores
- RF-08: El sistema debe permitir registrar, editar y desactivar proveedores con nombre, contacto e ítems que suministra.

### 5.3 Usuarios y accesos
- RF-09: El sistema debe permitir al dueño registrar administradores.
- RF-10: El sistema debe permitir al administrador registrar empleados y encargados de área.
- RF-11: El sistema debe restringir cada función según el rol del usuario (sección 3).
- RF-12: El sistema debe permitir desactivar usuarios sin borrar su historial.

### 5.4 Ventas y pagos
- RF-13: El sistema debe permitir registrar una venta con uno o más productos y cantidades.
- RF-14: El sistema debe permitir elegir el método de pago: efectivo, tarjeta o QR.
- RF-15: Al pagar en efectivo, el sistema debe calcular y mostrar el cambio.
- RF-16: Al confirmar una venta, el sistema debe descontar del inventario los ítems correspondientes.
- RF-17: El sistema debe registrar en cada venta la fecha, el usuario y el método de pago.
- RF-18: El sistema debe permitir pagos divididos (efectivo, tarjeta, QR). Cada parte debe confirmarse manualmente.
- RF-18a: El sistema debe registrar cada parte de pago con su método y estado de confirmación.
- RF-18b: No se permite registrar propinas en v1.

### 5.5 Caja
- RF-19: El sistema debe permitir al administrador abrir caja con un monto inicial.
- RF-20: El sistema debe impedir ventas en efectivo si no hay una caja abierta.
- RF-21: El sistema debe permitir registrar entradas y salidas de efectivo (gastos, retiros) con motivo, monto y usuario.
- RF-22: Al cerrar caja, el sistema debe mostrar el efectivo esperado, el efectivo contado y la diferencia.
- RF-23: El sistema debe conservar el historial de aperturas y cierres de caja.

### 5.6 Comandas y recibos
- RF-24: El sistema debe generar una comanda por cada pedido y un recibo por cada venta.
- RF-25: El sistema debe permitir reimprimir cualquier comanda o recibo, dejando registro de quién lo reimprimió y cuándo.
- RF-26: El sistema debe enviar comandas por **áreas configurables**. Cada ítem define su área de preparación y se imprime un ticket por área. Las áreas deben ser configurables sin tocar código (O-05).

### 5.7 Costos
- RF-27: El sistema debe permitir registrar el costo de compra de cada ítem y su historial.
- RF-28: El sistema debe calcular el costo de un producto elaborado a partir de una receta (ítems y cantidades).
- RF-29: El sistema debe mostrar el margen de cada producto (precio de venta menos costo).
- RF-30: Al vender, el sistema **descuenta solo el producto terminado** del inventario. La receta se usa para calcular costo y margen (RF-28/29), no para descontar ingredientes.

### 5.8 Personal: asistencia y pagos
- RF-31: El sistema debe permitir a cada empleado registrar su hora de llegada y de salida.
- RF-32: El sistema debe permitir registrar descansos de forma opcional.
- RF-33: El sistema debe permitir al administrador registrar y consultar los pagos al personal (monto, fecha, concepto, periodo).
- RF-34: El sistema permite **solo registrar manualmente** los pagos al personal (monto, fecha, concepto, periodo). No incluye cálculo automático de horas, sueldos ni comisiones en v1.

### 5.9 Reportes
- RF-35: El sistema debe ofrecer reportes de ventas por periodo, producto, empleado y método de pago.
- RF-36: El sistema debe ofrecer un reporte de inventario (stock actual, bajo mínimo, movimientos).
- RF-37: El sistema debe ofrecer un reporte de caja por turno.
- RF-38: El sistema debe ofrecer un reporte de asistencia y pagos del personal.
- RF-39: El sistema debe permitir exportar los reportes en **CSV y PDF**.

## 6. Requisitos no funcionales

- RNF-01 (Rendimiento): Registrar una venta debe tardar menos de 2 s en condiciones normales de uso. **Volumen medio**: hasta 2 000 ventas/día, ~10 000 productos, hasta 25 usuarios simultáneos.
- RNF-02 (Seguridad): Solo usuarios autenticados pueden operar; cada acción se limita por rol.
- RNF-03 (Trazabilidad): Todo movimiento de inventario, caja, venta, reimpresión y pago queda en un historial que no se puede editar ni borrar.
- RNF-04 (Integridad): Una venta nunca debe quedar a medias; si falla, no debe descontar stock ni registrar dinero.
- RNF-05 (Disponibilidad): **Modo offline obligatorio**. El sistema opera con **dos bases de datos sincronizadas**: una base **local** (dentro del negocio, sirve el POS por la red local y es el **único origen de las escrituras de negocio**) y una base **remota** (copia de respaldo, legible). Con internet, la local se sincroniza con la remota. Sin internet, solo la base local recibe ventas y al reconectar se sincroniza a la remota. **No hay cola de datos en el navegador**: los clientes siempre conversan con el servidor local. La sincronización debe preservar la integridad transaccional (RNF-04/RF-16). (Vinculado a RNF-04/RF-16.)
- RNF-06 (Respaldo): Los datos deben poder respaldarse y restaurarse.
- RNF-07 (Extensibilidad): Agregar un módulo o rubro nuevo no debe requerir modificar los módulos existentes.
- RNF-08 (Usabilidad): **Responsive**. Debe funcionar en PC (ratón/teclado), tablet/táctil y celular. Priorizando operación táctil para agilizar la venta.

## 7. Criterios de aceptación

- **RF-03**: *Given* un producto con stock 5, *when* se registra una salida de 8, *then* el sistema muestra una advertencia y exige confirmación; si el usuario confirma, el stock queda en -3 y el movimiento queda marcado como "stock negativo".
- **RF-04**: *Given* productos A (mínimo 10, stock 10, proveedor X) y B (mínimo 5, stock 2, proveedor Y), *when* se consulta el reporte de bajo stock, *then* aparecen A y B agrupados bajo sus proveedores X e Y.
- **RF-14/15**: *Given* una venta de 45 pagada con 50 en efectivo, *when* se confirma, *then* el sistema muestra cambio de 5 y registra el método "efectivo".
- **RF-16**: *Given* un producto con stock 10, *when* se vende 1 unidad, *then* el stock pasa a 9 y existe un movimiento de salida vinculado a la venta.
- **RF-20**: *Given* que no hay caja abierta, *when* un empleado intenta cobrar en efectivo, *then* el sistema lo impide e indica abrir caja.
- **RF-22**: *Given* una caja abierta con 100 de monto inicial y 300 en ventas en efectivo, *when* el administrador cierra con 395 contados, *then* el sistema muestra esperado 400, contado 395 y diferencia -5.
- **RF-25**: *Given* un recibo emitido, *when* un empleado lo reimprime, *then* se imprime marcado como "copia" y se registra usuario y hora.
- **RF-11**: *Given* un empleado autenticado, *when* intenta registrar a otro empleado, *then* el sistema rechaza la acción.
- **RF-01**: *Given* un producto con código "TOR-001", *when* se intenta crear otro con el mismo código, *then* el sistema rechaza la operación y no crea duplicados.

## 8. Casos borde y errores

- Código de producto duplicado.
- Cantidades cero, negativas o no numéricas.
- Venta de un producto sin stock (ver RF-03).
- Dos usuarios vendiendo o moviendo el mismo producto a la vez.
- Cierre de caja con diferencia: debe registrarse, no ocultarse.
- Cierre de caja con ventas pendientes o comandas abiertas.
- Intento de abrir caja cuando ya hay una abierta.
- Falla de impresión: la venta debe quedar registrada y permitir reimprimir.
- QR o tarjeta: la confirmación del pago es **manual** por parte del cajero (cada parte confirmada manualmente, sin integración obligatoria con pasarela en v1).
- Empleado que olvida registrar salida: el administrador debe poder corregirlo con registro de la corrección.
- Reporte sin datos: mostrar estado vacío.
- Proveedor desactivado con ítems asociados.
- Eliminación de usuarios, productos o proveedores con historial: solo desactivar.

## 9. Restricciones y supuestos

- Supuesto: la v1 atiende un solo local.
- Supuesto: el catálogo de productos y las categorías son configurables para adaptarse a otros rubros.
- **Moneda**: Boliviano (Bs), moneda única. Montos en unidad mínima entera (sin flotantes). Símbolo y formato configurables.
- **Hardware**: Sin hardware dedicado. Impresión desde el navegador. Sin lector de código de barras, sin cajón de dinero obligatorio.
- **Alcance inicial**: Un solo local (repostería/negocio propio), adaptable a otros rubros.
- **Stack tecnológico (v1)**: **Go** (API/servidor), **React + TypeScript** (interfaz responsive) y **PostgreSQL** (base de datos local y base de datos remota). El modelo de datos y la sincronización local ↔ remota se detallan en `docs/arquitectura.md`.
- Dependencias de despliegue: servidor en el negocio (LAN) con acceso a internet para sincronizar con la base remota; impresora usada desde el navegador (sin hardware dedicado).

## 10. Preguntas abiertas (resumen)

Las clarificaciones han sido resueltas (ver `.ai/DECISIONS.md`, 2026-10-20). Los puntos quedan cerrados:

1. Roles combinados y autorizaciones: **un único rol por usuario**; anulaciones/descuentos/devoluciones requieren **admin o dueño** con registro en historial.
2. Pagos divididos y propinas: **pagos divididos** (efectivo/tarjeta/QR), cada parte **confirmada manualmente**; **sin propinas**.
3. Comandas por área: **sí, por áreas configurables** (cada ítem define su área; ticket por área; configurable sin código).
4. Recetas: **solo producto terminado** se descuenta al vender; receta para costo/margen.
5. Pagos al personal: **solo registro manual** (sin cálculo automático de horas/comisiones).
6. Sin conexión: **modo offline obligatorio** (dos bases sincronizadas: la **local** en el negocio escribe y la **remota** es respaldo; sincronización local → remota al reconectar; **sin cola en el navegador**; la remota no origina escrituras de negocio).
7. Confirmación tarjeta/QR: **manual** (sin pasarela obligatoria).
8. Hardware: **sin hardware dedicado**; impresión desde navegador; sin lector de código de barras ni cajón.
9. Exportación: **CSV + PDF**.
10. Moneda: **Boliviano (Bs)**, moneda única, montos en unidad mínima entera.
11. Volumen: **medio** (2 000 ventas/día, ~10 000 productos, 25 usuarios simultáneos), venta < 2 s.
12. Fuera de v1: **delivery, reservas, fidelidad**.
13. Corrección de movimientos: **deshacer por administrador** desde pantalla de correcciones; se genera **movimiento compensador nuevo** (historial inmutable, RNF-03).

