// Ported from web-react/src/pages/TemplateList.tsx.
import { component$, useContext, useSignal, useVisibleTask$ } from '@builder.io/qwik'
import type { DocumentHead } from '@builder.io/qwik-city'
import { Link } from '@builder.io/qwik-city'
import { api } from '../../../lib/api/client'
import { AuthContext } from '../../../lib/auth/auth-context'
import type { Template } from '../../../lib/api/types'

export default component$(() => {
  const auth = useContext(AuthContext)
  const templates = useSignal<Template[] | null>(null)
  const error = useSignal<string | null>(null)

  useVisibleTask$(async () => {
    try {
      templates.value = await api.get<Template[]>('/api/templates')
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to load templates'
    }
  })

  if (error.value) return <p class="error">{error.value}</p>
  if (!templates.value) return <p class="loading">Loading…</p>

  const byName = new Map<string, Template[]>()
  for (const t of templates.value) {
    const list = byName.get(t.Name) ?? []
    list.push(t)
    byName.set(t.Name, list)
  }
  const groups = [...byName.values()].map((versions) => {
    const sorted = [...versions].sort((a, b) => b.Version - a.Version)
    return { latest: sorted[0], versions: sorted }
  })

  return (
    <div class="template-list">
      <h1>Templates</h1>
      {auth.me?.IsAdmin && (
        <p>
          <Link href="/templates/new">New template</Link>
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
                  <Link href={`/templates/view?id=${g.latest.ID}`}>{g.latest.Name}</Link>
                </td>
                <td>{`v${g.latest.Version}`}</td>
                <td>
                  {g.versions.map((v) => (
                    <Link key={v.ID} href={`/templates/view?id=${v.ID}`} style={{ marginRight: '8px' }}>
                      {`v${v.Version}`}
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
})

export const head: DocumentHead = {
  title: 'Templates - ChecklistHQ',
}
