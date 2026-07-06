// Ported from web-react/src/pages/TemplateDetail.tsx. Uses a static ?id=
// route rather than a dynamic path segment — see the checklists/view route
// for the full rationale (SSG can't pre-render per-tenant dynamic ids, and
// this sidesteps DESIGN.md's flagged historical Qwik SSG route-URL bug
// entirely rather than relying on internal/webqwik's SPA fallback).
import { component$, useSignal, useVisibleTask$ } from '@builder.io/qwik'
import type { DocumentHead } from '@builder.io/qwik-city'
import { Link, useLocation } from '@builder.io/qwik-city'
import { api } from '../../../../lib/api/client'
import type { Template, TemplateDetail as TemplateDetailData } from '../../../../lib/api/types'

export default component$(() => {
  const loc = useLocation()
  const id = loc.url.searchParams.get('id') ?? ''
  const template = useSignal<TemplateDetailData | null>(null)
  const otherVersions = useSignal<Template[]>([])
  const error = useSignal<string | null>(null)

  useVisibleTask$(async () => {
    try {
      const [t, all] = await Promise.all([
        api.get<TemplateDetailData>(`/api/templates/${id}`),
        api.get<Template[]>('/api/templates'),
      ])
      template.value = t
      otherVersions.value = all.filter((o) => o.Name === t.Name && o.ID !== t.ID).sort((a, b) => a.Version - b.Version)
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to load template'
    }
  })

  if (error.value) return <p class="error">{error.value}</p>
  if (!template.value) return <p class="loading">Loading…</p>

  const t = template.value

  return (
    <div class="template-detail">
      <h1>
        {t.Name} <small>{`v${t.Version}`}</small>
      </h1>
      {otherVersions.value.length > 0 && (
        <p>
          Other versions:{' '}
          {otherVersions.value.map((v) => (
            <Link key={v.ID} href={`/templates/view?id=${v.ID}`} style={{ marginRight: '8px' }}>
              {`v${v.Version}`}
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
          {t.items.map((item) => (
            <tr key={item.ID}>
              <td>{item.Position}</td>
              <td>{item.Name}</td>
              <td>{item.ValidationRef}</td>
            </tr>
          ))}
        </tbody>
      </table>
      <p>
        <Link href="/templates">Back to templates</Link>
      </p>
    </div>
  )
})

export const head: DocumentHead = {
  title: 'Template - ChecklistHQ',
}
