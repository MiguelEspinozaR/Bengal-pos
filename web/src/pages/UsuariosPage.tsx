import { useCallback, useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { apiGet, apiPost, apiPut, errorMessage } from '../api/client'
import { AppShell } from '../components/AppShell'
import { useAuth } from '../auth/AuthContext'
import { allowedRolesForCreator, canManageUsuario, canResetPassword, roleLabel } from '../lib/roles'
import type { Rol, UsuarioDetalle } from '../types'

function validatePassword(pw: string): string | null {
  if (pw.length < 8 || pw.length > 72) return 'La contraseña debe tener entre 8 y 72 caracteres.'
  const hasLetter = /[A-Za-z]/.test(pw)
  const hasDigit = /\d/.test(pw)
  if (!hasLetter || !hasDigit) return 'La contraseña debe contener al menos una letra y un dígito.'
  return null
}

export function UsuariosPage() {
  const { usuario, logout } = useAuth()
  const navigate = useNavigate()
  const [usuarios, setUsuarios] = useState<UsuarioDetalle[]>([])
  const [loading, setLoading] = useState(false)
  const [incluirInactivos, setIncluirInactivos] = useState(false)

  const [form, setForm] = useState({ nombre_completo: '', login: '', password: '', rol: '' })
  const [createBusy, setCreateBusy] = useState(false)
  const [createError, setCreateError] = useState<string | null>(null)

  const [editId, setEditId] = useState<number | null>(null)
  const [editNombre, setEditNombre] = useState('')
  const [editBusy, setEditBusy] = useState(false)
  const [editError, setEditError] = useState<string | null>(null)

  const [resetId, setResetId] = useState<number | null>(null)
  const [resetPw, setResetPw] = useState('')
  const [resetBusy, setResetBusy] = useState(false)
  const [resetError, setResetError] = useState<string | null>(null)

  const actorRol = usuario?.rol as Rol | string
  const actorId = usuario?.id ?? 0

  const loadUsuarios = useCallback(async () => {
    setLoading(true)
    try {
      const data = await apiGet<{ usuarios: UsuarioDetalle[] }>(`/api/usuarios?incluir_inactivos=${incluirInactivos ? 'true' : 'false'}`)
      setUsuarios(data.usuarios ?? [])
    } catch {
      setUsuarios([])
    } finally {
      setLoading(false)
    }
  }, [incluirInactivos])

  useEffect(() => {
    void loadUsuarios()
  }, [loadUsuarios])

  const opcionesCrear = allowedRolesForCreator(actorRol)

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault()
    setCreateError(null)
    if (!form.nombre_completo.trim() || !form.login.trim() || !form.password || !form.rol) {
      setCreateError('Todos los campos son obligatorios.')
      return
    }
    if (!opcionesCrear.includes(form.rol as Rol)) {
      setCreateError('Rol no permitido.')
      return
    }
    const pwErr = validatePassword(form.password)
    if (pwErr) {
      setCreateError(pwErr)
      return
    }
    setCreateBusy(true)
    try {
      await apiPost('/api/usuarios', {
        nombre_completo: form.nombre_completo.trim(),
        login: form.login.trim(),
        password: form.password,
        rol: form.rol,
      })
      setForm({ nombre_completo: '', login: '', password: '', rol: '' })
      await loadUsuarios()
    } catch (err) {
      setCreateError(errorMessage(err))
    } finally {
      setCreateBusy(false)
    }
  }

  const iniciarEdicion = (u: UsuarioDetalle) => {
    setEditId(u.id)
    setEditNombre(u.nombre_completo)
    setEditError(null)
  }

  const guardarEdicion = async (e: React.FormEvent) => {
    e.preventDefault()
    if (editId === null) return
    setEditError(null)
    if (!editNombre.trim()) {
      setEditError('Nombre obligatorio.')
      return
    }
    setEditBusy(true)
    try {
      await apiPut(`/api/usuarios/${editId}`, { nombre_completo: editNombre.trim() })
      setEditId(null)
      await loadUsuarios()
    } catch (err) {
      setEditError(errorMessage(err))
    } finally {
      setEditBusy(false)
    }
  }

  const toggleActivo = async (u: UsuarioDetalle) => {
    // deshabilitar auto-desactivación y último dueño (UI)
    if (u.id === actorId && u.activo && actorRol === 'dueno') {
      // puede desactivarse solo si hay otros dueños? backend lo valida; UI avisa vía error
    }
    if (u.rol === 'dueno' && u.activo) {
      // última validación backend; no bloqueamos completamente
    }
    try {
      await apiPut(`/api/usuarios/${u.id}`, { activo: !u.activo })
      await loadUsuarios()
    } catch (err) {
      alert(errorMessage(err))
    }
  }

  const iniciarReset = (u: UsuarioDetalle) => {
    setResetId(u.id)
    setResetPw('')
    setResetError(null)
  }

  const guardarReset = async (e: React.FormEvent) => {
    e.preventDefault()
    if (resetId === null) return
    setResetError(null)
    const pwErr = validatePassword(resetPw)
    if (pwErr) {
      setResetError(pwErr)
      return
    }
    setResetBusy(true)
    try {
      await apiPut(`/api/usuarios/${resetId}`, { password: resetPw })
      const selfReset = resetId === actorId
      setResetId(null)
      setResetPw('')
      // Si resetea su propia contraseña, backend revoca sesión
      if (selfReset) {
        await logout()
        navigate('/login', { replace: true })
        return
      }
      await loadUsuarios()
    } catch (err) {
      setResetError(errorMessage(err))
    } finally {
      setResetBusy(false)
    }
  }

  const canEdit = (u: UsuarioDetalle) => canManageUsuario(actorRol, u.rol) || u.id === actorId
  const canReset = (u: UsuarioDetalle) => canResetPassword(actorRol, u.rol, u.id, actorId)
  const canToggle = (u: UsuarioDetalle) => {
    if (u.id === actorId && u.activo) return false // auto-desactivación
    if (u.rol === 'dueno' && u.activo && u.id !== actorId) return actorRol === 'dueno'
    if (u.rol === 'dueno' && u.activo) return false
    if (actorRol === 'dueno') return u.rol !== 'dueno' || u.id === actorId
    if (actorRol === 'administrador') return u.rol === 'encargado' || u.rol === 'empleado' || u.id === actorId
    return false
  }

  return (
    <AppShell>
      <section className="page">
        <header className="page-header">
          <div>
            <h1>Usuarios y roles</h1>
            <p className="muted">Listado, alta, edición, activación/desactivación y reseteo de contraseña.</p>
          </div>
        </header>

        <div className="grid-two">
          <article className="card">
            <header className="section-header">
              <h2>Nuevo usuario</h2>
            </header>
            <form onSubmit={(e) => void handleCreate(e)} className="stack">
              <div className="form-row">
                <div className="field">
                  <label htmlFor="usr-nombre">Nombre completo</label>
                  <input id="usr-nombre" value={form.nombre_completo} onChange={(e) => setForm({ ...form, nombre_completo: e.target.value })} required />
                </div>
                <div className="field">
                  <label htmlFor="usr-login">Login</label>
                  <input id="usr-login" value={form.login} onChange={(e) => setForm({ ...form, login: e.target.value })} required />
                </div>
              </div>
              <div className="form-row">
                <div className="field">
                  <label htmlFor="usr-pass">Contraseña</label>
                  <input id="usr-pass" type="password" value={form.password} onChange={(e) => setForm({ ...form, password: e.target.value })} required />
                  <span className="muted small">8–72 caracteres, al menos una letra y un dígito.</span>
                </div>
                <div className="field">
                  <label htmlFor="usr-rol">Rol</label>
                  <select id="usr-rol" value={form.rol} onChange={(e) => setForm({ ...form, rol: e.target.value })} required>
                    <option value="">Seleccione</option>
                    {opcionesCrear.map((r) => (
                      <option key={r} value={r}>
                        {roleLabel(r)}
                      </option>
                    ))}
                  </select>
                </div>
              </div>
              {createError ? (
                <p className="form-error" role="alert">
                  {createError}
                </p>
              ) : null}
              <div className="form-actions">
                <button className="btn btn-primary" type="submit" disabled={createBusy}>
                  {createBusy ? 'Creando…' : 'Crear usuario'}
                </button>
              </div>
            </form>
          </article>

          <article className="card">
            <header className="section-header">
              <h2>Listado</h2>
              <label className="inline-actions">
                <input type="checkbox" checked={incluirInactivos} onChange={(e) => setIncluirInactivos(e.target.checked)} />
                Incluir inactivos
              </label>
            </header>
            {loading ? <p className="muted">Cargando…</p> : null}
            <div className="table-wrap">
              <table className="table">
                <thead>
                  <tr>
                    <th>Nombre</th>
                    <th>Login</th>
                    <th>Rol</th>
                    <th>Estado</th>
                    <th>Acciones</th>
                  </tr>
                </thead>
                <tbody>
                  {usuarios.map((u) => (
                    <tr key={u.id}>
                      <td>{u.nombre_completo}</td>
                      <td>{u.login}</td>
                      <td>{roleLabel(u.rol)}</td>
                      <td>{u.activo ? 'Activo' : 'Inactivo'}</td>
                      <td className="inline-actions">
                        {canEdit(u) ? (
                          <button className="btn btn-small" type="button" onClick={() => iniciarEdicion(u)}>
                            Editar nombre
                          </button>
                        ) : null}
                        {canToggle(u) ? (
                          <button className="btn btn-small" type="button" onClick={() => void toggleActivo(u)}>
                            {u.activo ? 'Desactivar' : 'Reactivar'}
                          </button>
                        ) : null}
                        {canReset(u) ? (
                          <button className="btn btn-small" type="button" onClick={() => iniciarReset(u)}>
                            Reset contraseña
                          </button>
                        ) : null}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </article>
        </div>

        {editId !== null ? (
          <article className="card">
            <header className="section-header">
              <h2>Editar nombre</h2>
              <button className="btn btn-small" type="button" onClick={() => setEditId(null)}>
                Cancelar
              </button>
            </header>
            <form onSubmit={(e) => void guardarEdicion(e)} className="stack">
              <div className="field">
                <label htmlFor="edit-nombre">Nombre completo</label>
                <input id="edit-nombre" value={editNombre} onChange={(e) => setEditNombre(e.target.value)} required />
              </div>
              {editError ? (
                <p className="form-error" role="alert">
                  {editError}
                </p>
              ) : null}
              <div className="form-actions">
                <button className="btn btn-primary" type="submit" disabled={editBusy}>
                  {editBusy ? 'Guardando…' : 'Guardar'}
                </button>
              </div>
            </form>
          </article>
        ) : null}

        {resetId !== null ? (
          <article className="card">
            <header className="section-header">
              <h2>Resetear contraseña</h2>
              <button className="btn btn-small" type="button" onClick={() => setResetId(null)}>
                Cancelar
              </button>
            </header>
            <form onSubmit={(e) => void guardarReset(e)} className="stack">
              <div className="field">
                <label htmlFor="reset-pw">Nueva contraseña</label>
                <input id="reset-pw" type="password" value={resetPw} onChange={(e) => setResetPw(e.target.value)} required />
                <span className="muted small">8–72 caracteres, al menos una letra y un dígito.</span>
              </div>
              {resetError ? (
                <p className="form-error" role="alert">
                  {resetError}
                </p>
              ) : null}
              <div className="form-actions">
                <button className="btn btn-primary" type="submit" disabled={resetBusy}>
                  {resetBusy ? 'Reseteando…' : 'Resetear'}
                </button>
              </div>
              <p className="muted small">Confirmación explícita requerida. Si resetea su propia contraseña, la sesión se revoca.</p>
            </form>
          </article>
        ) : null}
      </section>
    </AppShell>
  )
}
