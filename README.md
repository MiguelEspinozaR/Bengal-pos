# Bengal POS

Punto de venta para administrar una repostería (adaptable a restaurantes y negocios similares): inventario, proveedores, caja con cuadre, ventas con pagos divididos, comandas por área, costos y reportes.

## Estado

| Fase | Estado |
|---|---|
| Especificación v1.0 (resuelta, sin preguntas abiertas) | ✅ `docs/specs-v1.md` |
| Arquitectura y modelo de datos | ✅ `docs/arquitectura.md` |
| Reglas de negocio | ✅ `docs/reglas-de-negocio.md` |
| Módulo de ventas y pagos divididos (API Go) | 🚧 en curso |
| Módulo de comandas por área | pendiente |
| Caja, inventario, usuarios, reportes, UI React | pendiente |

## Decisiones clave de la v1

- **Stack**: Go (API), React + TypeScript (UI responsive), PostgreSQL.
- **Una sola moneda**: boliviano (Bs), montos en unidad mínima entera (sin decimales flotantes).
- **Historial inmutable**: los movimientos de inventario, caja y auditoría nunca se editan ni se borran; las correcciones se registran como asientos compensadores (solo administrador/dueño).
- **Pagos divididos**: una venta puede pagarse en varias partes (efectivo, tarjeta, QR), cada parte se confirma manualmente; no hay propinas.
- **Cobro en efectivo**: exige una caja abierta; calcula el cambio.
- **Inventario**: al vender se descuenta solo el producto terminado; las recetas se usan para costo y margen.
- **Comandas por área**: cada producto define su área de preparación (cocina, pastelería, …); se imprime un ticket por área.
- **Respaldo**: base de datos local (en el negocio, por LAN) con réplica remota sincronizada; la venta sigue funcionando sin internet.

## Estructura

```
docs/         Especificación, arquitectura y reglas de negocio
cmd/api/      Servidor API (Go)
internal/     Lógica de dominio, persistencia e HTTP
migrations/   Migraciones SQL (PostgreSQL)
```

## Desarrollo

Requisitos: Go 1.26+, PostgreSQL 18+.

```bash
go build ./...
go test ./...
```

Los tests de integración se activan automáticamente con la variable `DATABASE_URL` (base de desarrollo); sin ella se ejecutan solo los tests unitarios.

## Documentación

- [`docs/specs-v1.md`](docs/specs-v1.md) — especificación funcional resuelta (roles, RF/RNF, criterios de aceptación).
- [`docs/arquitectura.md`](docs/arquitectura.md) — componentes, modelo de datos, invariantes y transacciones.
- [`docs/reglas-de-negocio.md`](docs/reglas-de-negocio.md) — reglas validables con sus mensajes exactos.
