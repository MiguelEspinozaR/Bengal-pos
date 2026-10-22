# Arquitectura y modelo de datos — Punto de venta

> Documento técnico de la versión 1. Define la arquitectura de componentes, el modelo de datos en PostgreSQL, los invariantes, las fronteras transaccionales y de sincronización, y el respaldo/restauración.
> Stack de la v1: **Go** (API/servidor), **React + TypeScript** (interfaz responsive), **PostgreSQL** (bases de datos local y remota).

## 1. Vista de componentes

```
┌─────────────────────────────────────────────────────────────┐
│  Navegador — React + TypeScript (responsive)                │
│  PC (ratón), tablet/táctil y celular                        │
│  · Flujo de venta, caja, inventario, reportes, comandas     │
│  · Impresión: recibos/comandas desde el navegador           │
│  · Exportación e impresión de reportes CSV / PDF            │
│  · Sin cola local de datos: ante caída de red solo reintenta │
└───────────────┬─────────────────────────────────────────────┘
                │ API REST (JSON) sobre HTTP/HTTPS — siempre
                │ hacia el servidor LOCAL (red del negocio)
┌───────────────▼─────────────────────────────────────────────┐
│  Servidor Go                                                │
│  · Transporte: handlers HTTP, validación de entrada         │
│  · Aplicación: casos de uso, autorización por rol,          │
│    transacciones (venta, caja, inventario)                  │
│  · Dominio: reglas de negocio (montos enteros, invariantes) │
│  · Persistencia: pgx/v5, migraciones SQL manuales           │
│    versionadas                                              │
└───────────────┬─────────────────────────────────────────────┘
                │
┌───────────────▼─────────────────────────────────────────────┐
│  PostgreSQL LOCAL (dentro del negocio)                      │
│  · ÚNICA fuente de escrituras de negocio                    │
│  · Sirve el POS durante la operación (LAN)                  │
│  · Publicación de replicación lógica ──────────────┐        │
└────────────────────────────────────────────────────┼────────┘
                                                     │ con internet
                                                     ▼
┌─────────────────────────────────────────────────────────────┐
│  PostgreSQL REMOTO (fuera del negocio)                      │
│  · Réplica de respaldo (solo lectura para la aplicación)    │
│  · Recibe local → remota; jamás origina escrituras          │
│  · Base de la estrategia de respaldo/restauración           │
└─────────────────────────────────────────────────────────────┘
```

**Reglas de la frontera**

- Los navegadores **siempre** hablan con el servidor local (LAN). No existe almacenamiento colgado del navegador ni cola de ventas en el cliente: si la red local cae, la venta no se registra; si cae internet, el negocio **sigue vendiendo** porque la base local sigue viva.
- El estado "offline" significa: sin enlace local ↔ remoto. En ese estado solo la base local recibe escrituras; al reconectar, la sincronización local → remota se reanuda sola.
- **Impresión**: recibos y comandas se imprimen desde el navegador (impresora del sistema/RED), sin drivers dedicados. Cada comanda se genera **por área de preparación** a partir de la venta. Los reportes se generan en CSV/PDF por el servidor y el navegador los abre/imprime.
- **Seguridad básica**: autenticación obligatoria en la API; cada endpoint autoriza por rol; contraseñas con hash (bcrypt/argon2); enlace a la base remota con TLS/VPN y credenciales de solo-replicación; ningún secreto en el repositorio.

## 2. Modelo de datos (PostgreSQL)

### 2.1 Convenciones

- Identificadores: `BIGINT GENERATED ALWAYS AS IDENTITY` (la base local es la única escritora, no hay colisión de secuencias).
- **Montos**: `BIGINT` en **unidad mínima de Bs** (1 Bs = 100 unidades mínimas). Nunca `float`/`double`; la conversión a texto se hace solo al presentar.
- **Cantidades** (inventario, recetas): `BIGINT` en la unidad mínima declarada del producto (p. ej. gramos), con columna `unidad`.
- Marca de tiempo: `TIMESTAMPTZ`.
- Baja lógica: columna `activo BOOLEAN` (usuarios, productos, proveedores, depósitos, áreas, categorías, métodos de pago). Con historial no se borra nada.
- Migraciones: SQL manual y versionada; la tabla `esquema_migraciones` existe en **ambas** bases con la misma versión.

