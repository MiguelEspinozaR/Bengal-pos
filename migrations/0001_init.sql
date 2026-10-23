-- T-03 initial schema: users, catalog, inventory, sales with split payments,
-- cash sessions, immutable history and idempotency.
-- All monetary amounts are BIGINT in the minimum unit of Bs (RT-01).

CREATE TABLE usuarios (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    nombre_completo TEXT NOT NULL,
    login TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    rol TEXT NOT NULL CHECK (rol IN ('dueno', 'administrador', 'encargado', 'empleado')),
    activo BOOLEAN NOT NULL DEFAULT true,
    creado_en TIMESTAMPTZ NOT NULL DEFAULT now(),
    actualizado_en TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE categorias (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    nombre TEXT NOT NULL UNIQUE,
    activo BOOLEAN NOT NULL DEFAULT true
);

CREATE TABLE areas_preparacion (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    nombre TEXT NOT NULL UNIQUE,
    orden_impresion INT NOT NULL DEFAULT 0,
    activo BOOLEAN NOT NULL DEFAULT true
);

CREATE TABLE productos (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    codigo TEXT NOT NULL UNIQUE,
    nombre TEXT NOT NULL,
    categoria_id BIGINT REFERENCES categorias (id),
    area_preparacion_id BIGINT REFERENCES areas_preparacion (id),
    unidad TEXT NOT NULL DEFAULT 'unidad',
    stock_minimo BIGINT NOT NULL DEFAULT 0 CHECK (stock_minimo >= 0),
    precio_venta_bs BIGINT NOT NULL CHECK (precio_venta_bs >= 0),
    activo BOOLEAN NOT NULL DEFAULT true,
    creado_en TIMESTAMPTZ NOT NULL DEFAULT now(),
    actualizado_en TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ix_productos_nombre ON productos (nombre);
CREATE INDEX ix_productos_categoria ON productos (categoria_id);

CREATE TABLE depositos (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    nombre TEXT NOT NULL UNIQUE,
    ubicacion TEXT,
    activo BOOLEAN NOT NULL DEFAULT true,
    creado_en TIMESTAMPTZ NOT NULL DEFAULT now(),
    actualizado_en TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE stock_depositos (
    producto_id BIGINT NOT NULL REFERENCES productos (id),
    deposito_id BIGINT NOT NULL REFERENCES depositos (id),
    cantidad BIGINT NOT NULL DEFAULT 0,
    actualizado_en TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (producto_id, deposito_id)
);
CREATE INDEX ix_stock_depositos_deposito ON stock_depositos (deposito_id);

CREATE TABLE correcciones (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    entidad_tipo TEXT NOT NULL CHECK (entidad_tipo IN ('inventario', 'caja', 'asistencia', 'pago_personal', 'venta')),
    entidad_id BIGINT NOT NULL,
    autorizado_por BIGINT NOT NULL REFERENCES usuarios (id),
    motivo TEXT NOT NULL,
    creado_en TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE caja_sesiones (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    estado TEXT NOT NULL CHECK (estado IN ('abierta', 'cerrada')),
    fecha_apertura TIMESTAMPTZ NOT NULL DEFAULT now(),
    usuario_apertura_id BIGINT NOT NULL REFERENCES usuarios (id),
    monto_inicial_bs BIGINT NOT NULL CHECK (monto_inicial_bs >= 0),
    fecha_cierre TIMESTAMPTZ,
    usuario_cierre_id BIGINT REFERENCES usuarios (id),
    efectivo_esperado_bs BIGINT,
    efectivo_contado_bs BIGINT,
    diferencia_bs BIGINT
);
CREATE UNIQUE INDEX ux_caja_una_abierta ON caja_sesiones (estado) WHERE estado = 'abierta';
CREATE INDEX ix_caja_sesiones_apertura ON caja_sesiones (fecha_apertura);

CREATE TABLE metodos_pago (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    codigo TEXT NOT NULL UNIQUE,
    nombre TEXT NOT NULL,
    requiere_caja_abierta BOOLEAN NOT NULL DEFAULT false,
    activo BOOLEAN NOT NULL DEFAULT true
);

INSERT INTO metodos_pago (codigo, nombre, requiere_caja_abierta)
VALUES ('efectivo', 'Efectivo', true),
       ('tarjeta', 'Tarjeta', false),
       ('qr', 'QR', false)
ON CONFLICT (codigo) DO NOTHING;

CREATE TABLE ventas (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    estado TEXT NOT NULL DEFAULT 'en_proceso' CHECK (estado IN ('en_proceso', 'completada', 'anulada')),
    usuario_id BIGINT NOT NULL REFERENCES usuarios (id),
    caja_sesion_id BIGINT REFERENCES caja_sesiones (id),
    fecha TIMESTAMPTZ NOT NULL DEFAULT now(),
    total_bs BIGINT NOT NULL CHECK (total_bs >= 0),
    descuento_bs BIGINT NOT NULL DEFAULT 0 CHECK (descuento_bs >= 0),
    descuento_usuario_id BIGINT REFERENCES usuarios (id),
    usuario_anulo_id BIGINT REFERENCES usuarios (id),
    creado_en TIMESTAMPTZ NOT NULL DEFAULT now(),
    actualizado_en TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ix_ventas_fecha ON ventas (fecha);
CREATE INDEX ix_ventas_usuario ON ventas (usuario_id);
CREATE INDEX ix_ventas_caja ON ventas (caja_sesion_id);
CREATE INDEX ix_ventas_estado ON ventas (estado);

CREATE TABLE venta_detalles (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    venta_id BIGINT NOT NULL REFERENCES ventas (id),
    producto_id BIGINT NOT NULL REFERENCES productos (id),
    cantidad BIGINT NOT NULL CHECK (cantidad > 0),
    precio_unitario_bs BIGINT NOT NULL CHECK (precio_unitario_bs >= 0),
    descuento_bs BIGINT NOT NULL DEFAULT 0 CHECK (descuento_bs >= 0),
    subtotal_bs BIGINT NOT NULL CHECK (subtotal_bs >= 0),
    CHECK (subtotal_bs = cantidad * precio_unitario_bs - descuento_bs)
);
CREATE INDEX ix_vdetalles_venta ON venta_detalles (venta_id);
CREATE INDEX ix_vdetalles_producto ON venta_detalles (producto_id);

CREATE TABLE partes_pago (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    venta_id BIGINT NOT NULL REFERENCES ventas (id),
    metodo_id BIGINT NOT NULL REFERENCES metodos_pago (id),
    monto_bs BIGINT NOT NULL CHECK (monto_bs > 0),
    estado TEXT NOT NULL CHECK (estado IN ('pendiente', 'confirmado')),
    confirmado_por BIGINT REFERENCES usuarios (id),
    confirmado_en TIMESTAMPTZ,
    referencia TEXT,
    creado_en TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (
        (estado = 'pendiente' AND confirmado_por IS NULL AND confirmado_en IS NULL)
        OR (estado = 'confirmado' AND confirmado_por IS NOT NULL AND confirmado_en IS NOT NULL)
    )
);
CREATE INDEX ix_partes_pago_venta ON partes_pago (venta_id);

CREATE TABLE movimientos_inventario (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    producto_id BIGINT NOT NULL REFERENCES productos (id),
    deposito_id BIGINT NOT NULL REFERENCES depositos (id),
    tipo TEXT NOT NULL CHECK (tipo IN ('entrada', 'salida', 'venta', 'traslado', 'ajuste', 'compensacion')),
    cantidad BIGINT NOT NULL CHECK (cantidad <> 0),
    stock_negativo_confirmado BOOLEAN NOT NULL DEFAULT false,
    grupo_id UUID,
    venta_id BIGINT REFERENCES ventas (id),
    usuario_id BIGINT NOT NULL REFERENCES usuarios (id),
    correccion_id BIGINT REFERENCES correcciones (id),
    motivo TEXT,
    creado_en TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (tipo <> 'traslado' OR grupo_id IS NOT NULL)
);
CREATE INDEX ix_movinv_producto_fecha ON movimientos_inventario (producto_id, creado_en);
CREATE INDEX ix_movinv_deposito_fecha ON movimientos_inventario (deposito_id, creado_en);
CREATE INDEX ix_movinv_venta ON movimientos_inventario (venta_id);
CREATE INDEX ix_movinv_fecha ON movimientos_inventario (creado_en);

CREATE TABLE caja_movimientos (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    sesion_id BIGINT NOT NULL REFERENCES caja_sesiones (id),
    tipo TEXT NOT NULL CHECK (tipo IN ('apertura', 'venta_efectivo', 'entrada', 'salida', 'cierre', 'compensacion')),
    monto_bs BIGINT NOT NULL,
    motivo TEXT,
    usuario_id BIGINT NOT NULL REFERENCES usuarios (id),
    venta_id BIGINT REFERENCES ventas (id),
    correccion_id BIGINT REFERENCES correcciones (id),
    creado_en TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (monto_bs <> 0 OR tipo IN ('apertura', 'cierre')),
    CHECK (tipo NOT IN ('entrada', 'salida') OR motivo IS NOT NULL),
    CHECK (tipo <> 'venta_efectivo' OR venta_id IS NOT NULL)
);
CREATE INDEX ix_cajamov_sesion_fecha ON caja_movimientos (sesion_id, creado_en);
CREATE INDEX ix_cajamov_venta ON caja_movimientos (venta_id);

CREATE TABLE auditoria (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    usuario_id BIGINT REFERENCES usuarios (id),
    accion TEXT NOT NULL,
    entidad_tipo TEXT NOT NULL,
    entidad_id BIGINT,
    datos JSONB,
    creado_en TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ix_auditoria_entidad ON auditoria (entidad_tipo, entidad_id);
CREATE INDEX ix_auditoria_usuario_fecha ON auditoria (usuario_id, creado_en);
CREATE INDEX ix_auditoria_fecha ON auditoria (creado_en);

CREATE TABLE idempotencia_claves (
    clave UUID PRIMARY KEY,
    usuario_id BIGINT NOT NULL REFERENCES usuarios (id),
    endpoint TEXT NOT NULL,
    respuesta JSONB,
    creado_en TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ix_idempotencia_fecha ON idempotencia_claves (creado_en);

CREATE TABLE IF NOT EXISTS esquema_migraciones (
    version BIGINT PRIMARY KEY,
    aplicado_en TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Immutable history: reject UPDATE/DELETE at the database boundary (RNF-03).
CREATE FUNCTION rechazar_mutacion_historial() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'El historial es de solo inserción (%).', TG_TABLE_NAME
        USING ERRCODE = 'integrity_constraint_violation';
END;
$$;

CREATE TRIGGER trg_movinv_inmutable
    BEFORE UPDATE OR DELETE ON movimientos_inventario
    FOR EACH ROW EXECUTE FUNCTION rechazar_mutacion_historial();
CREATE TRIGGER trg_cajamov_inmutable
    BEFORE UPDATE OR DELETE ON caja_movimientos
    FOR EACH ROW EXECUTE FUNCTION rechazar_mutacion_historial();
CREATE TRIGGER trg_auditoria_inmutable
    BEFORE UPDATE OR DELETE ON auditoria
    FOR EACH ROW EXECUTE FUNCTION rechazar_mutacion_historial();
CREATE TRIGGER trg_correcciones_inmutable
    BEFORE UPDATE OR DELETE ON correcciones
    FOR EACH ROW EXECUTE FUNCTION rechazar_mutacion_historial();

-- A completed sale is sealed: amounts/details/parts cannot change afterwards.
CREATE FUNCTION sellar_venta_completada() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'Las ventas no se eliminan.';
    END IF;
    IF OLD.estado = 'completada' THEN
        IF NEW.total_bs <> OLD.total_bs
           OR NEW.descuento_bs <> OLD.descuento_bs
           OR NEW.usuario_id <> OLD.usuario_id
           OR NEW.caja_sesion_id IS DISTINCT FROM OLD.caja_sesion_id
           OR NEW.fecha <> OLD.fecha
           OR NEW.descuento_usuario_id IS DISTINCT FROM OLD.descuento_usuario_id
           OR NEW.estado NOT IN ('completada', 'anulada') THEN
            RAISE EXCEPTION 'La venta completada está sellada.';
        END IF;
    END IF;
    IF OLD.estado = 'anulada' AND NEW.estado <> 'anulada' THEN
        RAISE EXCEPTION 'La venta anulada está sellada.';
    END IF;
    IF NEW.estado = 'anulada' AND NEW.usuario_anulo_id IS NULL THEN
        RAISE EXCEPTION 'La anulación requiere usuario_anulo_id.';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_ventas_selladas
    BEFORE UPDATE OR DELETE ON ventas
    FOR EACH ROW EXECUTE FUNCTION sellar_venta_completada();

CREATE FUNCTION bloquear_detalle_venta_completada() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    v_venta_id BIGINT;
    v_estado TEXT;
BEGIN
    IF TG_OP = 'DELETE' THEN
        v_venta_id := OLD.venta_id;
    ELSE
        v_venta_id := NEW.venta_id;
    END IF;
    SELECT estado INTO v_estado FROM ventas WHERE id = v_venta_id;
    IF v_estado = 'completada' THEN
        RAISE EXCEPTION 'La venta completada está sellada.';
    END IF;
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_vdetalles_sellados
    BEFORE INSERT OR UPDATE OR DELETE ON venta_detalles
    FOR EACH ROW EXECUTE FUNCTION bloquear_detalle_venta_completada();

CREATE FUNCTION bloquear_parte_venta_completada() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    v_venta_id BIGINT;
    v_estado TEXT;
BEGIN
    IF TG_OP = 'DELETE' THEN
        v_venta_id := OLD.venta_id;
    ELSE
        v_venta_id := NEW.venta_id;
    END IF;
    SELECT estado INTO v_estado FROM ventas WHERE id = v_venta_id;
    IF v_estado = 'completada' THEN
        RAISE EXCEPTION 'La venta completada está sellada.';
    END IF;
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_partes_selladas
    BEFORE INSERT OR UPDATE OR DELETE ON partes_pago
    FOR EACH ROW EXECUTE FUNCTION bloquear_parte_venta_completada();
