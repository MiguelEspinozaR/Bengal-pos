import type { ReactNode } from 'react'
import { NavLink, useNavigate } from 'react-router-dom'
import { useAuth } from '../auth/AuthContext'
import { canAccessCaja, canAccessInventario, canAccessUsuarios, roleLabel } from '../lib/roles'

export { roleLabel }

interface AppShellProps {
  children: ReactNode
  headerAction?: ReactNode
}

export function AppShell({ children, headerAction }: AppShellProps) {
  const { usuario, logout } = useAuth()
  const navigate = useNavigate()

  const handleLogout = async () => {
    await logout()
    navigate('/login', { replace: true })
  }

  return (
    <div className="app-shell">
      <header className="app-header">
        <div className="brand">
          <span className="brand-mark" aria-hidden="true">
            B
          </span>
          <span className="brand-name">Bengal POS</span>
        </div>
        {usuario ? (
          <nav className="main-nav" aria-label="Navegación principal">
            <NavLink to="/venta" className={({ isActive }) => (isActive ? 'nav-link active' : 'nav-link')}>
              Venta
            </NavLink>
            {canAccessCaja(usuario.rol) ? (
              <NavLink to="/caja" className={({ isActive }) => (isActive ? 'nav-link active' : 'nav-link')}>
                Caja
              </NavLink>
            ) : null}
            {canAccessInventario(usuario.rol) ? (
              <NavLink to="/inventario" className={({ isActive }) => (isActive ? 'nav-link active' : 'nav-link')}>
                Inventario
              </NavLink>
            ) : null}
            {canAccessUsuarios(usuario.rol) ? (
              <NavLink to="/usuarios" className={({ isActive }) => (isActive ? 'nav-link active' : 'nav-link')}>
                Usuarios
              </NavLink>
            ) : null}
          </nav>
        ) : null}
        {headerAction ? <div className="header-action">{headerAction}</div> : null}
        {usuario ? (
          <div className="user-box">
            <span className="user-name">{usuario.nombre_completo}</span>
            <span className="badge">{roleLabel(usuario.rol)}</span>
            <button type="button" className="btn btn-ghost" onClick={() => void handleLogout()}>
              Cerrar sesión
            </button>
          </div>
        ) : null}
      </header>
      <main className="app-main">{children}</main>
    </div>
  )
}
