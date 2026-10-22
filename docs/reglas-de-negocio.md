# Reglas de negocio — Punto de venta (v1)

> Reglas operativas de la versión 1. El *qué* vive en `docs/specs-v1.md`; el *dónde se guarda* (modelo de datos e invariantes, autoritativo) vive en `docs/arquitectura.md`. Aquí queda el *comportamiento exacto observable* por regla, con su RF de referencia y sus casos borde, para que la implementación no requiera interpretaciones.
> Convención: los montos se manejan en **unidad mínima de Bs** (1 Bs = 100 unidades mínimas), enteros (RT-01).

## 1. Reglas transversales

### RT-01 — Montos en enteros (spec §9; arquitectura §2.1, invariante 1)
- **Regla**: todos los montos (precios, subtotales, descuentos, pagos, cambio, caja, costos) son números **enteros** en unidad mínima de Bs. No se usan decimales ni flotantes en la API, los cálculos ni la base de datos (`BIGINT`).
- **Casos borde**: monto con decimales enviado por el cliente; operación que produciría una fracción.
- **Comportamiento exacto**: la API rechaza montos no enteros con error de validación; no se redondea (no existe división de montos en el negocio: cambio, cuadre, subtotales y costo de receta son aritmética entera exacta). Ejemplos: 45,50 Bs = 4550; cambio de 5000 − 4500 = 500 (se muestra "Bs 5,00"). La conversión a texto con símbolo/decimales configurables ocurre **solo al presentar**. Los costos de compra y el costo de receta usan la **misma escala** de unidad mínima de Bs que todos los demás montos; no se aplica ningún multiplicador (p. ej. ×1000) ni escala por lote.

### RT-02 — Cantidades inválidas (RF-13, RF-02, RF-06; spec §8)
- **Regla**: las cantidades de ítems de venta son enteros en la unidad del producto y deben ser **mayores que cero**. Las cantidades de movimientos de inventario (entrada, salida, traslado, ajuste) se capturan como **enteros positivos** con un tipo que indica el sentido; el sistema aplica el signo (positivo sube stock, negativa baja).
- **Casos borde**: cantidad cero; cantidad negativa; cantidad no numérica o decimal; unidad no declarada.
- **Comportamiento exacto**: cero, negativas y no numéricas se **rechazan** con error de validación en la API y no se persiste nada. En la base, `venta_detalles.cantidad` exige `> 0` y `movimientos_inventario.cantidad` exige `<> 0` con signo ya aplicado por el sistema (última red ante errores de la interfaz).

### RT-03 — Venta atómica (RNF-04, RF-16; arquitectura §3.5 y §4)
- **Regla**: confirmar una venta es **una sola transacción** en la base local: detalle de ítems, partes de pago confirmadas, descuento de stock por depósito, movimientos de inventario, movimientos de caja del efectivo y auditoría.
- **Casos borde**: falla a mitad de la confirmación; dos usuarios vendiendo el mismo producto a la vez; pérdida de la respuesta del cliente tras confirmar.
- **Comportamiento exacto**: ante cualquier error **todo se revierte** (sin stock descontado sin dinero registrado ni viceversa). El stock se valida y descuenta dentro de la transacción con bloqueo de filas en orden fijo de producto (la advertencia de stock negativo se evalúa contra el stock al momento de ejecutar, no al armar el borrador). Los reintentos del navegador llevan clave de idempotencia: no duplican la venta. Las comandas y el recibo se imprimen **después** de confirmar; una falla de impresión no afecta la venta registrada.

### RT-04 — Historial inmutable y correcciones por compensación (RNF-03, decisión #13, RF-13; arquitectura §2.7, invariante 2)
- **Regla**: el historial (movimientos de inventario, movimientos de caja, auditoría, impresiones, asistencia, pagos al personal, costos de compra, correcciones) es de **solo inserción**: nunca se edita ni se borra. Corregir un registro ("deshacer") crea un **registro compensador nuevo** con signo/tipo opuesto que apunta a una cabecera de corrección.
- **Autorización**: las correcciones las realiza únicamente **administrador o dueño**, con motivo obligatorio y registro en auditoría.
- **Casos borde**: intento de editar/borrar un movimiento por API o SQL; corrección de un registro ya corregido; corrección sin motivo.
- **Comportamiento exacto**: cualquier UPDATE/DELETE sobre las tablas de historial es rechazado por la base (trigger). La corrección válida: inserta cabecera `correcciones` (autorizado_por, motivo) + asiento(es) compensador(es) con `correccion_id` + auditoría, en una transacción; los reportes muestran original y compensación (el neto es el estado real). Un registro ya compensado puede corregirse de nuevo con una nueva corrección (la cadena es el historial). Sin motivo o sin rol admin/dueno: rechazo con error de autorización.

