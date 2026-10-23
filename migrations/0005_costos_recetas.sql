-- T-08: purchase costs, recipes and margin (RF-27..RF-30, RT-01, RT-04).
--
-- costos_compra is an insert-only history: the current cost is the latest row
-- by (fecha DESC, creado_en DESC). All amounts stay BIGINT in the minimum unit
-- of Bs; no floating point and no per-batch multiplier is applied (RT-01).
--
-- A recipe is editable configuration, not history: `recetas` is the header of
-- the elaborated product and `receta_detalles` holds one row per ingredient.
-- The recipe is used only for cost/margin. Selling never touches ingredients
-- (RF-30); only the finished product is deducted by the existing sale flow.

CREATE TABLE costos_compra (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    producto_id BIGINT NOT NULL REFERENCES productos (id),
    proveedor_id BIGINT REFERENCES proveedores (id),
    costo_bs BIGINT NOT NULL CHECK (costo_bs >= 0),
    fecha DATE NOT NULL,
    usuario_id BIGINT NOT NULL REFERENCES usuarios (id),
    creado_en TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ix_costos_compra_producto_fecha
    ON costos_compra (producto_id, fecha DESC, creado_en DESC);
CREATE INDEX ix_costos_compra_proveedor ON costos_compra (proveedor_id);

-- Both functions already exist from 0001_init.sql: the same trigger function
-- rejects UPDATE/DELETE on every insert-only history table.
CREATE TRIGGER trg_costos_compra_inmutable
    BEFORE UPDATE OR DELETE ON costos_compra
    FOR EACH ROW EXECUTE FUNCTION rechazar_mutacion_historial();

CREATE TABLE recetas (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    producto_elaborado_id BIGINT NOT NULL UNIQUE REFERENCES productos (id),
    rendimiento BIGINT NOT NULL DEFAULT 1 CHECK (rendimiento > 0),
    usuario_id BIGINT NOT NULL REFERENCES usuarios (id),
    creado_en TIMESTAMPTZ NOT NULL DEFAULT now(),
    actualizado_en TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE receta_detalles (
    receta_id BIGINT NOT NULL REFERENCES recetas (id) ON DELETE CASCADE,
    producto_insumo_id BIGINT NOT NULL REFERENCES productos (id),
    cantidad BIGINT NOT NULL CHECK (cantidad > 0),
    PRIMARY KEY (receta_id, producto_insumo_id)
);
CREATE INDEX ix_receta_detalles_insumo
    ON receta_detalles (producto_insumo_id);

-- The composite PK prevents duplicate ingredients, but the elaborated product
-- and its ingredients live in different columns; this trigger enforces "a
-- product cannot be an ingredient of its own recipe" at the database boundary.
CREATE FUNCTION validar_receta_detalle() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    elaborado_id BIGINT;
BEGIN
    SELECT producto_elaborado_id INTO elaborado_id
    FROM recetas WHERE id = NEW.receta_id;

    IF elaborado_id = NEW.producto_insumo_id THEN
        RAISE EXCEPTION 'Un producto no puede ser ingrediente de su propia receta.'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_receta_detalle_autorreferencia
    BEFORE INSERT OR UPDATE ON receta_detalles
    FOR EACH ROW EXECUTE FUNCTION validar_receta_detalle();

-- Compatibility read-only view for the name used by the architecture document.
-- v1 writes through `recetas`/`receta_detalles` so a replacement is atomic.
CREATE VIEW receta_items AS
SELECT r.producto_elaborado_id AS producto_id,
       d.producto_insumo_id AS ingrediente_id,
       d.cantidad
FROM recetas r
JOIN receta_detalles d ON d.receta_id = r.id;
