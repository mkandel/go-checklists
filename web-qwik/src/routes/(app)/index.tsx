// Equivalent of web-react's <Route path="/" element={<Navigate to="/checklists" replace />} />.
import { component$, useVisibleTask$ } from '@builder.io/qwik'
import type { DocumentHead } from '@builder.io/qwik-city'
import { useNavigate } from '@builder.io/qwik-city'

export default component$(() => {
  const nav = useNavigate()
  useVisibleTask$(async () => {
    await nav('/checklists', { replaceState: true })
  })
  return null
})

export const head: DocumentHead = {
  title: 'ChecklistHQ',
}
