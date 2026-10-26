import { Navigate, Outlet, Route, Routes } from 'react-router-dom'
import { AuthProvider, useAuth } from './auth/AuthContext'
import { AccessDenied } from './components/AccessDenied'
import { RoleGate } from './components/RoleGate'
import { canAccessCaja, canAccessInventario, canAccessUsuarios } from './lib/roles'
import { CajaPage } from './pages/CajaPage'
import { InventarioPage } from './pages/InventarioPage'
import { LoginPage } from './pages/LoginPage'
import { UsuariosPage } from './pages/UsuariosPage'
import { VentaCompletadaPage } from './pages/VentaCompletadaPage'
import { VentaPage } from './pages/VentaPage'

function LoadingScreen() {
  return (
    <div className="app-loading" role="status">
      Cargando sesión…
    </div>
  )
}

function RequireAuth() {
  const { usuario, loading } = useAuth()
  if (loading) return <LoadingScreen />
  if (!usuario) return <Navigate to="/login" replace />
  return <Outlet />
}

function RootRedirect() {
  const { usuario, loading } = useAuth()
  if (loading) return <LoadingScreen />
  return <Navigate to={usuario ? '/venta' : '/login'} replace />
}

export function App() {
  return (
    <AuthProvider>
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route element={<RequireAuth />}>
          <Route path="/venta" element={<VentaPage />} />
          <Route path="/venta/:id/completada" element={<VentaCompletadaPage />} />
          <Route
            path="/caja"
            element={
              <RoleGate allow={canAccessCaja}>
                <CajaPage />
              </RoleGate>
            }
          />
          <Route
            path="/inventario"
            element={
              <RoleGate allow={canAccessInventario}>
                <InventarioPage />
              </RoleGate>
            }
          />
          <Route
            path="/usuarios"
            element={
              <RoleGate allow={canAccessUsuarios}>
                <UsuariosPage />
              </RoleGate>
            }
          />
          <Route path="/acceso-restringido" element={<AccessDenied />} />
        </Route>
        <Route path="/" element={<RootRedirect />} />
        <Route path="*" element={<RootRedirect />} />
      </Routes>
    </AuthProvider>
  )
}