### RT-05 — Rol único y autorizaciones (decisión #2, RF-11, RF-12, RF-09/10; arquitectura §2.2, invariante 3)
- **Regla**: cada usuario tiene **un solo rol** (dueño, administrador, encargado, empleado). **Anulaciones de venta, descuentos y correcciones** requieren administrador o dueño, con registro obligatorio en auditoría. Solo el dueño registra administradores (RF-09); solo el administrador registra empleados y encargados (RF-10).
- **Casos borde**: empleado que intenta registrar usuarios, anular, descontar o corregir; asignación de más de un rol; descuento sin autorizador.
- **Comportamiento exacto**: el servidor rechaza la acción con error de autorización (nada se persiste). Un descuento en una venta exige `descuento_bs ≥ 0`, `descuento_bs ≤ total de los ítems` y captura del usuario autorizador (admin/dueno); sin él, la venta no admite descuento. El rol se almacena en una única columna con lista cerrada: no existe forma de combinar roles.

### RT-06 — Desactivación sin borrado (RF-08, RF-12; spec §8; arquitectura §2.1)
- **Regla**: usuarios, productos, proveedores, depósitos, áreas, categorías y métodos de pago se **desactivan** (`activo = false`); nunca se eliminan registros con historial.
- **Comportamiento exacto**: un registro desactivado no aparece en las selecciones de formularios nuevos ni en el catálogo de venta por defecto, pero **conserva todo su historial** y aparece en reportes e impresiones históricas. Los detalles específicos por tipo de registro se describen en PR-03, PR-04 y US-01.

## 2. Productos (RF-01)

### PR-01 — Código único (RF-01; spec §7; invariante 6)
- **Regla**: `codigo` es obligatorio, no vacío y **único** en todo el catálogo.
- **Casos borde**: duplicado exacto; duplicado con mayúsculas/minúsculas distintas; código vacío o solo espacios; reintentos del mismo formulario.
- **Comportamiento exacto**: al guardar, el código se recorta de bordes y se compara tal como se almacena; un segundo producto con el mismo código se **rechaza** con el mensaje "El código del producto ya existe" y no se crea el duplicado (garantía final: restricción UNIQUE en la base). Código vacío tras recortar: rechazo por validación. `nombre` obligatorio, `stock_minimo ≥ 0`, `precio_venta_bs ≥ 0` (enteros, RT-01).

### PR-02 — Stock mínimo (RF-01, RF-04; arquitectura §2.3)
- **Regla**: cada producto declara `stock_minimo ≥ 0` en su unidad mínima. El reporte de bajo stock lista los productos con stock **igual o menor** al mínimo, agrupados por su proveedor principal.
- **Comportamiento exacto**: producto con stock = mínimo entra en el reporte (caso borde de la spec, RF-04). El stock por producto y depósito es la suma de sus movimientos; un producto sin stock registrado se trata como stock 0.

### PR-03 — Producto desactivado (spec §8, "solo desactivar"; regla derivada)
- **Regla**: un producto desactivado **no puede agregarse a ventas nuevas** ni a recetas nuevas ni a asociaciones nuevas con proveedores.
- **Casos borde**: producto desactivado ya agregado a un borrador de venta; producto desactivado en una venta ya confirmada; reactivación.
- **Comportamiento exacto**: la búsqueda de venta no ofrece productos inactivos por defecto. Un borrador de venta que ya contiene el producto **puede completarse** normalmente (el precio se selló al agregarlo, ver PG-04). Las ventas confirmadas lo muestran en recibos e historial. Reactivar el producto lo devuelve a la búsqueda sin alterar historial.

### PR-04 — Proveedor desactivado con ítems asociados (RF-07, RF-08; spec §8; regla derivada)
- **Regla**: desactivar un proveedor **conserva** sus asociaciones con productos y su historial; el proveedor deja de estar disponible para **nuevas** asociaciones y ediciones de contacto.
- **Casos borde**: producto cuyo único proveedor quedó desactivado; reporte de bajo stock con proveedor desactivado.
- **Comportamiento exacto**: el reporte de bajo stock sigue agrupando los ítems bajo el proveedor (aunque esté desactivado) para que la reposición no se pierda. Se recomienda advertir en la ficha del producto si todos sus proveedores están inactivos (no bloquea la venta).

## 3. Inventario y stock

