// Ported from web-react/src/pages/ChecklistCreate.tsx.
import { $, component$, useSignal, useVisibleTask$ } from '@builder.io/qwik'
import type { DocumentHead } from '@builder.io/qwik-city'
import { useNavigate } from '@builder.io/qwik-city'
import { ApiError, api } from '../../../../lib/api/client'
import type { CreateChecklistRequest, Group, Template, User } from '../../../../lib/api/types'

export default component$(() => {
  const nav = useNavigate()
  const templates = useSignal<Template[]>([])
  const groups = useSignal<Group[]>([])
  const users = useSignal<User[]>([])
  const error = useSignal<string | null>(null)

  const templateID = useSignal('')
  const name = useSignal('')
  const assignedGroupID = useSignal('')
  const assignedUserID = useSignal('')
  const approverID = useSignal('')
  const hidden = useSignal(false)
  const submitting = useSignal(false)

  useVisibleTask$(async () => {
    try {
      const [t, g, u] = await Promise.all([
        api.get<Template[]>('/api/templates'),
        api.get<Group[]>('/api/groups'),
        api.get<User[]>('/api/users'),
      ])
      templates.value = t
      groups.value = g
      users.value = u
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to load form data'
    }
  })

  const submit = $(async () => {
    error.value = null
    submitting.value = true
    const payload: CreateChecklistRequest = {
      template_id: Number(templateID.value),
      name: name.value || undefined,
      assigned_group_id: assignedGroupID.value ? Number(assignedGroupID.value) : null,
      assigned_user_id: assignedUserID.value ? Number(assignedUserID.value) : null,
      approver_id: approverID.value ? Number(approverID.value) : null,
      hidden: hidden.value,
    }
    try {
      await api.post('/api/checklists', payload)
      await nav('/checklists')
    } catch (err) {
      error.value = err instanceof ApiError ? err.message : 'Failed to create checklist'
    } finally {
      submitting.value = false
    }
  })

  return (
    <div class="checklist-create">
      <h1>New checklist</h1>
      {error.value && <div class="error">{error.value}</div>}
      <form onSubmit$={submit} preventdefault:submit class="card">
        <fieldset>
          <legend>Template</legend>
          <label for="template_id">Template</label>
          <select
            id="template_id"
            value={templateID.value}
            onChange$={(_, el) => (templateID.value = el.value)}
            required
          >
            <option value="">Select a template</option>
            {templates.value.map((t) => (
              <option key={t.ID} value={String(t.ID)}>
                {`${t.Name} (v${t.Version})`}
              </option>
            ))}
          </select>

          <label for="name">Name</label>
          <input
            type="text"
            id="name"
            value={name.value}
            onInput$={(_, el) => (name.value = el.value)}
            placeholder="Defaults to the template's name"
          />
        </fieldset>

        <fieldset>
          <legend>Assignment</legend>
          <label for="assigned_group_id">Assigned group</label>
          <select
            id="assigned_group_id"
            value={assignedGroupID.value}
            onChange$={(_, el) => (assignedGroupID.value = el.value)}
          >
            <option value="">None</option>
            {groups.value.map((g) => (
              <option key={g.ID} value={g.ID}>
                {g.Name}
              </option>
            ))}
          </select>

          <label for="assigned_user_id">Assigned user</label>
          <select
            id="assigned_user_id"
            value={assignedUserID.value}
            onChange$={(_, el) => (assignedUserID.value = el.value)}
          >
            <option value="">None</option>
            {users.value.map((u) => (
              <option key={u.ID} value={u.ID}>
                {u.Name}
              </option>
            ))}
          </select>
          <p>
            <small>At least one of group/user is required; if both are set, the user must belong to the group.</small>
          </p>

          <label for="approver_id">Approver</label>
          <select id="approver_id" value={approverID.value} onChange$={(_, el) => (approverID.value = el.value)}>
            <option value="">None</option>
            {users.value.map((u) => (
              <option key={u.ID} value={u.ID}>
                {u.Name}
              </option>
            ))}
          </select>

          <label>
            <input type="checkbox" checked={hidden.value} onChange$={(_, el) => (hidden.value = el.checked)} /> Hidden
            (only visible to assignee/approver/creator until claimed)
          </label>
        </fieldset>

        <button type="submit" disabled={submitting.value}>
          Create checklist
        </button>
      </form>
    </div>
  )
})

export const head: DocumentHead = {
  title: 'New checklist - ChecklistHQ',
}
