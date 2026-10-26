export type Rol = 'dueno' | 'administrador' | 'encargado' | 'empleado'

export interface Usuario {
  id: number
  nombre_completo: string
  login: string
  rol: Rol | string
}

export interface Producto {
  id: number
  codigo: string
  nombre: string
  precio_venta_bs: number
  activo: boolean
}

export interface Deposito {
  id: number
  nombre: string
  activo: boolean
}

export interface CajaSesion {
  id: number
  estado: string
  fecha_apertura?: string
  usuario_apertura_id?: number
  monto_inicial_bs: number
  fecha_cierre?: string | null
  usuario_cierre_id?: number | null
  efectivo_esperado_bs?: number | null
  efectivo_contado_bs?: number | null
  diferencia_bs?: number | null
}

export interface SaleLine {
  id: number
  producto_id: number
  producto_codigo?: string
  producto_nombre?: string
  cantidad: number
  precio_unitario_bs: number
  descuento_bs: number
  subtotal_bs: number
}

export interface PaymentPart {
  id: number
  venta_id: number
  metodo: string
  metodo_nombre?: string
  monto_bs: number
  estado: 'pendiente' | 'confirmado' | string
  referencia?: string
  cambio_bs: number
}

export interface Sale {
  id: number
  estado: string
  usuario_id: number
  caja_sesion_id?: number
  total_bs: number
  descuento_bs: number
  items: SaleLine[]
  partes: PaymentPart[]
}

export interface ComandaItem {
  id: number
  comanda_id: number
  producto_id: number
  cantidad: number
  nombre: string
}

export interface Comanda {
  id: number
  venta_id: number
  area_preparacion_id: number
  area_nombre: string
  orden_impresion: number
  estado: string
  impresa: boolean
  copia: boolean
  items: ComandaItem[]
}

export interface SaleCompletion {
  venta: Sale
  stock_negativo: boolean
  comandas: Comanda[]
  sin_area: string[]
}

export interface CartLine {
  producto: Producto
  cantidad: number
}

export interface SaleCompletionView {
  completion: SaleCompletion
  cambioTotal: number
}

export interface CajaMovimiento {
  id: number
  sesion_id: number
  tipo: string
  monto_bs: number
  motivo?: string | null
  usuario_id: number
  venta_id?: number | null
  creado_en: string
}

export interface CajaVentaEnProceso {
  id: number
  usuario_id: number
  total_bs: number
  fecha: string
}

export interface CajaCierreResult {
  id: number
  estado: string
  fecha_apertura: string
  usuario_apertura_id: number
  monto_inicial_bs: number
  fecha_cierre?: string | null
  usuario_cierre_id?: number | null
  efectivo_esperado_bs?: number | null
  efectivo_contado_bs?: number | null
  diferencia_bs?: number | null
  aviso?: string
  ventas_en_proceso: number
  ventas_en_proceso_detalle: CajaVentaEnProceso[]
}

export interface DepositoDetalle extends Deposito {
  ubicacion?: string | null
  creado_en?: string
  actualizado_en?: string
}

export interface ProductoDetalle extends Producto {
  unidad?: string | null
  stock_minimo?: number
  categoria_id?: number | null
  area_preparacion_id?: number | null
  creado_en?: string
  actualizado_en?: string
}

export interface StockDeposito {
  deposito_id: number
  deposito_nombre: string
  cantidad: number
}

export interface StockProducto {
  producto_id: number
  producto_codigo: string
  producto_nombre: string
  stock_minimo: number
  stock_total: number
  depositos: StockDeposito[]
}

export interface InventarioMovimiento {
  id: number
  producto_id: number
  producto_codigo?: string
  producto_nombre?: string
  deposito_id: number
  deposito_nombre?: string
  tipo: string
  cantidad: number
  stock_negativo_confirmado: boolean
  grupo_id?: string | null
  venta_id?: number | null
  usuario_id: number
  motivo?: string | null
  creado_en: string
}

export interface MovimientoResultado {
  movimiento: InventarioMovimiento
  stock_resultante: number
}

export interface TrasladoResultado {
  grupo_id: string
  origen: InventarioMovimiento
  destino: InventarioMovimiento
  stock_origen: number
  stock_destino: number
}

export interface BajoStockProducto {
  producto_id: number
  codigo: string
  nombre: string
  stock_minimo: number
  stock_total: number
  depositos: StockDeposito[]
  proveedor_id?: number | null
  proveedor_nombre?: string | null
}

export interface BajoStockGrupo {
  proveedor_id?: number | null
  proveedor_nombre: string
  proveedor_activo?: boolean | null
  productos: BajoStockProducto[]
}

export interface BajoStockReporte {
  grupos: BajoStockGrupo[]
}

export interface UsuarioDetalle {
  id: number
  nombre_completo: string
  login: string
  rol: Rol | string
  activo: boolean
  creado_en?: string
  actualizado_en?: string
}

export interface ApiErrorBody {
  error?: {
    code?: string
    message?: string
  }
}
