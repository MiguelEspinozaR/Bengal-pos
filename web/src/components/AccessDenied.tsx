import { Link } from 'react-router-dom'
import { AppShell } from './AppShell'

export function AccessDenied() {
  return (
    <AppShell>
      <section className="page">
        <header className="page-header">
          <div>
            <h1>Acceso restringido</h1>
            <p className="muted">No tiene permisos para acceder a esta sección.</p>
          </div>
        </header>
        <div className="card">
          <p>Puede volver a la venta o utilizar el menú de navegación.</p>
          <div className="form-actions">
            <Link className="btn btn-primary" to="/venta">
              Ir a Venta
            </Link>
          </div>
        </div>
      </section>
    </AppShell>
  )
}
