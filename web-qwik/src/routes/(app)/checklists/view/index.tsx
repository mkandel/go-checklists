// Ported from web-react/src/pages/ChecklistDetail.tsx. Uses a static route
// with the checklist id in a query string (?id=) rather than a Qwik City
// dynamic path segment ([id]) — per-tenant checklist ids aren't known at
// build time, and this app's SSG adapter only pre-renders routes with no
// dynamic segments. A ?id= route is itself fully static (one prerendered
// page, same as /checklists or /login) so it sidesteps DESIGN.md's flagged
// historical Qwik SSG bug around client-side URL updates on dynamic routes
// entirely, rather than relying on internal/webqwik's SPA fallback to serve
// the right thing for a hard-loaded /checklists/<id>. See NOTES-QWIK.md.
import { $, component$, useSignal, useVisibleTask$ } from '@builder.io/qwik'
import type { DocumentHead } from '@builder.io/qwik-city'
import { useLocation } from '@builder.io/qwik-city'
import { ApiError, api } from '../../../../lib/api/client'
import { AuthContext } from '../../../../lib/auth/auth-context'
import { useContext } from '@builder.io/qwik'
import type { Checklist, ChecklistItem, Group, User } from '../../../../lib/api/types'

// Mirrors internal/web/checklist_detail.go's buildChecklistPanelData: the
// permission flags are recomputed client-side from the same rules rather
// than sent by the API, since GET /api/checklists/{id} returns the plain
// domain.Checklist.
function responsibleUserFor(c: Checklist, item: ChecklistItem): number | null {
  return item.AssigneeOverrideUserID ?? c.AssignedUserID
}

