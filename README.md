# Bengal POS

Sistema de Punto de Venta (POS) para reposterías, restaurantes y negocios similares. Desarrollado con Go + React + TypeScript + PostgreSQL.

## Estado del proyecto

| Hito | Estado | Descripción |
|---|---|---|
| **M1** | ✅ Completado | Ventas con pagos divididos, comandas por área, harness de integración |
| **M2** | ✅ Completado | Caja completa, inventario, proveedores, costos y recetas |
| **M3** | ✅ Completado | Autenticación por sesión, gestión de usuarios, UI React de venta |
| **M4** | 🚧 Pendiente de push | Reportes + exportación CSV/PDF |

## Características

### Ventas
- Ventas con uno o múltiples ítems
- Pagos divididos (efectivo, tarjeta, QR) con confirmación manual
- Cálculo de cambio en efectivo
- Descuento automático de inventario al completar venta
- Validación de stock negativo con confirmación
- Impresión de comandas por área y recibo

### Caja
- Apertura/cierre de caja con cuadre
- Entradas y salidas de efectivo con motivo
- Historial de sesiones inmutable
- Control de ventas en proceso al cerrar

### Inventario
- Gestión de productos, depósitos y stock por ubicación
- Movimientos de inventario inmutables (entradas/salidas/ajustes/traslados)
- Alertas de bajo stock agrupadas por proveedor
- Gestión de proveedores
- Recetas con cálculo automático de costo y margen

### Usuarios y autenticación
- Roles: Dueño, Administrador, Encargado de área, Empleado
- Autenticación por sesión (cookie HttpOnly)
- Control de acceso basado en roles
- Baja lógica de usuarios con historial intacto

### Reportes
- Ventas por periodo, producto, empleado y método de pago
- Inventario (stock actual, bajo mínimo, movimientos)
- Caja por turno/sesión
- Asistencia y pagos del personal
- Exportación CSV y PDF

## Stack Tecnológico

- **Backend:** Go 1.26+ con net/http
- **Frontend:** React 18 + TypeScript + Vite
- **Base de datos:** PostgreSQL 18
- **Autenticación:** Sesiones con cookies HttpOnly (bcrypt + SHA-256)
- **Testing:** Go testing + Vitest + Testing Library

## Instalación

### Requisitos
- Go 1.26 o superior
- Node.js 22 o superior
- PostgreSQL 18 o superior

### Configuración

1. Clonar el repositorio:
```bash
git clone git@github.com:MiguelEspinozaR/Bengal-pos.git
cd Bengal-pos
```

2. Configurar base de datos PostgreSQL

3. Crear archivo `.env` (opcional):
```env
DATABASE_URL=postgres://usuario:password@localhost:5432/bengalpos?sslmode=disable
SESSION_COOKIE_SECURE=false
ADMIN_INICIAL_LOGIN=admin
ADMIN_INICIAL_PASSWORD=changeme
ADMIN_INICIAL_NOMBRE=Administrador
```

4. Instalar dependencias:
```bash
go mod download
npm install
```

### Desarrollo

**Backend:**
```bash
go run ./cmd/api
```

**Frontend:**
```bash
npm run dev
```

**Tests:**
```bash
go test ./...
npm test
```

## Arquitectura

- **Modo offline-ready:** Base de datos local en el negocio con posibilidad de replicación
- **Historial inmutable:** Todos los movimientos se registran como insert-only
- **Transaccionalidad:** Las ventas se procesan en transacciones atómicas
- **Moneda:** Boliviano (Bs) en unidad mínima entera
- **Sin hardware obligatorio:** Impresión desde navegador

## Documentación

- [`docs/specs-v1.md`](docs/specs-v1.md) - Especificación funcional v1.0
- [`docs/arquitectura.md`](docs/arquitectura.md) - Arquitectura y modelo de datos
- [`docs/reglas-de-negocio.md`](docs/reglas-de-negocio.md) - Reglas de negocio validadas

## Licencia

Proyecto privado - Todos los derechos reservados
