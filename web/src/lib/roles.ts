import type { Rol } from '../types'

export const ROLES: Rol[] = ['dueno', 'administrador', 'encargado', 'empleado']

const ROLE_LABEL: Record<string, string> = {
  dueno: 'Dueño',
  administrador: 'Administrador',
  encargado: 'Encargado',
  empleado: 'Empleado',
}

export function roleLabel(rol: Rol | string): string {
  return ROLE_LABEL[rol] ?? rol
}

export function canAccessCaja(rol: Rol | string): boolean {
  return rol === 'dueno' || rol === 'administrador'
}

export function canAccessInventario(rol: Rol | string): boolean {
  return rol === 'dueno' || rol === 'administrador' || rol === 'encargado'
}

export function canAccessUsuarios(rol: Rol | string): boolean {
  return rol === 'dueno' || rol === 'administrador'
}

export function canAccessVenta(_rol: Rol | string): boolean {
  return true
}

export function allowedRolesForCreator(actor: Rol | string): Rol[] {
  if (actor === 'dueno') {
    return ['administrador', 'encargado', 'empleado']
  }
  if (actor === 'administrador') {
    return ['encargado', 'empleado']
  }
  return []
}

export function canManageUsuario(actor: Rol | string, targetRol: Rol | string): boolean {
  if (actor === targetRol) {
    return true // puede editar su propio perfil con restricciones
  }
  if (actor === 'dueno') {
    return targetRol !== 'dueno'
  }
  if (actor === 'administrador') {
    return targetRol === 'encargado' || targetRol === 'empleado'
  }
  return false
}

export function canResetPassword(actor: Rol | string, targetRol: Rol | string, targetId: number, actorId: number): boolean {
  if (actor === 'dueno') {
    return targetRol !== 'dueno' || targetId === actorId
  }
  if (actor === 'administrador') {
    if (targetRol === 'dueno' || targetRol === 'administrador') {
      return targetId === actorId
    }
    return targetRol === 'encargado' || targetRol === 'empleado'
  }
  return false
}
