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
  monto_inicial_bs: number
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

export interface ApiErrorBody {
  error?: {
    code?: string
    message?: string
  }
}