### 2.2 Usuarios y configuración

**`usuarios`**

| Columna | Tipo | Restricciones |
|---|---|---|
| id | BIGINT identity | PK |
| nombre_completo | TEXT | NOT NULL |
| login | TEXT | NOT NULL, **UNIQUE** |
| password_hash | TEXT | NOT NULL |
| rol | TEXT | NOT NULL, `CHECK (rol IN ('dueno','administrador','encargado','empleado'))` |
| activo | BOOLEAN | NOT NULL DEFAULT true |
| creado_en / actualizado_en | TIMESTAMPTZ | NOT NULL DEFAULT now() |

- **Rol único por usuario**: el rol es una sola columna (no tabla de muchos a muchos); la exclusividad es estructural.
- `CHECK (rol IN (...))` cubre RF-11/RF-12; desactivar sin borrar (`activo = false`).

**`configuracion`**: `clave TEXT PK`, `valor JSONB NOT NULL`, `actualizado_en`. Guarda símbolo/formato de la moneda y parámetros configurables (O-05).

### 2.3 Catálogo, proveedores y depósitos

**`categorias`**: id PK, `nombre TEXT NOT NULL UNIQUE`, activo. (Configurable sin código, O-05.)

**`areas_preparacion`**: id PK, `nombre TEXT NOT NULL UNIQUE`, `orden_impresion INT NOT NULL DEFAULT 0`, activo. Áreas configurables (RF-26/O-05).

**`productos`**

| Columna | Tipo | Restricciones |
|---|---|---|
| id | BIGINT identity | PK |
| codigo | TEXT | NOT NULL, **UNIQUE** (RF-01) |
| nombre | TEXT | NOT NULL |
| categoria_id | BIGINT | FK → categorias, NULL |
| area_preparacion_id | BIGINT | FK → areas_preparacion, NULL (ver producto_areas) |
| unidad | TEXT | NOT NULL DEFAULT 'unidad' |
| stock_minimo | BIGINT | NOT NULL DEFAULT 0, CHECK ≥ 0 (RF-01/RF-04) |
| precio_venta_bs | BIGINT | NOT NULL, CHECK ≥ 0 |
| activo | BOOLEAN | NOT NULL DEFAULT true |
| creado_en / actualizado_en | TIMESTAMPTZ | NOT NULL DEFAULT now() |

Índices: `UNIQUE (codigo)`; `ix_productos_nombre (nombre)`; `ix_productos_categoria (categoria_id)`.

**`producto_areas`** (ítem ↔ área de preparación)

| Columna | Tipo | Restricciones |
|---|---|---|
| producto_id | BIGINT | PK, FK → productos |
| area_preparacion_id | BIGINT | FK → areas_preparacion, NOT NULL |

- PK compuesta `(producto_id, area_preparacion_id)` **más** índice único `ux_producto_areas_producto (producto_id)`: cada ítem tiene **exactamente un** área (RF-26: "cada ítem define su área"). La forma de tabla permite ampliar a varias áreas por ítem en el futuro sin rediseñar.
- Ticket por área: la comanda se agrupa por `producto_areas` al imprimir (impresión derivada, sin tabla propia de tickets).

**`proveedores`**: id PK, `nombre TEXT NOT NULL`, contacto (`contacto_nombre`, `telefono`, `email`, `direccion`) NULL, activo, timestamps. Índice `ix_proveedores_nombre (nombre)` (RF-08, agrupación RF-04).

**`producto_proveedores`**: `producto_id` + `proveedor_id` PK compuesta, FKs a productos/proveedores, `es_principal BOOLEAN NOT NULL DEFAULT false` (RF-07).

