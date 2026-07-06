import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../api/client'
import type { Checklist } from '../api/types'

export default function ChecklistList() {
  const [checklists, setChecklists] = useState<Checklist[] | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    api
      .get<Checklist[]>('/api/checklists')
      .then(setChecklists)
      .catch((err) => setError(err instanceof Error ? err.message : 'Failed to load checklists'))
  }, [])

  if (error) return <p className="error">{error}</p>
  if (!checklists) return <p className="loading">Loading…</p>

  return (
    <div className="checklist-list">
      <h1>Checklists</h1>
      <table>
        <thead>
          <tr>
            <th>Name</th>
            <th>Status</th>
          </tr>
        </thead>
        <tbody>
          {checklists.map((c) => (
            <tr key={c.ID}>
              <td>
                <Link to={`/checklists/${c.ID}`}>{c.Name || `#${c.ID}`}</Link>
              </td>
              <td>{c.Status}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
