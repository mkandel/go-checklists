// Ported from web-react/src/pages/AdminUsersList.tsx.
import { $, component$, useSignal, useVisibleTask$ } from '@builder.io/qwik'
import type { DocumentHead } from '@builder.io/qwik-city'
import { ApiError, api } from '../../../../lib/api/client'
import type { BulkCreateUserResult, User } from '../../../../lib/api/types'

type SortColumn = 'name' | 'username' | 'email' | 'is_admin' | 'is_active'

export default component$(() => {
  const users = useSignal<User[] | null>(null)
  const sort = useSignal<SortColumn>('name')
  const dir = useSignal<'asc' | 'desc'>('asc')
  const showInactive = useSignal(false)
  const error = useSignal<string | null>(null)

  const username = useSignal('')
  const name = useSignal('')
  const email = useSignal('')
  const password = useSignal('')
  const isAdmin = useSignal(false)

  const bulkResults = useSignal<BulkCreateUserResult[] | null>(null)

  const load = $(async () => {
    try {
      users.value = await api.get<User[]>(
        `/api/admin/users?sort=${sort.value}&dir=${dir.value}&show_inactive=${showInactive.value ? 1 : 0}`,
      )
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to load users'
    }
  })

  useVisibleTask$(({ track }) => {
    track(() => sort.value)
    track(() => dir.value)
    track(() => showInactive.value)
    void load()
  })

  const sortIndicator = (column: SortColumn) => (sort.value === column ? (dir.value === 'asc' ? ' ▲' : ' ▼') : '')
  const toggleSort = $((column: SortColumn) => {
    if (sort.value === column) dir.value = dir.value === 'asc' ? 'desc' : 'asc'
    else {
      sort.value = column
      dir.value = 'asc'
    }
  })

  const createUser = $(async () => {
    error.value = null
    try {
      await api.post('/api/admin/users', {
        username: username.value,
        name: name.value,
        email: email.value || undefined,
        password: password.value,
        is_admin: isAdmin.value,
      })
      username.value = ''
      name.value = ''
      email.value = ''
      password.value = ''
      isAdmin.value = false
      await load()
    } catch (err) {
      error.value = err instanceof ApiError ? err.message : 'Failed to create user'
    }
  })

  const setActive = $(async (user: User, active: boolean) => {
    if (!active && !confirm(`Suspend ${user.Username}?`)) return
    try {
      await api.post(`/api/admin/users/${user.ID}/active`, { active })
      await load()
    } catch (err) {
      error.value = err instanceof ApiError ? err.message : 'Failed to update user'
    }
  })

  const handleBulkUpload = $(async (_: Event, el: HTMLInputElement) => {
    const file = el.files?.[0]
    if (!file) return
    const text = await file.text()
    el.value = ''
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
      bulkResults.value = await res.json()
      await load()
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Bulk upload failed'
    }
  })

  if (error.value) return <p class="error">{error.value}</p>
  if (!users.value) return <p class="loading">Loading…</p>

  const column = (col: SortColumn, label: string) => (
    <th>
      <a href="#" onClick$={(e) => { e.preventDefault(); toggleSort(col) }}>
        {`${label}${sortIndicator(col)}`}
      </a>
    </th>
  )

  return (
    <div class="admin-users">
      <h1>Users</h1>
      {error.value && <div class="error">{error.value}</div>}

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
          {users.value.length === 0 ? (
            <tr>
              <td colSpan={6}>No users yet.</td>
            </tr>
          ) : (
            users.value.map((u) => (
              <tr key={u.ID}>
                <td>{u.Name}</td>
                <td>{u.Username}</td>
                <td>{u.Email}</td>
                <td>{u.IsAdmin ? 'Yes' : ''}</td>
                <td>{u.IsActive ? 'Yes' : ''}</td>
                <td>
                  {u.IsActive ? (
                    <button type="button" onClick$={() => setActive(u, false)}>
                      Suspend
                    </button>
                  ) : (
                    <button type="button" onClick$={() => setActive(u, true)}>
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
        <input type="checkbox" checked={showInactive.value} onChange$={(_, el) => (showInactive.value = el.checked)} />
        Show inactive users
      </label>

      <h2>Create user</h2>
      <form onSubmit$={createUser} preventdefault:submit class="card">
        <label for="username">Username</label>
        <input id="username" value={username.value} onInput$={(_, el) => (username.value = el.value)} required />
        <label for="name">Name</label>
        <input id="name" value={name.value} onInput$={(_, el) => (name.value = el.value)} required />
        <label for="email">Email</label>
        <input id="email" type="email" value={email.value} onInput$={(_, el) => (email.value = el.value)} />
        <label for="password">Password</label>
        <input
          id="password"
          type="password"
          minLength={8}
          value={password.value}
          onInput$={(_, el) => (password.value = el.value)}
          required
        />
        <label>
          <input type="checkbox" checked={isAdmin.value} onChange$={(_, el) => (isAdmin.value = el.checked)} /> Admin
        </label>
        <button type="submit">Create user</button>
      </form>

      <h2>Bulk import (CSV)</h2>
      <p>
        <small>Columns: username,password,name[,is_admin[,email]] — no header row.</small>
      </p>
      <input type="file" accept=".csv,text/csv" onChange$={handleBulkUpload} />
      {bulkResults.value && (
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
            {bulkResults.value.map((r) => (
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
})

export const head: DocumentHead = {
  title: 'Admin users - ChecklistHQ',
}
