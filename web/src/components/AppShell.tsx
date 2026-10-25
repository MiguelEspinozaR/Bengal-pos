import type { ReactNode } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '../auth/AuthContext'
import type { Rol } from '../types'

const ROLE_LABEL: Record<string, string> = {
  dueno: 'Dueño',
  administrador: 'Administrador',
  encargado: 'Encargado',
  empleado: 'Empleado',
}

export function roleLabel(rol: Rol | string): string {
  return ROLE_LABEL[rol] ?? rol
}

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
