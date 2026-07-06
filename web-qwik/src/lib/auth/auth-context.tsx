// Qwik equivalent of web-react/src/auth/AuthContext.tsx. Qwik has no
// per-render hook state the way React does — a `useStore` proxy provided
// via context is the direct analogue, and its fields mutate in place rather
// than being replaced via a setter, but callers observe the same shape
// (`{ me, loading }`) that React's AuthState exposed.
//
// There's no live SSR here (this is an SSG build — see DESIGN.md), so
// there's no server-rendered session state to hydrate from: the initial
// /api/me check only happens once this component is visible in the
// browser, via useVisibleTask$. Every route sits under the root layout
// (src/routes/layout.tsx), which is what provides this context, so any
// page can read it via useContext(AuthContext).
import {
  Slot,
  component$,
  createContextId,
  useContextProvider,
  useStore,
  useVisibleTask$,
} from '@builder.io/qwik'
import { ApiError, api, login as apiLogin, logout as apiLogout } from '../api/client'
import type { Me } from '../api/types'

export interface AuthStore {
  me: Me | null
  loading: boolean
}

export const AuthContext = createContextId<AuthStore>('auth-context')

export async function refreshAuth(state: AuthStore): Promise<void> {
  try {
    state.me = await api.get<Me>('/api/me')
  } catch (err) {
    if (err instanceof ApiError && err.status === 401) {
      state.me = null
    } else {
      throw err
    }
  } finally {
    state.loading = false
  }
}

export async function login(state: AuthStore, username: string, password: string): Promise<void> {
  await apiLogin(username, password)
  await refreshAuth(state)
}

export async function logout(state: AuthStore): Promise<void> {
  await apiLogout()
  state.me = null
}

export const AuthProvider = component$(() => {
  const state = useStore<AuthStore>({ me: null, loading: true })
  useContextProvider(AuthContext, state)

  useVisibleTask$(async () => {
    await refreshAuth(state)
  })

  return <Slot />
})
