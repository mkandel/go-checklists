import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, ApiError } from '../api/client'
import type { CreateTemplateRequest, TemplateDetail } from '../api/types'

interface DraftItem {
  key: number
  name: string
  validationRef: string
}

// Reorder uses the same native HTML5 drag-and-drop as ChecklistDetail's item
// list (no SortableJS dependency) — see NOTES-QWIK.md.
export default function TemplateCreate() {
  const navigate = useNavigate()
  const [name, setName] = useState('')
  const [items, setItems] = useState<DraftItem[]>([{ key: 0, name: '', validationRef: '' }])
  const [nextKey, setNextKey] = useState(1)
  const [dragKey, setDragKey] = useState<number | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  const addItem = () => {
    setItems((prev) => [...prev, { key: nextKey, name: '', validationRef: '' }])
    setNextKey((k) => k + 1)
  }
  const removeItem = (key: number) => setItems((prev) => prev.filter((i) => i.key !== key))
  const updateItem = (key: number, field: 'name' | 'validationRef', value: string) =>
    setItems((prev) => prev.map((i) => (i.key === key ? { ...i, [field]: value } : i)))

  function handleDrop(targetKey: number) {
    if (dragKey == null || dragKey === targetKey) return
    setItems((prev) => {
      const next = [...prev]
      const from = next.findIndex((i) => i.key === dragKey)
      const to = next.findIndex((i) => i.key === targetKey)
      next.splice(to, 0, ...next.splice(from, 1))
      return next
    })
    setDragKey(null)
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    setError(null)
    setSubmitting(true)
    const payload: CreateTemplateRequest = {
      name,
      items: items.map((i) => ({ name: i.name, validation_ref: i.validationRef || undefined })),
    }
    try {
      const created = await api.post<TemplateDetail>('/api/templates', payload)
      navigate(`/templates/${created.ID}`)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to create template')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="template-create">
      <h1>New template</h1>
      {error && <div className="error">{error}</div>}
      <form onSubmit={submit} className="card">
        <label htmlFor="name">Name</label>
        <input id="name" value={name} onChange={(e) => setName(e.target.value)} required />

        <h2>Items</h2>
        <ul className="checklist-items">
          {items.map((item) => (
            <li
              key={item.key}
              className="drag-item"
              draggable
              onDragStart={() => setDragKey(item.key)}
              onDragOver={(e) => e.preventDefault()}
              onDrop={() => handleDrop(item.key)}
            >
              <span className="drag-handle">⠿</span>
              <input
                type="text"
                value={item.name}
                onChange={(e) => updateItem(item.key, 'name', e.target.value)}
                placeholder="Item name"
                required
              />
              <input
                type="text"
                value={item.validationRef}
                onChange={(e) => updateItem(item.key, 'validationRef', e.target.value)}
                placeholder="Validation ref (optional)"
              />
              <button type="button" onClick={() => removeItem(item.key)}>
                Remove
              </button>
            </li>
          ))}
        </ul>
        <button type="button" onClick={addItem}>
          Add item
        </button>

        <div>
          <button type="submit" disabled={submitting}>
            Create template
          </button>
        </div>
      </form>
    </div>
  )
}