**`depositos`**: id PK, `nombre TEXT NOT NULL UNIQUE`, `ubicacion TEXT`, activo, timestamps (RF-05).

**`stock_depositos`**

| Columna | Tipo | Restricciones |
|---|---|---|
| producto_id | BIGINT | PK #1, FK → productos |
| deposito_id | BIGINT | PK #2, FK → depositos |
| cantidad | BIGINT | NOT NULL (puede ser negativa si hubo confirmación, RF-03) |
| actualizado_en | TIMESTAMPTZ | NOT NULL DEFAULT now() |

- `UNIQUE (producto_id, deposito_id)`; índice `ix_stock_depositos_deposito (deposito_id)` (bajo stock por depósito, RF-04).
- Es una proyección acelerada: **`cantidad = Σ movimientos_inventario (producto, depósito)`**; se actualiza solo dentro de la transacción del movimiento y es reconciliable con una consulta de verificación.

**`movimientos_inventario`** — historial inmutable (RF-02/RF-03/RF-06/RNF-03)

| Columna | Tipo | Restricciones |
|---|---|---|
| id | BIGINT identity | PK |
| producto_id | BIGINT | FK → productos, NOT NULL |
| deposito_id | BIGINT | FK → depositos, NOT NULL (depósito afectado) |
| tipo | TEXT | NOT NULL, `CHECK IN ('entrada','salida','venta','traslado','ajuste','compensacion')` |
| cantidad | BIGINT | NOT NULL, `CHECK (cantidad <> 0)`; **signada**: positiva = sube stock, negativa = baja |
| stock_negativo_confirmado | BOOLEAN | NOT NULL DEFAULT false (RF-03) |
| grupo_id | UUID | NULL; `CHECK (tipo <> 'traslado' OR grupo_id IS NOT NULL)` |
| venta_id | BIGINT | FK → ventas, NULL (salida por venta, RF-16) |
| usuario_id | BIGINT | FK → usuarios, NOT NULL (RF-02) |
| correccion_id | BIGINT | FK → correcciones, NULL (asiento compensador) |
| motivo | TEXT | NULL (traslados/ajustes/compensaciones) |
| creado_en | TIMESTAMPTZ | NOT NULL DEFAULT now() |

Índices: `ix_movinv_producto_fecha (producto_id, creado_en)`, `ix_movinv_deposito_fecha (deposito_id, creado_en)`, `ix_movinv_venta (venta_id)`, `ix_movinv_fecha (creado_en)`.

- **Traslados (RF-06)**: una transferencia = **dos filas** en la misma transacción con el mismo `grupo_id` (salida negativa en origen + entrada positiva en destino); nunca quedan a medias.
- **Inmutabilidad**: trigger `BEFORE UPDATE OR DELETE` que rechaza la operación en esta tabla (y en las demás de solo-inserción). Corregir = INSERT de fila `tipo='compensacion'` con `correccion_id`.

**`costos_compra`** (RF-27): id PK, producto_id FK, proveedor_id FK NULL, `costo_bs BIGINT CHECK ≥ 0`, `fecha DATE NOT NULL`, usuario_id FK, creado_en. Solo inserción (historial de costos); el costo vigente = último registro. Índices `(producto_id, fecha DESC)`, `(proveedor_id)`.

**`receta_items`** (RF-28/RF-29/RF-30): `producto_id` (elaborado) + `ingrediente_id` PK compuesta, `cantidad BIGINT NOT NULL CHECK > 0`, FKs → productos, `CHECK (producto_id <> ingrediente_id)`.
- Solo alimenta costo/margen: costo(elaborado) = Σ `cantidad × costo_ingrediente`; margen = `precio_venta_bs − costo`. **Nunca descuenta ingredientes al vender** (RF-30): la venta solo toca el stock del producto terminado.

### 2.4 Ventas y pagos

**`metodos_pago`** (configurables, O-05/RF-14): id PK, `codigo TEXT NOT NULL UNIQUE` (precargados: `efectivo`, `tarjeta`, `qr`), `nombre TEXT NOT NULL`, `requiere_caja_abierta BOOLEAN NOT NULL DEFAULT false` (true para efectivo), activo.

