import { Navigate } from 'react-router-dom'
import { useAuth } from '../auth/AuthContext'
import type { Rol } from '../types'
import { canAccessCaja, canAccessInventario, canAccessUsuarios } from '../lib/roles'

interface RoleGateProps {
  allow: (rol: Rol | string) => boolean
  children: React.ReactElement
}

export function RoleGate({ allow, children }: RoleGateProps) {
  const { usuario } = useAuth()
  if (!usuario) {
    return <Navigate to="/login" replace />
  }
  if (!allow(usuario.rol)) {
    return <Navigate to="/acceso-restringido" replace />
  }
  return children
}

export function RequireCaja() {
  return <RoleGate allow={canAccessCaja}><></></RoleGate> // placeholder, used differently
}

export function cajaAllowed(rol: Rol | string) {
  return canAccessCaja(rol)
}

export function inventarioAllowed(rol: Rol | string) {
  return canAccessInventario(rol)
}

export function usuariosAllowed(rol: Rol | string) {
  return canAccessUsuarios(rol)
}
