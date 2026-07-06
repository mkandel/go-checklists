// Ported from web-react/src/components/NotificationBadge.tsx. Polling
// only — internal/web's SSE push (GET /notifications/stream) lives on the
// :8081 web server, not under /api/, so it isn't reachable from this SPA
// as currently proxied/served. See NOTES-QWIK.md #1.
import { component$, useSignal, useVisibleTask$ } from '@builder.io/qwik'
import { Link } from '@builder.io/qwik-city'
import { api } from '../../lib/api/client'
import type { Notification } from '../../lib/api/types'

const POLL_INTERVAL_MS = 20_000

export default component$(() => {
  const unread = useSignal(0)

  useVisibleTask$(({ cleanup }) => {
    let cancelled = false
    const poll = () => {
      api
        .get<Notification[]>('/api/notifications')
        .then((notifications) => {
          if (!cancelled) unread.value = notifications.filter((n) => !n.ReadAt).length
        })
        .catch(() => {
          /* transient poll failure — try again next interval */
        })
    }
    poll()
    const timer = setInterval(poll, POLL_INTERVAL_MS)
    cleanup(() => {
      cancelled = true
      clearInterval(timer)
    })
  })

  return (
    <Link href="/notifications">
      Notifications{unread.value > 0 && <span class="badge"> {unread.value}</span>}
    </Link>
  )
})