**`ventas`**

| Columna | Tipo | Restricciones |
|---|---|---|
| id | BIGINT identity | PK |
| estado | TEXT | NOT NULL, `CHECK IN ('en_proceso','completada','anulada')`, DEFAULT 'en_proceso' |
| usuario_id | BIGINT | FK → usuarios, NOT NULL (RF-17) |
| caja_sesion_id | BIGINT | FK → caja_sesiones, **NULL salvo que la venta incluya efectivo** (entonces obligatoria y abierta, RF-20) |
| fecha | TIMESTAMPTZ | NOT NULL DEFAULT now() (RF-17) |
| total_bs | BIGINT | NOT NULL, CHECK ≥ 0 |
| descuento_bs | BIGINT | NOT NULL DEFAULT 0, CHECK ≥ 0 (autorizado, ver abajo) |
| descuento_usuario_id | BIGINT | FK → usuarios, NULL: administrador/dueno que autorizó |
| usuario_anulo_id | BIGINT | FK → usuarios, NULL (solo si estado = 'anulada') |
| creado_en / actualizado_en | TIMESTAMPTZ | NOT NULL DEFAULT now() |

Índices: `ix_ventas_fecha (fecha)`, `ix_ventas_usuario (usuario_id)`, `ix_ventas_caja (caja_sesion_id)`, `ix_ventas_estado (estado)`.

- `estado='en_proceso'`: borrador mientras se arman las partes de pago (ventas pendientes del cierre de caja). Sin efecto en stock ni caja.
- `estado='completada'`: los montos quedan **sellados** (trigger bloquea UPDATE de montos/detalles; solo se permite la transición a `'anulada'` con `usuario_anulo_id`), nunca DELETE.
- Anulación = marca de estado + asientos compensadores de stock/caja + registro en auditoría (requiere administrador/dueno).

**`venta_detalles`**: id PK, `venta_id FK NOT NULL` (índice `ix_vdetalles_venta`), `producto_id FK NOT NULL` (índice, reportes por producto), `cantidad BIGINT NOT NULL CHECK > 0`, `precio_unitario_bs BIGINT NOT NULL CHECK ≥ 0` (precio sellado al vender), `descuento_bs BIGINT NOT NULL DEFAULT 0 CHECK ≥ 0`, `subtotal_bs BIGINT NOT NULL CHECK ≥ 0` con `subtotal_bs = cantidad × precio_unitario_bs − descuento_bs`.

**`partes_pago`** (RF-18/RF-18a)

| Columna | Tipo | Restricciones |
|---|---|---|
| id | BIGINT identity | PK |
| venta_id | BIGINT | FK → ventas, NOT NULL (índice) |
| metodo_id | BIGINT | FK → metodos_pago, NOT NULL |
| monto_bs | BIGINT | NOT NULL, CHECK > 0 |
| estado | TEXT | NOT NULL, `CHECK IN ('pendiente','confirmado')` |
| confirmado_por | BIGINT | FK → usuarios; obligatorio si estado='confirmado' (trigger) |
| confirmado_en | TIMESTAMPTZ | obligatoria si estado='confirmado' |
| referencia | TEXT | NULL (nro. de operación de tarjeta/QR) |
| creado_en | TIMESTAMPTZ | NOT NULL DEFAULT now() |

- **Confirmación manual por parte**: cada fila lleva `confirmado_por`/`confirmado_en`; la API no confirma partes por sí sola. Sin propinas (RF-18b): no existe columna de propina y el total solo admite `Σ(monto_bs de las partes)` de productos/descuentos.
- **Invariantes**: con venta `completada`, `Σ(partes con estado='confirmado') = total_bs` (validado en la transacción de confirmación); el efectivo exige caja abierta.

**`impresiones`** (RF-25): id PK, `documento_tipo CHECK IN ('recibo','comanda')`, `documento_id BIGINT` (venta), `numero_copia INT NOT NULL DEFAULT 1`, `usuario_id FK NOT NULL`, `creado_en NOT NULL DEFAULT now()`. Solo inserción; índices `(documento_tipo, documento_id)`.

