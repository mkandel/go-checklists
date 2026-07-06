import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { api } from '../api/client'
import type { Template, TemplateDetail as TemplateDetailData } from '../api/types'

export default function TemplateDetail() {
  const { id } = useParams<{ id: string }>()
  const [template, setTemplate] = useState<TemplateDetailData | null>(null)
  const [otherVersions, setOtherVersions] = useState<Template[]>([])
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    Promise.all([api.get<TemplateDetailData>(`/api/templates/${id}`), api.get<Template[]>('/api/templates')])
      .then(([t, all]) => {
        setTemplate(t)
        setOtherVersions(all.filter((o) => o.Name === t.Name && o.ID !== t.ID).sort((a, b) => a.Version - b.Version))
      })
      .catch((err) => setError(err instanceof Error ? err.message : 'Failed to load template'))
  }, [id])

  if (error) return <p className="error">{error}</p>
  if (!template) return <p className="loading">Loading…</p>

  return (
    <div className="template-detail">
      <h1>
        {template.Name} <small>v{template.Version}</small>
      </h1>
      {otherVersions.length > 0 && (
        <p>
          Other versions:{' '}
          {otherVersions.map((v) => (
            <Link key={v.ID} to={`/templates/${v.ID}`} style={{ marginRight: 8 }}>
              v{v.Version}
            </Link>
          ))}
        </p>
      )}
      <table>
        <thead>
          <tr>
            <th>#</th>
            <th>Item</th>
            <th>Validation ref</th>
          </tr>
        </thead>
        <tbody>
          {template.items.map((item) => (
            <tr key={item.ID}>
              <td>{item.Position}</td>
              <td>{item.Name}</td>
              <td>{item.ValidationRef}</td>
            </tr>
          ))}
        </tbody>
      </table>
      <p>
        <Link to="/templates">Back to templates</Link>
      </p>
    </div>
  )
}
