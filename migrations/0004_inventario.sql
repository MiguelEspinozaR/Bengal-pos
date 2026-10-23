-- T-07: inventory operation, suppliers and item-supplier associations
-- (RF-01..RF-08, PR-01..PR-04, ST-01/ST-02).
--
-- The base catalog/inventory tables already exist in 0001_init.sql. This
-- migration only adds suppliers, the item-supplier relation and database
-- guards for the sign/type contract of inventory history. Quantities and
-- amounts stay BIGINT (RT-01/RT-02) and history stays insert-only (RNF-03).

CREATE TABLE proveedores (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    nombre TEXT NOT NULL CHECK (btrim(nombre) <> ''),
    contacto_nombre TEXT,
    telefono TEXT,
    email TEXT,
    direccion TEXT,
    activo BOOLEAN NOT NULL DEFAULT true,
    creado_en TIMESTAMPTZ NOT NULL DEFAULT now(),
    actualizado_en TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ix_proveedores_nombre ON proveedores (nombre);

-- An item can have one or more suppliers (RF-07) and at most one principal
-- supplier (RF-04/PR-02). The partial unique index is the final guarantee.
CREATE TABLE producto_proveedores (
    producto_id BIGINT NOT NULL REFERENCES productos (id),
    proveedor_id BIGINT NOT NULL REFERENCES proveedores (id),
    es_principal BOOLEAN NOT NULL DEFAULT false,
    creado_en TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (producto_id, proveedor_id)
);
CREATE UNIQUE INDEX ux_producto_proveedores_principal
    ON producto_proveedores (producto_id)
    WHERE es_principal;
CREATE INDEX ix_producto_proveedores_proveedor
    ON producto_proveedores (proveedor_id);

-- Deposits already carry nombre UNIQUE, ubicacion and activo. The stock
-- projection already has its composite PK and ix_stock_depositos_deposito;
-- add the reverse lookup used when reporting low stock per supplier.
CREATE INDEX IF NOT EXISTS ix_stock_depositos_producto
    ON stock_depositos (producto_id);

-- The server applies the sign of every inventory movement (RT-02): entradas
-- add stock, salidas/ventas remove it and ajustes use an explicit sentido.
-- NOT VALID keeps any pre-existing history intact while enforcing new rows.
ALTER TABLE movimientos_inventario
    ADD CONSTRAINT ck_movinv_entrada_positiva
        CHECK (tipo <> 'entrada' OR cantidad > 0) NOT VALID,
    ADD CONSTRAINT ck_movinv_salida_negativa
        CHECK (tipo <> 'salida' OR cantidad < 0) NOT VALID,
    ADD CONSTRAINT ck_movinv_venta_negativa
        CHECK (tipo <> 'venta' OR cantidad < 0) NOT VALID,
    ADD CONSTRAINT ck_movinv_ajuste_motivo
        CHECK (tipo <> 'ajuste' OR (motivo IS NOT NULL AND btrim(motivo) <> '')) NOT VALID;
