import { createContext, useCallback, useContext, useEffect, useState } from 'react'
import type { ReactNode } from 'react'
import { api, login as apiLogin, logout as apiLogout, ApiError } from '../api/client'
import type { Me } from '../api/types'

interface AuthState {
  me: Me | null
  loading: boolean
  login: (username: string, password: string) => Promise<void>
  logout: () => Promise<void>
}

const AuthContext = createContext<AuthState | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [me, setMe] = useState<Me | null>(null)
  const [loading, setLoading] = useState(true)

  const refresh = useCallback(async () => {
    try {
      setMe(await api.get<Me>('/api/me'))
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setMe(null)
      } else {
        throw err
      }
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    refresh()
  }, [refresh])

  const login = useCallback(
    async (username: string, password: string) => {
      await apiLogin(username, password)
      await refresh()
    },
    [refresh],
  )

  const logout = useCallback(async () => {
    await apiLogout()
    setMe(null)
  }, [])

  return (
    <AuthContext.Provider value={{ me, loading, login, logout }}>
      {children}
    </AuthContext.Provider>
  )
}

export function useAuth(): AuthState {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth must be used within an AuthProvider')
  return ctx
}
