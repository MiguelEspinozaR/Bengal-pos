import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react'
import { UNAUTHORIZED_EVENT, apiGet, apiRequest } from '../api/client'
import type { Usuario } from '../types'

interface AuthContextValue {
  usuario: Usuario | null
  loading: boolean
  login: (login: string, password: string) => Promise<void>
  logout: () => Promise<void>
  refresh: () => Promise<void>
}

const AuthContext = createContext<AuthContextValue | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [usuario, setUsuario] = useState<Usuario | null>(null)
  const [loading, setLoading] = useState(true)

  const refresh = useCallback(async () => {
    try {
      const data = await apiGet<{ usuario: Usuario }>('/api/auth/me')
      setUsuario(data.usuario)
    } catch {
      setUsuario(null)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void refresh()
  }, [refresh])

  useEffect(() => {
    const onUnauthorized = () => setUsuario(null)
    window.addEventListener(UNAUTHORIZED_EVENT, onUnauthorized)
    return () => window.removeEventListener(UNAUTHORIZED_EVENT, onUnauthorized)
  }, [])

  const login = useCallback(async (loginValue: string, password: string) => {
    const data = await apiRequest<{ usuario: Usuario }>('/api/auth/login', {
      method: 'POST',
      body: { login: loginValue, password },
      skipAuthRedirect: true,
    })
    setUsuario(data.usuario)
    setLoading(false)
  }, [])

  const logout = useCallback(async () => {
    try {
      await apiRequest<void>('/api/auth/logout', { method: 'POST' })
    } finally {
      setUsuario(null)
    }
  }, [])

  const value = useMemo(
    () => ({ usuario, loading, login, logout, refresh }),
    [usuario, loading, login, logout, refresh],
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth(): AuthContextValue {
  const context = useContext(AuthContext)
  if (!context) {
    throw new Error('useAuth debe usarse dentro de AuthProvider')
  }
  return context
}