export default component$(() => {
  const loc = useLocation()
  const id = loc.url.searchParams.get('id') ?? ''
  const auth = useContext(AuthContext)

  const checklist = useSignal<Checklist | null>(null)
  const users = useSignal<User[]>([])
  const groups = useSignal<Group[]>([])
  const error = useSignal<string | null>(null)
  const rejectIds = useSignal<number[]>([])
  const newItemName = useSignal('')
  const newItemRef = useSignal('')
  const dragId = useSignal<number | null>(null)

  const load = $(async () => {
    try {
      const [c, u, g] = await Promise.all([
        api.get<Checklist>(`/api/checklists/${id}`),
        api.get<User[]>('/api/users'),
        api.get<Group[]>('/api/groups'),
      ])
      checklist.value = c
      users.value = u
      groups.value = g
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to load checklist'
    }
  })

  useVisibleTask$(() => {
    void load()
  })

  const runMutation = $(async (fn: () => Promise<Checklist | void>) => {
    error.value = null
    try {
      const updated = await fn()
      if (updated) checklist.value = updated
      else await load()
    } catch (err) {
      error.value = err instanceof ApiError ? err.message : 'Action failed'
    }
  })

  const claim = $(() => runMutation(() => api.post(`/api/checklists/${id}/claim`, {})))
  const checkItem = $((itemId: number) => runMutation(() => api.post<Checklist>(`/api/checklists/${id}/items/${itemId}/check`)))
  const overrideChecked = $((itemId: number, checked: boolean) =>
    runMutation(() => api.put<Checklist>(`/api/checklists/${id}/items/${itemId}/checked`, { checked })),
  )
  const removeItem = $((itemId: number) => {
    if (!confirm('Remove this item?')) return
    runMutation(() => api.del<Checklist>(`/api/checklists/${id}/items/${itemId}`))
  })
  const addItem = $(async () => {
    if (!newItemName.value.trim()) return
    await runMutation(() =>
      api.post<Checklist>(`/api/checklists/${id}/items`, {
        name: newItemName.value,
        validation_ref: newItemRef.value || undefined,
      }),
    )
    newItemName.value = ''
    newItemRef.value = ''
  })
  const approve = $(() => runMutation(() => api.post<Checklist>(`/api/checklists/${id}/approve`)))
  const reject = $(() => runMutation(() => api.post<Checklist>(`/api/checklists/${id}/reject`, { item_ids: rejectIds.value })))
  const toggleReject = $((itemId: number) => {
    const next = new Set(rejectIds.value)
    if (next.has(itemId)) next.delete(itemId)
    else next.add(itemId)
    rejectIds.value = [...next]
  })

  const reorder = $((newOrder: number[]) => runMutation(() => api.put<Checklist>(`/api/checklists/${id}/items/order`, { item_ids: newOrder })))

  const handleDrop = $((targetId: number) => {
    if (dragId.value == null || dragId.value === targetId || !checklist.value) return
    const ids = checklist.value.Items.map((i) => i.ID)
    const from = ids.indexOf(dragId.value)
    const to = ids.indexOf(targetId)
    ids.splice(to, 0, ...ids.splice(from, 1))
    dragId.value = null
    reorder(ids)
  })

  if (error.value) return <p class="error">{error.value}</p>
  if (!checklist.value || !auth.me) return <p class="loading">Loading…</p>

  const c = checklist.value
  const me = auth.me
  const userName = (uid: number | null) => (uid == null ? null : users.value.find((u) => u.ID === uid)?.Name ?? `#${uid}`)
  const assigneeLabel =
    c.AssignedUserID != null
      ? userName(c.AssignedUserID)
      : c.AssignedGroupID != null
        ? groups.value.find((g) => g.ID === c.AssignedGroupID)?.Name ?? `Group #${c.AssignedGroupID}`
        : 'Unassigned'

  const isCreator = me.ID === c.CreatorID
  const isApproverValidating = c.Status === 'validating' && c.ApproverID === me.ID
  const canClaim = c.AssignedUserID == null

  return (
    <div class="checklist-detail card">
      <h1>
        {c.Name || `#${c.ID}`} <small>{c.Status}</small>
      </h1>
      {error.value && <div class="error">{error.value}</div>}

      <dl>
        <dt>Assignee</dt>
        <dd>{assigneeLabel}</dd>
        <dt>Approver</dt>
        <dd>{userName(c.ApproverID) ?? 'None'}</dd>
        <dt>Creator</dt>
        <dd>{userName(c.CreatorID)}</dd>
        <dt>Hidden</dt>
        <dd>{c.Hidden ? 'Yes' : 'No'}</dd>
      </dl>

      {canClaim && (
        <button type="button" onClick$={claim}>
          Claim
        </button>
      )}

      <h2>Items</h2>
      {c.Items.length === 0 ? (
        <p>No items.</p>
      ) : (
        <ul class="checklist-items">
          {c.Items.map((item) => {
            const responsible = responsibleUserFor(c, item)
            const canCheck = c.Status === 'open' && !item.Checked && responsible === me.ID
            return (
              <li
                key={item.ID}
                class={item.Checked ? 'checked' : ''}
                draggable={isCreator}
                onDragStart$={() => (dragId.value = item.ID)}
                onDragOver$={(e) => e.preventDefault()}
                onDrop$={() => handleDrop(item.ID)}
              >
                <input
                  type="checkbox"
                  checked={item.Checked}
                  disabled={!canCheck}
                  onChange$={() => canCheck && checkItem(item.ID)}
                />
                <span>{item.Name}</span>
                {item.ValidationRef && <span class="validation-ref">({item.ValidationRef})</span>}
                <span class="unchecked-label">
                  {item.Checked
                    ? `Checked${userName(item.CheckedBy) ? ` by ${userName(item.CheckedBy)}` : ''}`
                    : `Unchecked${responsible != null ? ` — waiting on ${userName(responsible)}` : ''}`}
                </span>
                {isApproverValidating && (
                  <label>
                    <input
                      type="checkbox"
                      checked={rejectIds.value.includes(item.ID)}
                      onChange$={() => toggleReject(item.ID)}
                    />
                    Reject
                  </label>
                )}
                {isCreator && (
                  <>
                    <button type="button" onClick$={() => overrideChecked(item.ID, !item.Checked)}>
                      {item.Checked ? 'Uncheck (override)' : 'Check (override)'}
                    </button>
                    <button type="button" onClick$={() => removeItem(item.ID)}>
                      Remove
                    </button>
                    <span class="drag-handle">⠿</span>
                  </>
                )}
              </li>
            )
          })}
        </ul>
      )}

      {isApproverValidating && (
        <div class="actions">
          <button type="button" onClick$={reject}>
            Reject selected items
          </button>
          <button type="button" onClick$={approve}>
            Approve
          </button>
        </div>
      )}

      {isCreator && (
        <>
          <h3>Add item</h3>
          <form onSubmit$={addItem} preventdefault:submit class="inline-form">
            <input
              type="text"
              placeholder="Item name"
              value={newItemName.value}
              onInput$={(_, el) => (newItemName.value = el.value)}
              required
            />
            <input
              type="text"
              placeholder="Validation ref (optional)"
              value={newItemRef.value}
              onInput$={(_, el) => (newItemRef.value = el.value)}
            />
            <button type="submit">Add item</button>
          </form>
        </>
      )}
    </div>
  )
})

export const head: DocumentHead = {
  title: 'Checklist - ChecklistHQ',
}
