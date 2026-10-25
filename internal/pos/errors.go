package pos

import (
	"errors"
	"fmt"
)

// AppError is a business error safe to expose through the HTTP API.
type AppError struct {
	Code    string
	Message string
	Status  int
}

func (e *AppError) Error() string { return e.Message }

func appError(code, message string, status int) *AppError {
	return &AppError{Code: code, Message: message, Status: status}
}

// AsAppError extracts an AppError from an error chain.
func AsAppError(err error) (*AppError, bool) {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr, true
	}
	return nil, false
}

var (
	ErrVentaSinItems   = appError("venta_sin_items", "La venta debe tener al menos un ítem", 422)
	ErrCantidad        = appError("cantidad_invalida", "La cantidad debe ser un entero mayor que cero", 422)
	ErrMonto           = appError("monto_invalido", "El monto debe ser un entero válido", 422)
	ErrCajaRequerida   = appError("caja_requerida", "Debe abrir caja para cobrar en efectivo", 409)
	ErrSumaPagos       = appError("suma_pagos_invalida", "La suma de los pagos confirmados no coincide con el total", 409)
	ErrCambio          = appError("efectivo_insuficiente", "El efectivo recibido es menor que el monto en efectivo a pagar", 422)
	ErrCajaAbierta     = appError("caja_ya_abierta", "Ya hay una caja abierta", 409)
	ErrSinCaja         = appError("sin_caja_abierta", "No hay caja abierta", 409)
	ErrSesionNoExiste  = appError("sesion_no_encontrada", "La sesión de caja no existe", 404)
	ErrTipoMovimiento  = appError("tipo_movimiento_invalido", "El tipo de movimiento debe ser entrada o salida", 422)
	ErrMotivoRequerido = appError("motivo_requerido", "El motivo es obligatorio", 422)
	ErrMetodoPago      = appError("metodo_pago_invalido", "Método de pago no disponible", 422)
	ErrVentaNoExiste   = appError("venta_no_encontrada", "La venta no existe", 404)
	ErrParteNoExiste   = appError("parte_no_encontrada", "La parte de pago no existe", 404)
	ErrNoDeposito      = appError("sin_deposito", "No hay depósito disponible", 409)
	ErrNoAutorizado    = appError("no_autorizado", "No autorizado para realizar esta operación", 403)
	ErrVentaAnulada    = appError("venta_anulada", "La venta está anulada", 409)
	ErrVentaCerrada    = appError("venta_cerrada", "La venta ya no está en proceso", 409)
	ErrDatosVenta      = appError("venta_invalida", "La venta no coincide con su detalle", 409)

	ErrAreaNombreRequerido = appError("area_nombre_requerido", "El nombre del área es obligatorio", 422)
	ErrAreaNombreDuplicado = appError("area_nombre_duplicado", "El nombre del área ya existe", 409)
	ErrAreaOrdenInvalido   = appError("area_orden_invalido", "El orden de impresión debe ser un entero mayor o igual a cero", 422)
	ErrAreaNoExiste        = appError("area_no_encontrada", "El área de preparación no existe", 404)
	ErrAreaInvalida        = appError("area_invalida", "El área de preparación es inválida", 422)
	ErrAreaInactiva        = appError("area_inactiva", "El área de preparación está inactiva", 422)
	ErrProductoNoExiste    = appError("producto_no_encontrado", "El producto no existe", 404)
	ErrComandaNoExiste     = appError("comanda_no_encontrada", "La comanda no existe", 404)
	ErrVentaNoCompletada   = appError("venta_no_completada", "La venta no está completada", 409)
	ErrFormatoImpresion    = appError("formato_impresion_invalido", "El formato de impresión debe ser html o texto", 422)

	// T-07 catalog and inventory (RF-01..RF-08, PR-01..PR-04).
	ErrCodigoProductoRequerido = appError("codigo_producto_requerido", "El código del producto es obligatorio", 422)
	ErrCodigoProductoDuplicado = appError("codigo_producto_duplicado", "El código del producto ya existe", 409)
	ErrNombreProductoRequerido = appError("nombre_producto_requerido", "El nombre del producto es obligatorio", 422)
	ErrStockMinimoInvalido     = appError("stock_minimo_invalido", "El stock mínimo debe ser un entero mayor o igual a cero", 422)
	ErrPrecioProductoInvalido  = appError("precio_producto_invalido", "El precio de venta debe ser un entero mayor o igual a cero", 422)
	ErrCategoriaNoExiste       = appError("categoria_no_encontrada", "La categoría no existe", 422)
	ErrProductoInactivo        = appError("producto_inactivo", "El producto está inactivo", 422)

	ErrDepositoNombreRequerido = appError("deposito_nombre_requerido", "El nombre del depósito es obligatorio", 422)
	ErrDepositoNombreDuplicado = appError("deposito_nombre_duplicado", "El nombre del depósito ya existe", 409)
	ErrDepositoNoExiste        = appError("deposito_no_encontrado", "El depósito no existe", 404)
	ErrDepositoInactivo        = appError("deposito_inactivo", "El depósito está inactivo", 422)

	ErrProveedorNombreRequerido = appError("proveedor_nombre_requerido", "El nombre del proveedor es obligatorio", 422)
	ErrProveedorNoExiste        = appError("proveedor_no_encontrado", "El proveedor no existe", 404)
	ErrProveedorInactivo        = appError("proveedor_inactivo", "El proveedor está inactivo", 422)
	ErrAsociacionNoExiste       = appError("asociacion_no_encontrada", "La asociación con el proveedor no existe", 404)

	ErrTipoMovimientoInventario = appError("tipo_movimiento_inventario_invalido", "El tipo de movimiento debe ser entrada, salida o ajuste", 422)
	ErrSentidoAjuste            = appError("sentido_ajuste_invalido", "El sentido del ajuste debe ser sube o baja", 422)
	ErrMotivoAjusteRequerido    = appError("motivo_ajuste_requerido", "El motivo del ajuste es obligatorio", 422)
	ErrTrasladoMismoDeposito    = appError("traslado_deposito_invalido", "El depósito de origen y destino deben ser distintos", 422)

	// T-09 authentication, single-role users and logical deactivation
	// (RF-09..RF-12, RNF-02, US-01).
	ErrCredencialesInvalidas  = appError("credenciales_invalidas", "Credenciales inválidas", 401)
	ErrNoAutenticado          = appError("no_autenticado", "Debe iniciar sesión", 401)
	ErrLoginRequerido         = appError("login_requerido", "El login es obligatorio", 422)
	ErrLoginDuplicado         = appError("login_duplicado", "El login ya existe", 409)
	ErrNombreUsuarioRequerido = appError("nombre_usuario_requerido",
		"El nombre completo es obligatorio", 422)
	ErrRolInvalido = appError("rol_invalido",
		"El rol debe ser administrador, encargado o empleado", 422)
	ErrRolNoPermitido = appError("rol_no_permitido",
		"No autorizado para asignar ese rol", 403)
	ErrPasswordLongitud = appError("password_longitud_invalida",
		"La contraseña debe tener entre 8 y 72 caracteres", 422)
	ErrPasswordDebil = appError("password_debil",
		"La contraseña debe incluir al menos una letra y un dígito", 422)
	ErrUsuarioNoExiste = appError("usuario_no_encontrado", "El usuario no existe", 404)
	ErrRolInmutable    = appError("rol_inmutable",
		"No se permite cambiar el rol del usuario", 422)
	ErrAutoDesactivacion = appError("auto_desactivacion",
		"No puede desactivar su propio usuario", 409)
	ErrUltimoDueno = appError("ultimo_dueno",
		"Debe existir al menos un dueño activo", 409)
	ErrBootstrapRequerido = appError("bootstrap_requerido",
		"Configure ADMIN_INICIAL_LOGIN, ADMIN_INICIAL_PASSWORD y ADMIN_INICIAL_NOMBRE para crear el primer dueño", 500)

	// T-08 purchase costs, recipes and margin (RF-27..RF-30, RT-01).
	ErrCostoInvalido             = appError("costo_invalido", "El costo debe ser un entero mayor o igual a cero", 422)
	ErrCostoFechaInvalida        = appError("costo_fecha_invalida", "La fecha del costo debe tener el formato AAAA-MM-DD", 422)
	ErrRecetaItemsRequeridos     = appError("receta_items_requeridos", "La lista de ingredientes es obligatoria", 422)
	ErrRecetaCantidad            = appError("receta_cantidad_invalida", "La cantidad del ingrediente debe ser un entero mayor que cero", 422)
	ErrRecetaAutoingrediente     = appError("receta_autoingrediente", "Un producto no puede ser ingrediente de su propia receta", 422)
	ErrRecetaIngredienteRepetido = appError("receta_ingrediente_repetido", "El ingrediente está repetido en la receta", 422)
	ErrIngredienteNoExiste       = appError("ingrediente_no_encontrado", "El ingrediente no existe", 404)
	ErrIngredienteInactivo       = appError("ingrediente_inactivo", "El ingrediente está inactivo", 422)
	ErrRecetaCiclo               = appError("receta_ciclo", "La receta produciría un ciclo entre productos elaborados", 409)
	ErrRecetaRendimiento         = appError("receta_rendimiento_invalido", "El rendimiento de la receta debe ser un entero mayor que cero", 422)
)

// stockError carries the exact ST-01 warning, including the negative stock
// that would result. The caller must ask for explicit confirmation before
// retrying with ConfirmarStockNegativo=true.
func stockError(restante int64) *AppError {
	return appError("stock_insuficiente",
		fmt.Sprintf("La salida dejará el stock en negativo (quedará en %d). ¿Desea continuar?", restante),
		409)
}
