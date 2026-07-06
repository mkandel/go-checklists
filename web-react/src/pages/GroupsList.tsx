import { useEffect, useState } from 'react'
import { api, ApiError } from '../api/client'
import { useAuth } from '../auth/AuthContext'
import type { Group, User } from '../api/types'

function GroupMembers({ group, isAdmin }: { group: Group; isAdmin: boolean }) {
  const [members, setMembers] = useState<User[] | null>(null)
  const [allUsers, setAllUsers] = useState<User[]>([])
  const [showInactive, setShowInactive] = useState(false)
  const [addUserID, setAddUserID] = useState('')
  const [error, setError] = useState<string | null>(null)

  const load = () => {
    Promise.all([api.get<User[]>(`/api/groups/${group.ID}/members`), api.get<User[]>('/api/users')])
      .then(([m, u]) => {
        setMembers(m)
        setAllUsers(u)
      })
      .catch((err) => setError(err instanceof Error ? err.message : 'Failed to load members'))
  }

  useEffect(load, [group.ID])

  if (error) return <p className="error">{error}</p>
  if (!members) return <p className="loading">Loading…</p>

  const memberIds = new Set(members.map((m) => m.ID))
  const available = allUsers.filter((u) => !memberIds.has(u.ID) && (u.IsActive || showInactive))

  async function addMember(e: React.FormEvent) {
    e.preventDefault()
    if (!addUserID) return
    try {
      await api.post(`/api/groups/${group.ID}/members`, { user_id: Number(addUserID) })
      setAddUserID('')
      load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to add member')
    }
  }

  async function removeMember(userID: number, name: string) {
    if (!confirm(`Remove ${name} from this group?`)) return
    try {
      await api.del(`/api/groups/${group.ID}/members/${userID}`)
      load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to remove member')
    }
  }

  return (
    <div className="members">
      <table>
        <thead>
          <tr>
            <th>Name</th>
            <th>Username</th>
            {isAdmin && <th></th>}
          </tr>
        </thead>
        <tbody>
          {members.length === 0 ? (
            <tr>
              <td colSpan={3}>No members yet.</td>
            </tr>
          ) : (
            members.map((m) => (
              <tr key={m.ID}>
                <td>{m.Name}</td>
                <td>{m.Username}</td>
                {isAdmin && (
                  <td>
                    <button type="button" onClick={() => removeMember(m.ID, m.Name)}>
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
        <form onSubmit={addMember}>
          <select value={addUserID} onChange={(e) => setAddUserID(e.target.value)} required>
            <option value="" disabled>
              Add member…
            </option>
            {available.map((u) => (
              <option key={u.ID} value={u.ID}>
                {u.Name} ({u.Username}){!u.IsActive && ' (inactive)'}
              </option>
            ))}
          </select>
          <button type="submit">Add</button>
          <label>
            <input type="checkbox" checked={showInactive} onChange={(e) => setShowInactive(e.target.checked)} />
            Show inactive users
          </label>
        </form>
      )}
    </div>
  )
}

export default function GroupsList() {
  const { me } = useAuth()
  const [groups, setGroups] = useState<Group[] | null>(null)
  const [newGroupName, setNewGroupName] = useState('')
  const [error, setError] = useState<string | null>(null)

  const load = () =>
    api
      .get<Group[]>('/api/groups')
      .then(setGroups)
      .catch((err) => setError(err instanceof Error ? err.message : 'Failed to load groups'))

  useEffect(() => {
    load()
  }, [])

  async function createGroup(e: React.FormEvent) {
    e.preventDefault()
    if (!newGroupName.trim()) return
    try {
      await api.post('/api/groups', { name: newGroupName })
      setNewGroupName('')
      load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to create group')
    }
  }

  if (error) return <p className="error">{error}</p>
  if (!groups) return <p className="loading">Loading…</p>

  const isAdmin = !!me?.IsAdmin

  return (
    <div className="groups-list">
      <h1>Groups</h1>
      {isAdmin && (
        <form onSubmit={createGroup} className="stacked card">
          <label htmlFor="new-group-name">New group name</label>
          <input id="new-group-name" value={newGroupName} onChange={(e) => setNewGroupName(e.target.value)} required />
          <button type="submit">Create group</button>
        </form>
      )}
      {groups.map((g) => (
        <details key={g.ID} className="card">
          <summary>{g.Name}</summary>
          <GroupMembers group={g} isAdmin={isAdmin} />
        </details>
      ))}
    </div>
  )
}
