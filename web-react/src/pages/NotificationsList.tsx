import { useEffect, useState } from 'react'
import { api, ApiError } from '../api/client'
import type { Notification } from '../api/types'

export default function NotificationsList() {
  const [notifications, setNotifications] = useState<Notification[] | null>(null)
  const [error, setError] = useState<string | null>(null)

  const load = () =>
    api
      .get<Notification[]>('/api/notifications')
      .then(setNotifications)
      .catch((err) => setError(err instanceof Error ? err.message : 'Failed to load notifications'))

  useEffect(() => {
    load()
  }, [])

  async function markRead(id: number) {
    try {
      await api.post(`/api/notifications/${id}/read`)
      load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to mark notification read')
    }
  }

  if (error) return <p className="error">{error}</p>
  if (!notifications) return <p className="loading">Loading…</p>

  return (
    <div className="notifications-list">
      <h1>Notifications</h1>
      {notifications.length === 0 ? (
        <p>No notifications.</p>
      ) : (
        <table>
          <thead>
            <tr>
              <th>Message</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {notifications.map((n) => (
              <tr key={n.ID} className={n.ReadAt ? '' : 'unread'}>
                <td>{n.Message}</td>
                <td>
                  {!n.ReadAt && (
                    <button type="button" onClick={() => markRead(n.ID)}>
                      Mark read
                    </button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  )
}
