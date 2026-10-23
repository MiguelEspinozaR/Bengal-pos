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
	ErrVentaSinItems = appError("venta_sin_items", "La venta debe tener al menos un ítem", 422)
	ErrCantidad      = appError("cantidad_invalida", "La cantidad debe ser un entero mayor que cero", 422)
	ErrMonto         = appError("monto_invalido", "El monto debe ser un entero válido", 422)
	ErrCajaRequerida = appError("caja_requerida", "Debe abrir caja para cobrar en efectivo", 409)
	ErrSumaPagos     = appError("suma_pagos_invalida", "La suma de los pagos confirmados no coincide con el total", 409)
	ErrCambio        = appError("efectivo_insuficiente", "El efectivo recibido es menor que el monto en efectivo a pagar", 422)
	ErrCajaAbierta   = appError("caja_ya_abierta", "Ya hay una caja abierta", 409)
	ErrSinCaja       = appError("sin_caja_abierta", "No hay caja abierta", 409)
	ErrMetodoPago    = appError("metodo_pago_invalido", "Método de pago no disponible", 422)
	ErrVentaNoExiste = appError("venta_no_encontrada", "La venta no existe", 404)
	ErrParteNoExiste = appError("parte_no_encontrada", "La parte de pago no existe", 404)
	ErrNoDeposito    = appError("sin_deposito", "No hay depósito disponible", 409)
	ErrNoAutorizado  = appError("no_autorizado", "No autorizado para realizar esta operación", 403)
	ErrVentaAnulada  = appError("venta_anulada", "La venta está anulada", 409)
	ErrVentaCerrada  = appError("venta_cerrada", "La venta ya no está en proceso", 409)
	ErrDatosVenta    = appError("venta_invalida", "La venta no coincide con su detalle", 409)

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
)

// stockError carries the exact ST-01 warning, including the negative stock
// that would result. The caller must ask for explicit confirmation before
// retrying with ConfirmarStockNegativo=true.
func stockError(restante int64) *AppError {
	return appError("stock_insuficiente",
		fmt.Sprintf("La salida dejará el stock en negativo (quedará en %d). ¿Desea continuar?", restante),
		409)
}
