// Ported from web-react/src/pages/GroupsList.tsx.
import { $, component$, useContext, useSignal, useVisibleTask$ } from '@builder.io/qwik'
import type { DocumentHead } from '@builder.io/qwik-city'
import { ApiError, api } from '../../../lib/api/client'
import { AuthContext } from '../../../lib/auth/auth-context'
import type { Group, User } from '../../../lib/api/types'

interface GroupMembersProps {
  group: Group
  isAdmin: boolean
}

const GroupMembers = component$<GroupMembersProps>(({ group, isAdmin }) => {
  const members = useSignal<User[] | null>(null)
  const allUsers = useSignal<User[]>([])
  const showInactive = useSignal(false)
  const addUserID = useSignal('')
  const error = useSignal<string | null>(null)

  const load = $(async () => {
    try {
      const [m, u] = await Promise.all([
        api.get<User[]>(`/api/groups/${group.ID}/members`),
        api.get<User[]>('/api/users'),
      ])
      members.value = m
      allUsers.value = u
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to load members'
    }
  })

  useVisibleTask$(() => {
    void load()
  })

  const addMember = $(async () => {
    if (!addUserID.value) return
    try {
      await api.post(`/api/groups/${group.ID}/members`, { user_id: Number(addUserID.value) })
      addUserID.value = ''
      await load()
    } catch (err) {
      error.value = err instanceof ApiError ? err.message : 'Failed to add member'
    }
  })

  const removeMember = $(async (userID: number, name: string) => {
    if (!confirm(`Remove ${name} from this group?`)) return
    try {
      await api.del(`/api/groups/${group.ID}/members/${userID}`)
      await load()
    } catch (err) {
      error.value = err instanceof ApiError ? err.message : 'Failed to remove member'
    }
  })

  if (error.value) return <p class="error">{error.value}</p>
  if (!members.value) return <p class="loading">Loading…</p>

  const memberIds = new Set(members.value.map((m) => m.ID))
  const available = allUsers.value.filter((u) => !memberIds.has(u.ID) && (u.IsActive || showInactive.value))

  return (
    <div class="members">
      <table>
        <thead>
          <tr>
            <th>Name</th>
            <th>Username</th>
            {isAdmin && <th></th>}
          </tr>
        </thead>
        <tbody>
          {members.value.length === 0 ? (
            <tr>
              <td colSpan={3}>No members yet.</td>
            </tr>
          ) : (
            members.value.map((m) => (
              <tr key={m.ID}>
                <td>{m.Name}</td>
                <td>{m.Username}</td>
                {isAdmin && (
                  <td>
                    <button type="button" onClick$={() => removeMember(m.ID, m.Name)}>
                      Remove
                    </button>
                  </td>
                )}
              </tr>
            ))
          )}
        </tbody>
      </table>
      {isAdmin && (
        <form onSubmit$={addMember} preventdefault:submit>
          <select value={addUserID.value} onChange$={(_, el) => (addUserID.value = el.value)} required>
            <option value="" disabled>
              Add member…
            </option>
            {available.map((u) => (
              <option key={u.ID} value={String(u.ID)}>
                {`${u.Name} (${u.Username})${!u.IsActive ? ' (inactive)' : ''}`}
              </option>
            ))}
          </select>
          <button type="submit">Add</button>
          <label>
            <input
              type="checkbox"
              checked={showInactive.value}
              onChange$={(_, el) => (showInactive.value = el.checked)}
            />
            Show inactive users
          </label>
        </form>
      )}
    </div>
  )
})

export default component$(() => {
  const auth = useContext(AuthContext)
  const groups = useSignal<Group[] | null>(null)
  const newGroupName = useSignal('')
  const error = useSignal<string | null>(null)

  const load = $(async () => {
    try {
      groups.value = await api.get<Group[]>('/api/groups')
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to load groups'
    }
  })

  useVisibleTask$(() => {
    void load()
  })

  const createGroup = $(async () => {
    if (!newGroupName.value.trim()) return
    try {
      await api.post('/api/groups', { name: newGroupName.value })
      newGroupName.value = ''
      await load()
    } catch (err) {
      error.value = err instanceof ApiError ? err.message : 'Failed to create group'
    }
  })

  if (error.value) return <p class="error">{error.value}</p>
  if (!groups.value) return <p class="loading">Loading…</p>

  const isAdmin = !!auth.me?.IsAdmin

  return (
    <div class="groups-list">
      <h1>Groups</h1>
      {isAdmin && (
        <form onSubmit$={createGroup} preventdefault:submit class="stacked card">
          <label for="new-group-name">New group name</label>
          <input
            id="new-group-name"
            value={newGroupName.value}
            onInput$={(_, el) => (newGroupName.value = el.value)}
            required
          />
          <button type="submit">Create group</button>
        </form>
      )}
      {groups.value.map((g) => (
        <details key={g.ID} class="card">
          <summary>{g.Name}</summary>
          <GroupMembers group={g} isAdmin={isAdmin} />
        </details>
      ))}
    </div>
  )
})

export const head: DocumentHead = {
  title: 'Groups - ChecklistHQ',
}
