import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../api/client'
import type { Checklist } from '../api/types'

type SortColumn = 'name' | 'status'

export default function ChecklistList() {
  const [checklists, setChecklists] = useState<Checklist[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [sort, setSort] = useState<SortColumn>('name')
  const [dir, setDir] = useState<'asc' | 'desc'>('asc')

  useEffect(() => {
    api
      .get<Checklist[]>(`/api/checklists?sort=${sort}&dir=${dir}`)
      .then(setChecklists)
      .catch((err) => setError(err instanceof Error ? err.message : 'Failed to load checklists'))
  }, [sort, dir])

  if (error) return <p className="error">{error}</p>
  if (!checklists) return <p className="loading">Loading…</p>

  const sortIndicator = (column: SortColumn) => (sort === column ? (dir === 'asc' ? ' ▲' : ' ▼') : '')

  const toggleSort = (column: SortColumn) => {
    if (sort === column) {
      setDir(dir === 'asc' ? 'desc' : 'asc')
    } else {
      setSort(column)
      setDir('asc')
    }
  }

  return (
    <div className="checklist-list">
      <h1>Checklists</h1>
      <table>
        <thead>
          <tr>
            <th>
              <a href="#" onClick={(e) => { e.preventDefault(); toggleSort('name') }}>
                Name{sortIndicator('name')}
              </a>
            </th>
            <th>
              <a href="#" onClick={(e) => { e.preventDefault(); toggleSort('status') }}>
                Status{sortIndicator('status')}
              </a>
            </th>
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