### ST-01 — Salida que deja stock negativo (RF-03; spec §7; invariante 12)
- **Regla**: el sistema **advierte** toda salida (manual o por venta) que dejaría el stock del depósito por debajo de cero y **exige confirmación explícita** del usuario. Sin confirmación no hay movimiento.
- **Casos borde**: stock 5 y salida de 8 (caso de la spec); salida exactamente igual al stock (queda 0, sin advertencia); varias salidas consecutivas; confirmación y luego otra salida.
- **Comportamiento exacto**: con stock 5 y salida de 8, el sistema muestra la advertencia "La salida dejará el stock en negativo (quedará en -3). ¿Desea continuar?" y el botón de confirmar. Si el usuario **no** confirma, no se registra nada y el stock sigue en 5. Si **confirma**, el stock queda en **-3** y el movimiento aparece en el historial marcado como **"stock negativo"** (bandera `stock_negativo_confirmado`). La advertencia se evalúa contra el stock vigente al ejecutar el movimiento (sección RT-03 para concurrencia). Un movimiento ya registrado nunca se edita: una corrección posterior usa RT-04.

### ST-02 — Traslados entre depósitos (RF-06; arquitectura §2.3)
- **Regla**: un traslado mueve una cantidad **entera > 0** de un depósito a otro en **una sola transacción**, registrado como dos movimientos con el mismo grupo (salida negativa en origen, entrada positiva en destino) y el usuario responsable.
- **Comportamiento exacto**: traslado que dejaría el origen en negativo genera la misma advertencia y confirmación de ST-01. Nunca queda un traslado a medias: si falla una de las dos mitades, no se persiste nada.

## 4. Ventas y pagos

### PG-01 — Cambio en efectivo (RF-15; spec §7)
- **Regla**: al cobrar en efectivo, el **cambio = efectivo recibido − monto a pagar en efectivo**. Solo la parte en efectivo genera cambio; tarjeta y QR no.
- **Casos borde**: recibido igual al monto (cambio 0); recibido menor que el monto; pago dividido donde solo una parte es efectivo; pago 100 % tarjeta/QR.
- **Comportamiento exacto**: venta de 4500 pagada con 5000 en efectivo → el sistema **muestra cambio de 500** (Bs 5,00) y registra el método "efectivo". Recibido = monto → cambio 0, se muestra "Sin cambio" y no se emite línea de monto de cambio. **Recibido < monto en efectivo → error de validación** "El efectivo recibido es menor que el monto en efectivo a pagar"; no se registra la parte ni avanza el cobro. En pago dividido (PG-02), el cambio se calcula solo sobre la parte en efectivo: `recibido_efectivo − parte_efectivo`. Ventas solo con tarjeta/QR no calculan cambio.

### PG-02 — Pagos divididos y confirmación manual (RF-18, RF-18a, RF-18b; decisiones #3; arquitectura §2.4, invariante 8)
- **Regla**: una venta puede pagarse con varias **partes**, cada una con método (efectivo, tarjeta, QR — configurables) y monto **entero > 0**. Cada parte pasa a "confirmado" **solo por acción manual** del cajero; el sistema nunca confirma partes por sí solo. La venta solo se completa cuando **la suma de las partes confirmadas es igual al total**. **No existen propinas**.
- **Casos borde**: suma de partes menor o mayor que el total; parte con monto cero o negativo; intento de completar con partes sin confirmar; intento de enviar propina; parte confirmada por un usuario distinto del cajero de turno.
- **Comportamiento exacto**: cada parte registrada queda en estado "pendiente" con su método y monto; al confirmarla manualmente se sellan usuario y hora de confirmación. Completar la venta con suma distinta del total se **rechaza** con el mensaje "La suma de los pagos confirmados no coincide con el total" y la venta sigue en proceso. Partes con monto ≤ 0: rechazo por validación. Cualquier campo o monto de propina enviado por el cliente se **rechaza** como dato desconocido; el total de la venta solo proviene de ítems y descuentos autorizados. Un método de pago inactivo no admite nuevas partes.

### PG-03 — Caja abierta requerida para efectivo (RF-20, RF-19; spec §7; invariante 4)
- **Regla**: **toda** parte en efectivo (venta simple o dividida) exige que exista una sesión de caja **abierta**; sin ella, el cobro en efectivo se impide. Tarjeta y QR no requieren caja abierta.
- **Casos borde**: empleado intenta cobrar sin caja abierta; caja se cierra mientras una venta está en proceso (ver CJ-04); venta solo tarjeta/QR con caja cerrada (permitida).
- **Comportamiento exacto**: sin caja abierta, al cobrar en efectivo el sistema lo impide e indica **"Debe abrir caja para cobrar en efectivo"**; no se registra la parte. Abrir caja es exclusivo de administrador y dueño (RF-19); un empleado recibe error de autorización. La venta completada con efectivo queda ligada a la sesión de caja abierta que la cobró.

