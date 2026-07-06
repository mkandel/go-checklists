import { useEffect, useState } from 'react'
import { useParams } from 'react-router-dom'
import { api, ApiError } from '../api/client'
import { useAuth } from '../auth/AuthContext'
import type { Checklist, ChecklistItem, Group, User } from '../api/types'

// Mirrors internal/web/checklist_detail.go's buildChecklistPanelData: the
// permission flags (CanClaim/CanCheck/IsCreator/IsApproverValidating) are
// recomputed client-side from the same rules rather than sent by the API,
// since GET /api/checklists/{id} returns the plain domain.Checklist.
function responsibleUserFor(c: Checklist, item: ChecklistItem): number | null {
  return item.AssigneeOverrideUserID ?? c.AssignedUserID
}

export default function ChecklistDetail() {
  const { id } = useParams<{ id: string }>()
  const { me } = useAuth()
  const [checklist, setChecklist] = useState<Checklist | null>(null)
  const [users, setUsers] = useState<User[]>([])
  const [groups, setGroups] = useState<Group[]>([])
  const [error, setError] = useState<string | null>(null)
  const [rejectIds, setRejectIds] = useState<Set<number>>(new Set())
  const [newItemName, setNewItemName] = useState('')
  const [newItemRef, setNewItemRef] = useState('')
  const [dragId, setDragId] = useState<number | null>(null)

  const load = () => {
    Promise.all([
      api.get<Checklist>(`/api/checklists/${id}`),
      api.get<User[]>('/api/users'),
      api.get<Group[]>('/api/groups'),
    ])
      .then(([c, u, g]) => {
        setChecklist(c)
        setUsers(u)
        setGroups(g)
      })
      .catch((err) => setError(err instanceof Error ? err.message : 'Failed to load checklist'))
  }

  useEffect(load, [id])

  if (error) return <p className="error">{error}</p>
  if (!checklist || !me) return <p className="loading">Loading…</p>

  const userName = (uid: number | null) => (uid == null ? null : users.find((u) => u.ID === uid)?.Name ?? `#${uid}`)
  const assigneeLabel = checklist.AssignedUserID != null
    ? userName(checklist.AssignedUserID)
    : checklist.AssignedGroupID != null
      ? groups.find((g) => g.ID === checklist.AssignedGroupID)?.Name ?? `Group #${checklist.AssignedGroupID}`
      : 'Unassigned'

  const isCreator = me.ID === checklist.CreatorID
  const isApproverValidating = checklist.Status === 'validating' && checklist.ApproverID === me.ID
  const canClaim = checklist.AssignedUserID == null

  async function runMutation(fn: () => Promise<Checklist | void>) {
    setError(null)
    try {
      const updated = await fn()
      if (updated) setChecklist(updated)
      else load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Action failed')
    }
  }

  const claim = () => runMutation(() => api.post(`/api/checklists/${id}/claim`, {}))
  const checkItem = (itemId: number) =>
    runMutation(() => api.post<Checklist>(`/api/checklists/${id}/items/${itemId}/check`))
  const overrideChecked = (itemId: number, checked: boolean) =>
    runMutation(() => api.put<Checklist>(`/api/checklists/${id}/items/${itemId}/checked`, { checked }))
  const removeItem = (itemId: number) => {
    if (!confirm('Remove this item?')) return
    runMutation(() => api.del<Checklist>(`/api/checklists/${id}/items/${itemId}`))
  }
  const addItem = (e: React.FormEvent) => {
    e.preventDefault()
    if (!newItemName.trim()) return
    runMutation(() =>
      api.post<Checklist>(`/api/checklists/${id}/items`, { name: newItemName, validation_ref: newItemRef || undefined }),
    ).then(() => {
      setNewItemName('')
      setNewItemRef('')
    })
  }
  const approve = () => runMutation(() => api.post<Checklist>(`/api/checklists/${id}/approve`))
  const reject = () => runMutation(() => api.post<Checklist>(`/api/checklists/${id}/reject`, { item_ids: [...rejectIds] }))
  const toggleReject = (itemId: number) => {
    setRejectIds((prev) => {
      const next = new Set(prev)
      if (next.has(itemId)) next.delete(itemId)
      else next.add(itemId)
      return next
    })
  }

  // Native HTML5 drag-and-drop, creator-only reorder. No SortableJS dependency
  // in the React app yet — this is the first drag-drop UI in this codebase,
  // see NOTES-QWIK.md for the pattern if porting.
  const reorder = (newOrder: number[]) =>
    runMutation(() => api.put<Checklist>(`/api/checklists/${id}/items/order`, { item_ids: newOrder }))

  function handleDrop(targetId: number) {
    if (dragId == null || dragId === targetId || !checklist) return
    const ids = checklist.Items.map((i) => i.ID)
    const from = ids.indexOf(dragId)
    const to = ids.indexOf(targetId)
    ids.splice(to, 0, ...ids.splice(from, 1))
    setDragId(null)
    reorder(ids)
  }

  return (
    <div className="checklist-detail card">
      <h1>
        {checklist.Name || `#${checklist.ID}`} <small>{checklist.Status}</small>
      </h1>
      {error && <div className="error">{error}</div>}

      <dl>
        <dt>Assignee</dt>
        <dd>{assigneeLabel}</dd>
        <dt>Approver</dt>
        <dd>{userName(checklist.ApproverID) ?? 'None'}</dd>
        <dt>Creator</dt>
        <dd>{userName(checklist.CreatorID)}</dd>
        <dt>Hidden</dt>
        <dd>{checklist.Hidden ? 'Yes' : 'No'}</dd>
      </dl>

      {canClaim && (
        <button type="button" onClick={claim}>
          Claim
        </button>
      )}

      <h2>Items</h2>
      {checklist.Items.length === 0 ? (
        <p>No items.</p>
      ) : (
        <ul className="checklist-items">
          {checklist.Items.map((item) => {
            const responsible = responsibleUserFor(checklist, item)
            const canCheck = checklist.Status === 'open' && !item.Checked && responsible === me.ID
            return (
              <li
                key={item.ID}
                className={item.Checked ? 'checked' : ''}
                draggable={isCreator}
                onDragStart={() => setDragId(item.ID)}
                onDragOver={(e) => e.preventDefault()}
                onDrop={() => handleDrop(item.ID)}
              >
                <input
                  type="checkbox"
                  checked={item.Checked}
                  disabled={!canCheck}
                  onChange={() => canCheck && checkItem(item.ID)}
                />
                <span>{item.Name}</span>
                {item.ValidationRef && <span className="validation-ref">({item.ValidationRef})</span>}
                <span className="unchecked-label">
                  {item.Checked
                    ? `Checked${userName(item.CheckedBy) ? ` by ${userName(item.CheckedBy)}` : ''}`
                    : `Unchecked${responsible != null ? ` — waiting on ${userName(responsible)}` : ''}`}
                </span>
                {isApproverValidating && (
                  <label>
                    <input type="checkbox" checked={rejectIds.has(item.ID)} onChange={() => toggleReject(item.ID)} />
                    Reject
                  </label>
                )}
                {isCreator && (
                  <>
                    <button type="button" onClick={() => overrideChecked(item.ID, !item.Checked)}>
                      {item.Checked ? 'Uncheck (override)' : 'Check (override)'}
                    </button>
                    <button type="button" onClick={() => removeItem(item.ID)}>
                      Remove
                    </button>
                    <span className="drag-handle">⠿</span>
                  </>
                )}
              </li>
            )
          })}
        </ul>
      )}

      {isApproverValidating && (
        <div className="actions">
          <button type="button" onClick={reject}>
            Reject selected items
          </button>
          <button type="button" onClick={approve}>
            Approve
          </button>
        </div>
      )}

      {isCreator && (
        <>
          <h3>Add item</h3>
          <form onSubmit={addItem} className="inline-form">
            <input
              type="text"
              placeholder="Item name"
              value={newItemName}
              onChange={(e) => setNewItemName(e.target.value)}
              required
            />
            <input
              type="text"
              placeholder="Validation ref (optional)"
              value={newItemRef}
              onChange={(e) => setNewItemRef(e.target.value)}
            />
            <button type="submit">Add item</button>
          </form>
        </>
      )}
    </div>
  )
}
