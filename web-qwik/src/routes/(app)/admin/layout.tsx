// Qwik equivalent of web-react's RequireAdmin. Nested inside (app)/layout.tsx,
// so by the time this runs auth.me is guaranteed non-null — this only needs
// to check IsAdmin.
import { Slot, component$, useContext, useVisibleTask$ } from '@builder.io/qwik'
import { useNavigate } from '@builder.io/qwik-city'
import { AuthContext } from '../../../lib/auth/auth-context'

export default component$(() => {
  const auth = useContext(AuthContext)
  const nav = useNavigate()

  useVisibleTask$(({ track }) => {
    track(() => auth.me)
    if (auth.me && !auth.me.IsAdmin) {
      nav('/checklists')
    }
  })

  if (!auth.me || !auth.me.IsAdmin) {
    // Non-admins never see this rendered — useVisibleTask$ above redirects
    // them away as soon as auth.me resolves.
    return null
  }

  return <Slot />
})
