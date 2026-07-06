// Root layout — wraps every route (public and authenticated alike) in
// AuthProvider so any page can read session state via
// useContext(AuthContext), the same role web-react/src/main.tsx's
// <AuthProvider> plays around the whole <App/> tree.
import { Slot, component$ } from '@builder.io/qwik'
import { AuthProvider } from '../lib/auth/auth-context'

export default component$(() => {
  return (
    <AuthProvider>
      <Slot />
    </AuthProvider>
  )
})
