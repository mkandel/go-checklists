import type { ReactNode } from 'react'
import { Navigate } from 'react-router-dom'
import { useAuth } from '../auth/AuthContext'

export default function RequireAdmin({ children }: { children: ReactNode }) {
  const { me } = useAuth()
  if (!me?.IsAdmin) return <Navigate to="/checklists" replace />
  return <>{children}</>
}
