// Ported from web-react/src/pages/NotificationsList.tsx.
import { $, component$, useSignal, useVisibleTask$ } from '@builder.io/qwik'
import type { DocumentHead } from '@builder.io/qwik-city'
import { ApiError, api } from '../../../lib/api/client'
import type { Notification } from '../../../lib/api/types'

export default component$(() => {
  const notifications = useSignal<Notification[] | null>(null)
  const error = useSignal<string | null>(null)

  const load = $(async () => {
    try {
      notifications.value = await api.get<Notification[]>('/api/notifications')
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to load notifications'
    }
  })

  useVisibleTask$(() => {
    void load()
  })

  const markRead = $(async (id: number) => {
    try {
      await api.post(`/api/notifications/${id}/read`)
      await load()
    } catch (err) {
      error.value = err instanceof ApiError ? err.message : 'Failed to mark notification read'
    }
  })

  if (error.value) return <p class="error">{error.value}</p>
  if (!notifications.value) return <p class="loading">Loading…</p>

  return (
    <div class="notifications-list">
      <h1>Notifications</h1>
      {notifications.value.length === 0 ? (
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
            {notifications.value.map((n) => (
              <tr key={n.ID} class={n.ReadAt ? '' : 'unread'}>
                <td>{n.Message}</td>
                <td>
                  {!n.ReadAt && (
                    <button type="button" onClick$={() => markRead(n.ID)}>
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
})

export const head: DocumentHead = {
  title: 'Notifications - ChecklistHQ',
}
