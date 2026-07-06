import { useEffect, useState } from 'react'
import { api, ApiError } from '../api/client'
import type { BulkCreateUserResult, User } from '../api/types'

type SortColumn = 'name' | 'username' | 'email' | 'is_admin' | 'is_active'

export default function AdminUsersList() {
  const [users, setUsers] = useState<User[] | null>(null)
  const [sort, setSort] = useState<SortColumn>('name')
  const [dir, setDir] = useState<'asc' | 'desc'>('asc')
  const [showInactive, setShowInactive] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const [username, setUsername] = useState('')
  const [name, setName] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [isAdmin, setIsAdmin] = useState(false)

  const [bulkResults, setBulkResults] = useState<BulkCreateUserResult[] | null>(null)

  const load = () =>
    api
      .get<User[]>(`/api/admin/users?sort=${sort}&dir=${dir}&show_inactive=${showInactive ? 1 : 0}`)
      .then(setUsers)
      .catch((err) => setError(err instanceof Error ? err.message : 'Failed to load users'))

  useEffect(() => {
    load()
  }, [sort, dir, showInactive])

  const sortIndicator = (column: SortColumn) => (sort === column ? (dir === 'asc' ? ' ▲' : ' ▼') : '')
  const toggleSort = (column: SortColumn) => {
    if (sort === column) setDir(dir === 'asc' ? 'desc' : 'asc')
    else {
      setSort(column)
      setDir('asc')
    }
  }

  async function createUser(e: React.FormEvent) {
    e.preventDefault()
    setError(null)
    try {
      await api.post('/api/admin/users', { username, name, email: email || undefined, password, is_admin: isAdmin })
      setUsername('')
      setName('')
      setEmail('')
      setPassword('')
      setIsAdmin(false)
      load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to create user')
    }
  }

  async function setActive(user: User, active: boolean) {
    if (!active && !confirm(`Suspend ${user.Username}?`)) return
    try {
      await api.post(`/api/admin/users/${user.ID}/active`, { active })
      load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to update user')
    }
  }

  async function handleBulkUpload(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    if (!file) return
    const text = await file.text()
    e.target.value = ''
    try {
      const res = await fetch('/api/admin/users/bulk', {
        method: 'POST',
        credentials: 'include',
        headers: {
          'Content-Type': 'text/csv',
          'X-CSRF-Token': document.cookie.match(/checklists_csrf=([^;]+)/)?.[1] ?? '',
        },
        body: text,
      })
      if (!res.ok) throw new Error(await res.text())
      setBulkResults(await res.json())
      load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Bulk upload failed')
    }
  }

  if (error) return <p className="error">{error}</p>
  if (!users) return <p className="loading">Loading…</p>

  const column = (col: SortColumn, label: string) => (
    <th>
      <a href="#" onClick={(e) => { e.preventDefault(); toggleSort(col) }}>
        {label}{sortIndicator(col)}
      </a>
    </th>
  )

  return (
    <div className="admin-users">
      <h1>Users</h1>
      {error && <div className="error">{error}</div>}

      <table>
        <thead>
          <tr>
            {column('name', 'Name')}
            {column('username', 'Username')}
            {column('email', 'Email')}
            {column('is_admin', 'Admin')}
            {column('is_active', 'Active')}
            <th>Actions</th>
          </tr>
        </thead>
        <tbody>
          {users.length === 0 ? (
            <tr>
              <td colSpan={6}>No users yet.</td>
            </tr>
          ) : (
            users.map((u) => (
              <tr key={u.ID}>
                <td>{u.Name}</td>
                <td>{u.Username}</td>
                <td>{u.Email}</td>
                <td>{u.IsAdmin ? 'Yes' : ''}</td>
                <td>{u.IsActive ? 'Yes' : ''}</td>
                <td>
                  {u.IsActive ? (
                    <button type="button" onClick={() => setActive(u, false)}>
                      Suspend
                    </button>
                  ) : (
                    <button type="button" onClick={() => setActive(u, true)}>
                      Reactivate
                    </button>
                  )}
                </td>
              </tr>
            ))
          )}
        </tbody>
      </table>
      <label>
        <input type="checkbox" checked={showInactive} onChange={(e) => setShowInactive(e.target.checked)} />
        Show inactive users
      </label>

      <h2>Create user</h2>
      <form onSubmit={createUser} className="card">
        <label htmlFor="username">Username</label>
        <input id="username" value={username} onChange={(e) => setUsername(e.target.value)} required />
        <label htmlFor="name">Name</label>
        <input id="name" value={name} onChange={(e) => setName(e.target.value)} required />
        <label htmlFor="email">Email</label>
        <input id="email" type="email" value={email} onChange={(e) => setEmail(e.target.value)} />
        <label htmlFor="password">Password</label>
        <input id="password" type="password" minLength={8} value={password} onChange={(e) => setPassword(e.target.value)} required />
        <label>
          <input type="checkbox" checked={isAdmin} onChange={(e) => setIsAdmin(e.target.checked)} /> Admin
        </label>
        <button type="submit">Create user</button>
      </form>

      <h2>Bulk import (CSV)</h2>
      <p>
        <small>Columns: username,password,name[,is_admin[,email]] — no header row.</small>
      </p>
      <input type="file" accept=".csv,text/csv" onChange={handleBulkUpload} />
      {bulkResults && (
        <table>
          <thead>
            <tr>
              <th>Row</th>
              <th>Username</th>
              <th>Status</th>
              <th>Error</th>
            </tr>
          </thead>
          <tbody>
            {bulkResults.map((r) => (
              <tr key={r.row}>
                <td>{r.row}</td>
                <td>{r.username}</td>
                <td>{r.status}</td>
                <td>{r.error}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      <h2>Export</h2>
      <p>
        <a href="/api/admin/users/export.csv">Download users.csv</a>
      </p>
    </div>
  )
}
