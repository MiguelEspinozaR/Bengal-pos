-- T-06 complete cash module: closure metadata, sealed sessions and the
-- database guarantee that a closed session never accepts new movements.
-- Amounts stay in BIGINT minor units of Bs (RT-01) and history stays
-- insert-only (RNF-03/RT-04).

-- A closed session must carry the complete cuadre and the closing user/time;
-- an open session must not carry closure data. Existing history is preserved
-- with NOT VALID while every new write is checked.
ALTER TABLE caja_sesiones
    ADD CONSTRAINT ck_caja_cierre_completo CHECK (
        (estado = 'abierta'
            AND fecha_cierre IS NULL
            AND usuario_cierre_id IS NULL
            AND efectivo_esperado_bs IS NULL
            AND efectivo_contado_bs IS NULL
            AND diferencia_bs IS NULL)
        OR
        (estado = 'cerrada'
            AND fecha_cierre IS NOT NULL
            AND usuario_cierre_id IS NOT NULL
            AND efectivo_esperado_bs IS NOT NULL
            AND efectivo_contado_bs IS NOT NULL
            AND diferencia_bs IS NOT NULL)
    ) NOT VALID;

-- The server applies the sign: entradas add cash and salidas remove it. The
-- motivo is mandatory and must contain more than whitespace.
ALTER TABLE caja_movimientos
    ADD CONSTRAINT ck_cajamov_entrada_positiva
        CHECK (tipo <> 'entrada' OR monto_bs > 0) NOT VALID,
    ADD CONSTRAINT ck_cajamov_salida_negativa
        CHECK (tipo <> 'salida' OR monto_bs < 0) NOT VALID,
    ADD CONSTRAINT ck_cajamov_motivo_requerido
        CHECK (tipo NOT IN ('entrada', 'salida') OR (motivo IS NOT NULL AND btrim(motivo) <> '')) NOT VALID;

-- The closing movement is inserted before the session transitions to
-- 'cerrada', so a closed session accepts no inserts at all (including a second
-- 'cierre' row).
CREATE OR REPLACE FUNCTION bloquear_movimiento_caja_cerrada() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    v_estado TEXT;
BEGIN
    SELECT estado INTO v_estado FROM caja_sesiones WHERE id = NEW.sesion_id;
    IF v_estado = 'cerrada' THEN
        RAISE EXCEPTION 'La caja cerrada no admite movimientos.'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_cajamov_sesion_abierta
    BEFORE INSERT ON caja_movimientos
    FOR EACH ROW EXECUTE FUNCTION bloquear_movimiento_caja_cerrada();

-- Sessions are permanent history: a closed session can no longer be changed or
-- removed. The close transition itself (abierta -> cerrada) is allowed.
CREATE OR REPLACE FUNCTION sellar_caja_cerrada() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'El historial de caja no se elimina.'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    IF OLD.estado = 'cerrada' THEN
        RAISE EXCEPTION 'La sesión de caja cerrada está sellada.'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_caja_sesion_sellada
    BEFORE UPDATE OR DELETE ON caja_sesiones
    FOR EACH ROW EXECUTE FUNCTION sellar_caja_cerrada();