### 2.5 Caja

**`caja_sesiones`** (RF-19/RF-20/RF-22/RF-23)

| Columna | Tipo | Restricciones |
|---|---|---|
| id | BIGINT identity | PK |
| estado | TEXT | NOT NULL, `CHECK IN ('abierta','cerrada')` |
| fecha_apertura | TIMESTAMPTZ | NOT NULL DEFAULT now() |
| usuario_apertura_id | BIGINT | FK → usuarios, NOT NULL |
| monto_inicial_bs | BIGINT | NOT NULL, CHECK ≥ 0 |
| fecha_cierre | TIMESTAMPTZ | NULL |
| usuario_cierre_id | BIGINT | FK → usuarios, NULL |
| efectivo_esperado_bs | BIGINT | NULL (calculado al cierre) |
| efectivo_contado_bs | BIGINT | NULL (lo que informa el cajero) |
| diferencia_bs | BIGINT | NULL (contado − esperado; nunca se oculta) |

- **Índice parcial único** `ux_caja_una_abierta ON caja_sesiones () WHERE estado='abierta'`: a lo sumo una caja abierta (evita apertura doble, caso borde de la spec).
- Índices: `ix_caja_sesiones_apertura (fecha_apertura)`.
- Cierre (RF-22): `efectivo_esperado_bs = Σ monto_bs de los caja_movimientos de la sesión` (la fila `'apertura'` aporta el `monto_inicial_bs`); se registran contado y diferencia; la sesión pasa a `'cerrada'` y se sella (solo inserción de movimientos ya no es posible para esa sesión).

**`caja_movimientos`** — historial inmutable (RF-21/RNF-03)

| Columna | Tipo | Restricciones |
|---|---|---|
| id | BIGINT identity | PK |
| sesion_id | BIGINT | FK → caja_sesiones, NOT NULL |
| tipo | TEXT | NOT NULL, `CHECK IN ('apertura','venta_efectivo','entrada','salida','cierre','compensacion')` |
| monto_bs | BIGINT | NOT NULL, `CHECK (monto_bs <> 0)`; **signado**: positivo = entra efectivo, negativo = sale |
| motivo | TEXT | NOT NULL para entradas/salidas (gastos, retiros) |
| usuario_id | BIGINT | FK → usuarios, NOT NULL |
| venta_id | BIGINT | FK → ventas, NULL (solo 'venta_efectivo') |
| correccion_id | BIGINT | FK → correcciones, NULL |
| creado_en | TIMESTAMPTZ | NOT NULL DEFAULT now() |

Índices: `ix_cajamov_sesion_fecha (sesion_id, creado_en)`, `ix_cajamov_venta (venta_id)`. Solo inserción (trigger).

### 2.6 Personal

**`asistencia`** (RF-31/RF-32): id PK, `usuario_id FK NOT NULL` (índice `(usuario_id, fecha_hora)`), `tipo CHECK IN ('entrada','salida','descanso_inicio','descanso_fin')`, `fecha_hora TIMESTAMPTZ NOT NULL DEFAULT now()`, `registrado_por FK NOT NULL`, `motivo TEXT NULL` (correcciones), `correccion_id FK NULL`, `creado_en`. Solo inserción. El "olvido de salida" se corrige con asiento compensador + registro de corrección (caso borde de la spec).

**`pagos_personal`** (RF-33/RF-34): id PK, `usuario_id FK NOT NULL` (beneficiario; índice `(usuario_id, fecha_pago)`), `monto_bs BIGINT NOT NULL CHECK > 0`, `fecha_pago DATE NOT NULL`, `concepto TEXT NOT NULL`, `periodo_desde DATE NULL`, `periodo_hasta DATE NULL`, `registrado_por FK NOT NULL`, `correccion_id FK NULL`, `creado_en`. **Solo registro manual** (sin cálculo de horas/comisiones); solo inserción.

