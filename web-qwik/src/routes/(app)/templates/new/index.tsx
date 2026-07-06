// Ported from web-react/src/pages/TemplateCreate.tsx. Reorder uses native
// HTML5 drag-and-drop, same as the checklist detail item list.
import { $, component$, useSignal } from '@builder.io/qwik'
import type { DocumentHead } from '@builder.io/qwik-city'
import { useNavigate } from '@builder.io/qwik-city'
import { ApiError, api } from '../../../../lib/api/client'
import type { CreateTemplateRequest, TemplateDetail } from '../../../../lib/api/types'

interface DraftItem {
  key: number
  name: string
  validationRef: string
}

export default component$(() => {
  const nav = useNavigate()
  const name = useSignal('')
  const items = useSignal<DraftItem[]>([{ key: 0, name: '', validationRef: '' }])
  const nextKey = useSignal(1)
  const dragKey = useSignal<number | null>(null)
  const error = useSignal<string | null>(null)
  const submitting = useSignal(false)

  const addItem = $(() => {
    items.value = [...items.value, { key: nextKey.value, name: '', validationRef: '' }]
    nextKey.value += 1
  })
  const removeItem = $((key: number) => {
    items.value = items.value.filter((i) => i.key !== key)
  })
  const updateItem = $((key: number, field: 'name' | 'validationRef', value: string) => {
    items.value = items.value.map((i) => (i.key === key ? { ...i, [field]: value } : i))
  })

  const handleDrop = $((targetKey: number) => {
    if (dragKey.value == null || dragKey.value === targetKey) return
    const next = [...items.value]
    const from = next.findIndex((i) => i.key === dragKey.value)
    const to = next.findIndex((i) => i.key === targetKey)
    next.splice(to, 0, ...next.splice(from, 1))
    items.value = next
    dragKey.value = null
  })

  const submit = $(async () => {
    error.value = null
    submitting.value = true
    const payload: CreateTemplateRequest = {
      name: name.value,
      items: items.value.map((i) => ({ name: i.name, validation_ref: i.validationRef || undefined })),
    }
    try {
      const created = await api.post<TemplateDetail>('/api/templates', payload)
      await nav(`/templates/view?id=${created.ID}`)
    } catch (err) {
      error.value = err instanceof ApiError ? err.message : 'Failed to create template'
    } finally {
      submitting.value = false
    }
  })

  return (
    <div class="template-create">
      <h1>New template</h1>
      {error.value && <div class="error">{error.value}</div>}
      <form onSubmit$={submit} preventdefault:submit class="card">
        <label for="name">Name</label>
        <input id="name" value={name.value} onInput$={(_, el) => (name.value = el.value)} required />

        <h2>Items</h2>
        <ul class="checklist-items">
          {items.value.map((item) => (
            <li
              key={item.key}
              class="drag-item"
              draggable
              onDragStart$={() => (dragKey.value = item.key)}
              onDragOver$={(e) => e.preventDefault()}
              onDrop$={() => handleDrop(item.key)}
            >
              <span class="drag-handle">⠿</span>
              <input
                type="text"
                value={item.name}
                onInput$={(_, el) => updateItem(item.key, 'name', el.value)}
                placeholder="Item name"
                required
              />
              <input
                type="text"
                value={item.validationRef}
                onInput$={(_, el) => updateItem(item.key, 'validationRef', el.value)}
                placeholder="Validation ref (optional)"
              />
              <button type="button" onClick$={() => removeItem(item.key)}>
                Remove
              </button>
            </li>
          ))}
        </ul>
        <button type="button" onClick$={addItem}>
          Add item
        </button>

        <div>
          <button type="submit" disabled={submitting.value}>
            Create template
          </button>
        </div>
      </form>
    </div>
  )
})

export const head: DocumentHead = {
  title: 'New template - ChecklistHQ',
}
