-- T-11: attendance and personal payments (read-only for reports).
-- Both are insert-only history tables per architecture/RNF-03.

CREATE TABLE asistencia (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    usuario_id BIGINT NOT NULL REFERENCES usuarios (id),
    tipo TEXT NOT NULL CHECK (tipo IN ('entrada','salida','descanso_inicio','descanso_fin')),
    fecha_hora TIMESTAMPTZ NOT NULL DEFAULT now(),
    registrado_por BIGINT NOT NULL REFERENCES usuarios (id),
    motivo TEXT,
    correccion_id BIGINT REFERENCES correcciones (id),
    creado_en TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ix_asistencia_usuario_fecha ON asistencia (usuario_id, fecha_hora);
CREATE INDEX ix_asistencia_fecha ON asistencia (fecha_hora);

CREATE TABLE pagos_personal (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    usuario_id BIGINT NOT NULL REFERENCES usuarios (id),
    monto_bs BIGINT NOT NULL CHECK (monto_bs > 0),
    fecha_pago DATE NOT NULL,
    concepto TEXT NOT NULL,
    periodo_desde DATE,
    periodo_hasta DATE,
    registrado_por BIGINT NOT NULL REFERENCES usuarios (id),
    correccion_id BIGINT REFERENCES correcciones (id),
    creado_en TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ix_pagos_personal_usuario_fecha ON pagos_personal (usuario_id, fecha_pago);
CREATE INDEX ix_pagos_personal_fecha ON pagos_personal (fecha_pago);

CREATE TRIGGER trg_asistencia_inmutable
    BEFORE UPDATE OR DELETE ON asistencia
    FOR EACH ROW EXECUTE FUNCTION rechazar_mutacion_historial();

CREATE TRIGGER trg_pagos_personal_inmutable
    BEFORE UPDATE OR DELETE ON pagos_personal
    FOR EACH ROW EXECUTE FUNCTION rechazar_mutacion_historial();
