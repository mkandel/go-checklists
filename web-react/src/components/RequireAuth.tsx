import type { ReactNode } from 'react'
import { Navigate, useLocation } from 'react-router-dom'
import { useAuth } from '../auth/AuthContext'

export default function RequireAuth({ children }: { children: ReactNode }) {
  const { me, loading } = useAuth()
  const location = useLocation()

  if (loading) return <p className="loading">Loading…</p>
  if (!me) return <Navigate to="/login" state={{ from: location }} replace />
  return <>{children}</>
}
