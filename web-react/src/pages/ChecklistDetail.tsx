import { useEffect, useState } from 'react'
import { useParams } from 'react-router-dom'
import { api } from '../api/client'
import type { Checklist } from '../api/types'

// Read-only for this first vertical slice — claim/check/approve actions are
// deferred to a follow-up sub-branch (see the React SPA plan).
export default function ChecklistDetail() {
  const { id } = useParams<{ id: string }>()
  const [checklist, setChecklist] = useState<Checklist | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    api
      .get<Checklist>(`/api/checklists/${id}`)
      .then(setChecklist)
      .catch((err) => setError(err instanceof Error ? err.message : 'Failed to load checklist'))
  }, [id])

  if (error) return <p className="error">{error}</p>
  if (!checklist) return <p className="loading">Loading…</p>

  return (
    <div className="checklist-detail">
      <h1>
        {checklist.Name || `#${checklist.ID}`} <small>{checklist.Status}</small>
      </h1>
      <ul className="checklist-items">
        {checklist.Items.map((item) => (
          <li key={item.ID} className={item.Checked ? 'checked' : ''}>
            <input type="checkbox" checked={item.Checked} readOnly />
            <span>{item.Name}</span>
          </li>
        ))}
      </ul>
    </div>
  )
}
