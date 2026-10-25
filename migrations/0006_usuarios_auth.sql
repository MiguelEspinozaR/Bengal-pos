-- T-09: server-side opaque sessions for real authentication (RNF-02).
--
-- Only the SHA-256 hash of the 32-byte session token is stored. The raw token
-- lives exclusively in the HttpOnly cookie sent to the browser; a database
-- leak therefore does not expose usable sessions. Expired and revoked rows are
-- kept as history and can be pruned by an operational job.
--
-- `usuarios.password_hash` already exists from 0001_init.sql and stores the
-- bcrypt hash (cost 12). No seed users or credentials are added here: the
-- first owner is bootstrapped from ADMIN_INICIAL_* environment variables only
-- when the table is empty.

-- Logins are normalized to lowercase by the API; this index also prevents
-- case-only duplicates created by other paths.
CREATE UNIQUE INDEX ux_usuarios_login_lower ON usuarios (lower(login));

CREATE TABLE sesiones_usuario (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    token_hash BYTEA NOT NULL UNIQUE,
    usuario_id BIGINT NOT NULL REFERENCES usuarios (id),
    creado_en TIMESTAMPTZ NOT NULL DEFAULT now(),
    expira_en TIMESTAMPTZ NOT NULL,
    revocada_en TIMESTAMPTZ
);
CREATE INDEX ix_sesiones_usuario_usuario ON sesiones_usuario (usuario_id);
CREATE INDEX ix_sesiones_usuario_expira ON sesiones_usuario (expira_en);
