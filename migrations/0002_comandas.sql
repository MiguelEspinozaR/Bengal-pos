-- T-04: configurable preparation areas, one comanda per area and auditable
-- printing (RF-24/RF-25/RF-26).
--
-- Comanda rows are generated inside the sale-completion transaction. Printing
-- is a separate, auditable step: a print failure never rolls back a completed
-- sale (RT-03, arquitectura §4).
--
-- Quantities and monetary amounts stay BIGINT (RT-01/RT-02).

CREATE TABLE comandas (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    venta_id BIGINT NOT NULL REFERENCES ventas (id),
    area_preparacion_id BIGINT NOT NULL REFERENCES areas_preparacion (id),
    usuario_id BIGINT NOT NULL REFERENCES usuarios (id),
    estado TEXT NOT NULL DEFAULT 'pendiente'
        CHECK (estado IN ('pendiente', 'impresa', 'anulada')),
    impresa BOOLEAN NOT NULL DEFAULT false,
    copia BOOLEAN NOT NULL DEFAULT false,
    impresa_en TIMESTAMPTZ,
    creado_en TIMESTAMPTZ NOT NULL DEFAULT now(),
    actualizado_en TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (NOT impresa OR impresa_en IS NOT NULL)
);

-- One ticket per area per sale (AC1): never two comandas for the same area.
CREATE UNIQUE INDEX ux_comandas_venta_area ON comandas (venta_id, area_preparacion_id);
CREATE INDEX ix_comandas_venta ON comandas (venta_id);
CREATE INDEX ix_comandas_area ON comandas (area_preparacion_id);

CREATE TABLE comanda_items (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    comanda_id BIGINT NOT NULL REFERENCES comandas (id) ON DELETE CASCADE,
    venta_detalle_id BIGINT NOT NULL REFERENCES venta_detalles (id),
    producto_id BIGINT NOT NULL REFERENCES productos (id),
    cantidad BIGINT NOT NULL CHECK (cantidad > 0),
    -- Snapshot of the product name at sale time: a later rename must not alter
    -- an already issued ticket.
    nombre TEXT NOT NULL
);
CREATE INDEX ix_comanda_items_comanda ON comanda_items (comanda_id);
CREATE UNIQUE INDEX ux_comanda_items_detalle ON comanda_items (comanda_id, venta_detalle_id);

-- Print audit (RF-25). Insert-only, like every history table (RNF-03).
CREATE TABLE impresiones (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    documento_tipo TEXT NOT NULL CHECK (documento_tipo IN ('recibo', 'comanda')),
    documento_id BIGINT NOT NULL,
    venta_id BIGINT NOT NULL REFERENCES ventas (id),
    comanda_id BIGINT REFERENCES comandas (id),
    numero_copia INT NOT NULL DEFAULT 1 CHECK (numero_copia >= 1),
    copia BOOLEAN NOT NULL DEFAULT false,
    usuario_id BIGINT NOT NULL REFERENCES usuarios (id),
    creado_en TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (copia = (numero_copia > 1)),
    CHECK (documento_tipo <> 'comanda' OR comanda_id IS NOT NULL),
    CHECK (documento_tipo <> 'comanda' OR documento_id = comanda_id),
    CHECK (documento_tipo <> 'recibo' OR comanda_id IS NULL),
    CHECK (documento_tipo <> 'recibo' OR documento_id = venta_id)
);
CREATE UNIQUE INDEX ux_impresiones_documento_copia
    ON impresiones (documento_tipo, documento_id, numero_copia);
CREATE INDEX ix_impresiones_documento ON impresiones (documento_tipo, documento_id);
CREATE INDEX ix_impresiones_venta ON impresiones (venta_id);

CREATE TRIGGER trg_impresiones_inmutable
    BEFORE UPDATE OR DELETE ON impresiones
    FOR EACH ROW EXECUTE FUNCTION rechazar_mutacion_historial();
