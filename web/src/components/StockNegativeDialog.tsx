interface StockNegativeDialogProps {
  message: string
  busy: boolean
  onConfirm: () => void
  onCancel: () => void
}

export function StockNegativeDialog({
  message,
  busy,
  onConfirm,
  onCancel,
}: StockNegativeDialogProps) {
  return (
    <div className="modal-backdrop">
      <div
        className="modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="stock-dialog-title"
        aria-describedby="stock-dialog-message"
      >
        <h2 id="stock-dialog-title">Confirmar stock negativo</h2>
        <p id="stock-dialog-message">{message}</p>
        <p className="muted small">
          Si cancela, la venta no se completa y el carrito permanece intacto.
        </p>
        <div className="modal-actions">
          <button type="button" className="btn btn-ghost" onClick={onCancel} disabled={busy}>
            Cancelar
          </button>
          <button type="button" className="btn btn-danger" onClick={onConfirm} disabled={busy}>
            {busy ? 'Completando…' : 'Continuar y completar'}
          </button>
        </div>
      </div>
    </div>
  )
}