### PG-04 — Composición de la venta (RF-13, RF-17; arquitectura §2.4)
- **Regla**: una venta lleva **al menos un ítem** con cantidad > 0 (RT-02) y precio unitario **sellado al agregarlo** (precio de catálogo vigente en ese momento). Cada venta registra fecha, usuario y método(s) de pago.
- **Comportamiento exacto**: venta sin ítems o con total que no cuadra con detalle + descuento: rechazo al completar. El precio de catálogo posterior no altera ventas ya confirmadas. Al completar, montos y detalles quedan **sellados** (solo admite la transición a "anulada" con usuario que anula, ver RT-04/RT-05).

## 5. Caja

### CJ-01 — Una sola caja abierta (spec §8; arquitectura §2.5)
- **Regla**: a lo sumo **una** sesión de caja abierta a la vez.
- **Comportamiento exacto**: intentar abrir caja cuando ya hay una abierta se **rechaza** con el mensaje "Ya hay una caja abierta"; la restricción en la base garantiza la regla aunque haya aperturas concurrentes.

### CJ-02 — Apertura de caja (RF-19; arquitectura §4)
- **Regla**: administrador o dueño abren caja declarando un **monto inicial entero ≥ 0** (RT-01). La apertura genera el movimiento inicial del historial de la sesión.
- **Casos borde**: monto inicial cero (permitido); monto negativo o decimal (rechazo por validación).
- **Comportamiento exacto**: la apertura en una transacción crea la sesión "abierta" y su movimiento de apertura por el monto inicial; la sesión queda visible como caja vigente.

### CJ-03 — Cuadre al cierre (RF-22; spec §7; arquitectura §2.5)
- **Regla**: al cerrar, el sistema calcula `efectivo_esperado = monto_inicial + ventas en efectivo + entradas − salidas` de la sesión; el cajero informa el `efectivo_contado`; `diferencia = contado − esperado`. **La diferencia siempre se registra, nunca se oculta** (aunque sea cero o negativa).
- **Casos borde**: diferencia distinta de cero; contado igual al esperado (diferencia 0, igualmente registrada); diferencia negativa; cierre concurrente.
- **Comportamiento exacto**: caja con 100 iniciales y 300 en ventas en efectivo, cerrada con 395 contados → el sistema muestra **esperado 400, contado 395, diferencia −5** y los tres valores quedan guardados en la sesión. El cierre sella la sesión: no admite más movimientos ni reaperturas; el historial de aperturas y cierres es permanente (RF-23). El cuadre usa solo enteros (RT-01).

### CJ-04 — Cierre con ventas en proceso (spec §8; regla derivada)
- **Regla**: una venta en proceso (borrador con partes de pago sin confirmar) **no aporta** al efectivo esperado: el esperado se calcula solo con movimientos de caja ya registrados. El cierre **no se bloquea** por borradores, pero se **advierte** al cajero que existen ventas en proceso.
- **Casos borde**: completar una venta en proceso después del cierre; borrador abandonado.
- **Comportamiento exacto**: el cierre muestra el aviso "Hay ventas en proceso sin confirmar" y continúa. Si luego se intenta completar una venta que incluye efectivo, al no haber caja abierta vigente se aplica PG-03 (rechazo con indicación de abrir caja nueva). Los borradores abandonados no afectan stock ni caja.

## 6. Usuarios

### US-01 — Usuario desactivado (RF-12; spec §8; regla derivada)
- **Regla**: un usuario desactivado **no puede iniciar sesión ni operar**; su historial (ventas, movimientos, auditoría, correcciones que autorizó) se conserva íntegro.
- **Casos borde**: sesión abierta que queda activa tras desactivar; registro histórico que lo muestra; intento de seleccionarlo como autorizador.
- **Comportamiento exacto**: al desactivar, el sistema cierra (rechaza) sus sesiones abiertas; los inicios de sesión posteriores se rechazan con credenciales inválidas/genéricas (sin revelar el estado). No aparece en formularios nuevos ni como autorizador de descuentos, anulaciones o correcciones. Sus registros pasados siguen siendo válidos y visibles en reportes e impresiones. Solo administrador y dueño desactivan usuarios (RF-10); el dueño también a administradores (RF-09). Reactivar requiere igual autorización.