### 2.7 Auditoría y correcciones ("deshacer")

**`auditoria`**: id PK, `usuario_id FK NULL`, `accion TEXT NOT NULL`, `entidad_tipo TEXT NOT NULL`, `entidad_id BIGINT NULL`, `datos JSONB NULL` (detalle de la acción), `creado_en NOT NULL DEFAULT now()`. Solo inserción. Índices: `(entidad_tipo, entidad_id)`, `(usuario_id, creado_en)`, `(creado_en)`. Registra anulaciones, descuentos autorizados, desactivaciones, reimpresiones, aperturas/cierres y correcciones (RNF-03).

**`correcciones`** (cabecera del "deshacer", decisión de producto): id PK, `entidad_tipo CHECK IN ('inventario','caja','asistencia','pago_personal','venta')`, `entidad_id BIGINT NOT NULL`, `autorizado_por FK NOT NULL` (solo administrador/dueno, validado en la API), `motivo TEXT NOT NULL`, `creado_en NOT NULL DEFAULT now()`. Solo inserción.
- Cada corrección genera **asientos compensadores nuevos** en las tablas de historial (`movimientos_inventario`, `caja_movimientos`, `asistencia`, `pagos_personal`) con `correccion_id` apuntando a esta fila y signo/tipo opuesto al original. **Nunca UPDATE/DELETE** del registro original (RNF-03).

### 2.8 Sincronización, idempotencia y migraciones

- **`esquema_migraciones`** (`version BIGINT PK`, `aplicado_en TIMESTAMPTZ`): existe en la base **local y remota**; debe coincidir en la misma versión (control de divergencia de esquema).
- **`idempotencia_claves`**: `clave UUID PK`, `usuario_id FK`, `endpoint TEXT NOT NULL`, `respuesta JSONB NULL`, `creado_en NOT NULL DEFAULT now()`, índice `(creado_en)` para poda. El navegador genera un UUID por operación de venta y lo reenvía en los reintentos; el servidor responde el resultado original sin duplicar la venta.
- **Réplica remota**: mismas tablas, sin objetos propios del cliente. Configuración de replicación (publicación/suscripción) es infraestructura, no modelo. Monitoreo: `pg_stat_replication` (local) y `pg_stat_subscription` (remota).

## 3. Invariantes

1. **Montos enteros**: todos los montos son `BIGINT` en unidad mínima de Bs; sin `float`/`double` en ningún cálculo (RF-15 cambio, RF-22 cuadre, RF-28/29 costos y márgenes).
2. **Historial inmutable**: `movimientos_inventario`, `caja_movimientos`, `auditoria`, `impresiones`, `asistencia`, `pagos_personal`, `costos_compra`, `correcciones` son de **solo inserción** (triggers rechazan UPDATE/DELETE). `ventas`/`venta_detalles`/`partes_pago` quedan sellados al completar la venta (solo transición a `'anulada'`, sin editar montos). **Deshacer = INSERT compensador** referenciando `correccion_id` (RNF-03).
3. **Rol único por usuario**: columna única `usuarios.rol` con CHECK; no hay asignación múltiple de roles.
4. **Caja abierta para efectivo**: toda parte en efectivo exige `caja_sesiones.estado='abierta'` (validación en servidor + índice parcial único de una sola caja abierta). Tarjeta/QR no requieren caja abierta.
5. **Venta atómica**: confirmar venta es **una transacción** que incluye detalle de ítems, partes de pago confirmadas, descuento de stock (por depósito), movimientos de inventario, movimientos de caja del efectivo y auditoría; si algo falla, **todo se revierte** (RNF-04, RF-16).
6. **Código de producto único**: `UNIQUE (productos.codigo)` (RF-01).
7. **Solo la base local escribe**: la aplicación nunca escribe en la base remota (la remota corre con un usuario de aplicación de solo lectura + usuario de replicación). Toda escritura de negocio nace en la local.
8. **Partes de pago**: con venta completada, `Σ confirmadas = total_bs`; cada parte tiene confirmación manual con usuario y hora; sin propinas (RF-18).
9. **Stock por depósito = Σ movimientos**: reconciliable por consulta; el descuento por venta ocurre en la misma transacción.
10. **Recetas no descuentan**: al vender solo baja el producto terminado; la receta es solo costo/margen (RF-30).
11. **Autorizaciones**: anulaciones, descuentos, devoluciones y correcciones las realiza únicamente administrador o dueño, siempre con registro en `auditoria`/`correcciones`.
12. **Stock negativo**: permitido solo con confirmación explícita; la fila de movimiento queda marcada con `stock_negativo_confirmado` (RF-03).

