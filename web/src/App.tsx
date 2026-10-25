import { Navigate, Outlet, Route, Routes } from 'react-router-dom'
import { AuthProvider, useAuth } from './auth/AuthContext'
import { LoginPage } from './pages/LoginPage'
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
        </Route>
        <Route path="/" element={<RootRedirect />} />
        <Route path="*" element={<RootRedirect />} />
      </Routes>
    </AuthProvider>
  )
}
