import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../api/client'
import type { Notification } from '../api/types'

// Polling only — internal/web's SSE push (GET /notifications/stream) lives on
// the :8081 web server, not under /api/, so it isn't reachable from this SPA
// as currently proxied/served. See NOTES-QWIK.md.
const POLL_INTERVAL_MS = 20_000

export default function NotificationBadge() {
  const [unread, setUnread] = useState(0)

  useEffect(() => {
    let cancelled = false
    const poll = () => {
      api
        .get<Notification[]>('/api/notifications')
        .then((notifications) => {
          if (!cancelled) setUnread(notifications.filter((n) => !n.ReadAt).length)
        })
        .catch(() => {
          /* transient poll failure — try again next interval */
        })
    }
    poll()
    const timer = setInterval(poll, POLL_INTERVAL_MS)
    return () => {
      cancelled = true
      clearInterval(timer)
    }
  }, [])

  return (
    <Link to="/notifications">
      Notifications{unread > 0 && <span className="badge"> {unread}</span>}
    </Link>
  )
}
