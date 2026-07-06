import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../api/client'
import { useAuth } from '../auth/AuthContext'
import type { Template } from '../api/types'

export default function TemplateList() {
  const { me } = useAuth()
  const [templates, setTemplates] = useState<Template[] | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    api
      .get<Template[]>('/api/templates')
      .then(setTemplates)
      .catch((err) => setError(err instanceof Error ? err.message : 'Failed to load templates'))
  }, [])

  if (error) return <p className="error">{error}</p>
  if (!templates) return <p className="loading">Loading…</p>

  const byName = new Map<string, Template[]>()
  for (const t of templates) {
    const list = byName.get(t.Name) ?? []
    list.push(t)
    byName.set(t.Name, list)
  }
  const groups = [...byName.values()].map((versions) => {
    const sorted = [...versions].sort((a, b) => b.Version - a.Version)
    return { latest: sorted[0], versions: sorted }
  })

  return (
    <div className="template-list">
      <h1>Templates</h1>
      {me?.IsAdmin && (
        <p>
          <Link to="/templates/new">New template</Link>
        </p>
      )}
      {groups.length === 0 ? (
        <p>No templates yet.</p>
      ) : (
        <table>
          <thead>
            <tr>
              <th>Name</th>
              <th>Latest version</th>
              <th>Versions</th>
            </tr>
          </thead>
          <tbody>
            {groups.map((g) => (
              <tr key={g.latest.Name}>
                <td>
                  <Link to={`/templates/${g.latest.ID}`}>{g.latest.Name}</Link>
                </td>
                <td>v{g.latest.Version}</td>
                <td>
                  {g.versions.map((v) => (
                    <Link key={v.ID} to={`/templates/${v.ID}`} style={{ marginRight: 8 }}>
                      v{v.Version}
                    </Link>
                  ))}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  )
}
