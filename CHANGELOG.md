# Changelog

## [0.1.0] - 2026-10-26

### Agregado
- **Ventas**: Pagos divididos (efectivo/tarjeta/QR) con confirmación manual, cambio automático
- **Comandas**: Generación por área de preparación con impresión independiente
- **Caja**: Apertura/cierre, cuadre, entradas/salidas con motivo, historial inmutable
- **Inventario**: Productos, depósitos, stock por ubicación, movimientos, traslados, alertas de bajo stock
- **Proveedores**: Gestión y asociación con productos
- **Costos y recetas**: Costo de compra, recetas para cálculo de costo/margen (solo producto terminado se descuenta)
- **Usuarios y autenticación**: Roles (Dueño/Administrador/Encargado/Empleado), sesión por cookie HttpOnly, baja lógica
- **UI React**: Pantalla de venta, caja, inventario, usuarios, reportes
- **Reportes**: Ventas, inventario, caja, personal con exportación CSV/PDF
- **Flujo de cobro mejorado**: Pago único por defecto, método secundario opcional
- **Integración**: Harness de tests con PostgreSQL efímero para pruebas reproducibles

### Técnico
- Stack: Go 1.26 + React 18 + TypeScript + Vite + PostgreSQL 18
- Historial inmutable con triggers, transacciones atómicas
- Montos en unidad mínima entera (Bs)
- Migraciones SQL versionadas
