// Ported from web-react/src/pages/ChecklistList.tsx.
import { $, component$, useSignal, useVisibleTask$ } from '@builder.io/qwik'
import type { DocumentHead } from '@builder.io/qwik-city'
import { Link } from '@builder.io/qwik-city'
import { api } from '../../../lib/api/client'
import type { Checklist } from '../../../lib/api/types'

type SortColumn = 'name' | 'status'

export default component$(() => {
  const checklists = useSignal<Checklist[] | null>(null)
  const error = useSignal<string | null>(null)
  const sort = useSignal<SortColumn>('name')
  const dir = useSignal<'asc' | 'desc'>('asc')

  const load = $(async () => {
    try {
      checklists.value = await api.get<Checklist[]>(`/api/checklists?sort=${sort.value}&dir=${dir.value}`)
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to load checklists'
    }
  })

  useVisibleTask$(({ track }) => {
    track(() => sort.value)
    track(() => dir.value)
    void load()
  })

  const sortIndicator = (column: SortColumn) => (sort.value === column ? (dir.value === 'asc' ? ' ▲' : ' ▼') : '')

  const toggleSort = $((column: SortColumn) => {
    if (sort.value === column) {
      dir.value = dir.value === 'asc' ? 'desc' : 'asc'
    } else {
      sort.value = column
      dir.value = 'asc'
    }
  })

  if (error.value) return <p class="error">{error.value}</p>
  if (!checklists.value) return <p class="loading">Loading…</p>

  return (
    <div class="checklist-list">
      <h1>Checklists</h1>
      <table>
        <thead>
          <tr>
            <th>
              <a href="#" onClick$={(e) => { e.preventDefault(); toggleSort('name') }}>
                Name{sortIndicator('name')}
              </a>
            </th>
            <th>
              <a href="#" onClick$={(e) => { e.preventDefault(); toggleSort('status') }}>
                Status{sortIndicator('status')}
              </a>
            </th>
          </tr>
        </thead>
        <tbody>
          {checklists.value.map((c) => (
            <tr key={c.ID}>
              <td>
                <Link href={`/checklists/view?id=${c.ID}`}>{c.Name || `#${c.ID}`}</Link>
              </td>
              <td>{c.Status}</td>
            </tr>
          ))}
        </tbody>
      </table>
      <p>
        <Link href="/checklists/new">New checklist</Link>
      </p>
    </div>
  )
})

export const head: DocumentHead = {
  title: 'Checklists - ChecklistHQ',
}