## 4. Fronteras transaccionales

**Confirmar venta (una transacción, base local)**

1. Si la venta incluye una parte en efectivo: debe existir `caja_sesiones` **abierta** para la venta; se toma `SELECT ... FOR UPDATE` sobre esa sesión. (Venta solo con tarjeta/QR: no exige caja abierta, RF-20.)
2. Lock ordenado por `producto_id` sobre `stock_depositos` (orden fijo ⇒ sin interbloqueos).
3. Validar: suma de partes confirmadas = `total_bs`; cantidades > 0; partes con `confirmado_por`.
4. Insertar/confirmar `partes_pago`; sellar `ventas` → `'completada'`; insertar `venta_detalles`.
5. Actualizar `stock_depositos.cantidad`; insertar `movimientos_inventario` (`tipo='venta'`, negativos, `venta_id`; marca de stock negativo solo si el usuario confirmó).
6. Insertar `caja_movimientos` (`tipo='venta_efectivo'`) por cada parte en efectivo confirmada.
7. Insertar `auditoria` de la venta.
8. **COMMIT**; ante cualquier error: **ROLLBACK** completo (no queda stock descontado sin dinero registrado, ni viceversa).

Las comandas por área y la impresión del recibo ocurren **después del COMMIT** (son derivadas; una falla de impresión no afecta la venta — caso borde de la spec: reimpresión auditable).

**Otras operaciones atómicas**

- *Movimiento de inventario / traslado*: actualiza `stock_depositos` + inserta fila(s) en `movimientos_inventario` en una transacción (traslados: ambas filas del `grupo_id` juntas).
- *Apertura de caja*: inserta sesión `'abierta'` + `caja_movimientos('apertura')` con `monto_bs = monto_inicial_bs` (el índice parcial único serializa aperturas concurrentes).
- *Cierre de caja*: calcula esperado, registra contado/diferencia, inserta `caja_movimientos('cierre')`, sella la sesión.
- *Corrección/deshacer*: inserta `correcciones` + asientos compensadores + `auditoria` en una transacción (solo rol admin/dueno).

**Concurrencia**: `READ COMMITTED` con locks de fila para la venta; cajón único de caja abierta garantizado por índice; reintento ante fallo de serialización. Volumen esperado (25 usuarios simultáneos, ~2 000 ventas/día) cubierto con pool de conexiones pgx e índices de fecha.

## 5. Frontera offline / sincronización

**Mecanismo elegido: replicación lógica de PostgreSQL (publicación en la base local → suscripción en la remota, salida `pgoutput`).**

Justificación:

- La remota **nunca origina escrituras de negocio** (es respaldo/ réplica de solo lectura), así que la replicación es de un solo sentido y **no hay resolución de conflictos que implementar**: no existen escrituras concurrentes sobre las mismas filas desde dos orígenes.
- Es **transaccional**: la réplica aplica transacciones completas, por lo que la base remota nunca queda "a medias" (consistente con RNF-04) y no requiere código de sincronización propio.
- **Reanudación automática**: el *replication slot* retiene el progreso; al reconectar tras un período offline, la remota recibe exactamente lo que faltaba. No hay cola en el navegador ni en la aplicación: la cola "es" el slot de replicación.
- Operación y monitoreo nativos (`pg_stat_replication`, `pg_stat_subscription`, lag de replicación).

