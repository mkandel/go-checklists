// Qwik equivalent of web-react's RequireAuth + Layout combined: guards every
// route nested under this (app) group and renders the nav shell around it.
// The `(app)` parens are a Qwik City route GROUP — they don't add a URL
// segment, so src/routes/(app)/checklists/index.tsx still serves /checklists.
//
// There's no server session check available at request time (SSG, no live
// Node process — see DESIGN.md), so the guard runs client-side once
// AuthProvider's /api/me check (see src/routes/layout.tsx) resolves.
import { Slot, component$, useContext, useVisibleTask$, $ } from '@builder.io/qwik'
import { Link, useLocation, useNavigate } from '@builder.io/qwik-city'
import { AuthContext, logout as doLogout } from '../../lib/auth/auth-context'
import NotificationBadge from '../../components/notification-badge/notification-badge'

export default component$(() => {
  const auth = useContext(AuthContext)
  const nav = useNavigate()
  const loc = useLocation()

  useVisibleTask$(({ track }) => {
    track(() => auth.loading)
    track(() => auth.me)
    if (!auth.loading && !auth.me) {
      nav('/login')
    }
  })

  const handleLogout = $(async () => {
    await doLogout(auth)
    await nav('/login')
  })

  if (auth.loading) {
    return <p class="loading">Loading…</p>
  }
  if (!auth.me) {
    // useVisibleTask$ above is already redirecting; avoid flashing protected
    // content in the moment before that navigation completes.
    return <p class="loading">Redirecting…</p>
  }

  const isActive = (path: string) => loc.url.pathname === path || loc.url.pathname.startsWith(`${path}/`)

  return (
    <div class="app-shell">
      <header class="app-nav">
        <span class="app-name">ChecklistHQ</span>
        <nav>
          <Link href="/checklists" class={{ active: isActive('/checklists') }}>
            Checklists
          </Link>
          <Link href="/groups" class={{ active: isActive('/groups') }}>
            Groups
          </Link>
          <Link href="/templates" class={{ active: isActive('/templates') }}>
            Templates
          </Link>
          <NotificationBadge />
          {auth.me.IsAdmin && (
            <Link href="/admin/users" class={{ active: isActive('/admin/users') }}>
              Admin
            </Link>
          )}
          {auth.me.IsAdmin && (
            <Link href="/admin/settings" class={{ active: isActive('/admin/settings') }}>
              Settings
            </Link>
          )}
        </nav>
        <div class="app-nav-user">
          <span>{auth.me.Name}</span>
          <button type="button" onClick$={handleLogout}>
            Log out
          </button>
        </div>
      </header>
      <main>
        <Slot />
      </main>
    </div>
  )
})