Alternativa considerada y descartada para v1: **sincronización a nivel de aplicación** (tabla de *outbox* + trabajos de envío): obligaría a mantener código propio de ordenamiento, reintentos, agrupación y estado, con mayor superficie de fallo. Se reserva solo si la réplica remota no pudiera ser PostgreSQL.

Consecuencias asumidas:

- Las migraciones de esquema (DDL) **no** se replican: se aplican a ambas bases en la misma versión (`esquema_migraciones`); desajuste de versión = divergencia detectable, no silenciosa.
- El enlace local → remota debe ser seguro (TLS/VPN) y el slot se monitorea para no retener WAL indefinidamente durante caídas largas.

**Idempotencia (tres capas)**

1. *Replicación*: cada transacción se aplica una sola vez según la posición LSN del slot; reconectar no duplica filas.
2. *API*: el navegador envía una **clave de idempotencia (UUID)** por operación; el servidor la registra en `idempotencia_claves` y ante un reintento devuelve la respuesta original (no duplica la venta aunque el cliente haya perdido la respuesta).
3. *Modelo*: restricciones únicas (código de producto, PKs) como última red ante duplicaciones.

**Conflictos**: en v1 no hay estrategia de merge porque solo la local escribe. Reglas:

- La remota jamás recibe escrituras de negocio (usuario de aplicación de solo lectura allí).
- Si la réplica se corrompe/diverge, se **regenera desde la local** (eliminar suscripción, nueva suscripción con copia inicial) — la local siempre es la verdad.
- Divergencia de esquema se controla con `esquema_migraciones`.

**Frontera offline**: sin internet, la operación local continúa y el lag de replicación crece; al reconectar se drena automáticamente. Ninguna venta espera a la red: la transacción de venta es 100% local.

## 6. Respaldo y restauración (RNF-06)

- **Respaldo continuo**: la base remota es la réplica de respaldo en línea (actualizada mientras hay internet).
- **Respaldo puntual**: además de la réplica, la base local genera respaldos periódicos (`pg_dump` de todo el cluster o respaldo físico) con retención definida por el negocio, almacenados **fuera del negocio** (en la remota o en almacenamiento externo). Esto cubre casos que la réplica sola no cubre (borrado lógico/erróneo reciente, ransomware, corrupción de la local).
- **Restauración** (procedimiento):
  1. Instalar PostgreSQL en el hardware nuevo.
  2. Restaurar el respaldo puntual (o crear la suscripción hacia la remota con copia inicial, si la remota está al día).
  3. Aplicar migraciones hasta la versión vigente y verificar `esquema_migraciones`.
  4. Reconciliar: conteos de ventas del día, `Σ stock_depositos` vs `Σ movimientos_inventario`, última sesión de caja.
  5. Apuntar el servidor Go (mismas credenciales por variable de entorno) y verificar venta de prueba.
- **Relación con RNF-03**: restaurar un respaldo no edita historial; reconstruye el historial completo tal como quedó registrado. El inmutable sigue vigente en la base remota (los mismos triggers viajan en el esquema).
- *Nota*: la spec no fija tiempos objetivo de recuperación (RTO) ni pérdida máxima aceptable (RPO); la retención y la frecuencia de respaldo puntual quedan como parámetros de operación del negocio.

## 7. Rendimiento y seguridad (marco)

- **RNF-01** (venta < 2 s, 25 usuarios simultáneos): pool de conexiones pgx en el servidor Go, los índices de la sección 2, consultas de reporte por rango de `fecha`, sin N+1 en el detalle de venta.
- **RNF-02**: autenticación obligatoria; autorización por rol en cada endpoint (RF-11); hash de contraseñas; secretos (credenciales de base de datos y de replicación) solo en variables de entorno del despliegue, nunca en el repositorio.
- **RNF-07**: el servidor separa transporte / aplicación / dominio / persistencia; los módulos nuevos (delivery, reservas) se agregan sin tocar los existentes; las tablas nuevas usan las mismas convenciones de la sección 2.1.
