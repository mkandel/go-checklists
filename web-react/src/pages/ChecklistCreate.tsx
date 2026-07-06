import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, ApiError } from '../api/client'
import type { CreateChecklistRequest, Group, Template, User } from '../api/types'

export default function ChecklistCreate() {
  const navigate = useNavigate()
  const [templates, setTemplates] = useState<Template[]>([])
  const [groups, setGroups] = useState<Group[]>([])
  const [users, setUsers] = useState<User[]>([])
  const [error, setError] = useState<string | null>(null)

  const [templateID, setTemplateID] = useState('')
  const [name, setName] = useState('')
  const [assignedGroupID, setAssignedGroupID] = useState('')
  const [assignedUserID, setAssignedUserID] = useState('')
  const [approverID, setApproverID] = useState('')
  const [hidden, setHidden] = useState(false)
  const [submitting, setSubmitting] = useState(false)

  useEffect(() => {
    Promise.all([api.get<Template[]>('/api/templates'), api.get<Group[]>('/api/groups'), api.get<User[]>('/api/users')])
      .then(([t, g, u]) => {
        setTemplates(t)
        setGroups(g)
        setUsers(u)
      })
      .catch((err) => setError(err instanceof Error ? err.message : 'Failed to load form data'))
  }, [])

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    setError(null)
    setSubmitting(true)
    const payload: CreateChecklistRequest = {
      template_id: Number(templateID),
      name: name || undefined,
      assigned_group_id: assignedGroupID ? Number(assignedGroupID) : null,
      assigned_user_id: assignedUserID ? Number(assignedUserID) : null,
      approver_id: approverID ? Number(approverID) : null,
      hidden,
    }
    try {
      await api.post('/api/checklists', payload)
      navigate('/checklists')
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to create checklist')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="checklist-create">
      <h1>New checklist</h1>
      {error && <div className="error">{error}</div>}
      <form onSubmit={submit} className="card">
        <fieldset>
          <legend>Template</legend>
          <label htmlFor="template_id">Template</label>
          <select id="template_id" value={templateID} onChange={(e) => setTemplateID(e.target.value)} required>
            <option value="">Select a template</option>
            {templates.map((t) => (
              <option key={t.ID} value={t.ID}>
                {t.Name} (v{t.Version})
              </option>
            ))}
          </select>

          <label htmlFor="name">Name</label>
          <input
            type="text"
            id="name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="Defaults to the template's name"
          />
        </fieldset>

        <fieldset>
          <legend>Assignment</legend>
          <label htmlFor="assigned_group_id">Assigned group</label>
          <select id="assigned_group_id" value={assignedGroupID} onChange={(e) => setAssignedGroupID(e.target.value)}>
            <option value="">None</option>
            {groups.map((g) => (
              <option key={g.ID} value={g.ID}>
                {g.Name}
              </option>
            ))}
          </select>

          <label htmlFor="assigned_user_id">Assigned user</label>
          <select id="assigned_user_id" value={assignedUserID} onChange={(e) => setAssignedUserID(e.target.value)}>
            <option value="">None</option>
            {users.map((u) => (
              <option key={u.ID} value={u.ID}>
                {u.Name}
              </option>
            ))}
          </select>
          <p>
            <small>At least one of group/user is required; if both are set, the user must belong to the group.</small>
          </p>

          <label htmlFor="approver_id">Approver</label>
          <select id="approver_id" value={approverID} onChange={(e) => setApproverID(e.target.value)}>
            <option value="">None</option>
            {users.map((u) => (
              <option key={u.ID} value={u.ID}>
                {u.Name}
              </option>
            ))}
          </select>

          <label>
            <input type="checkbox" checked={hidden} onChange={(e) => setHidden(e.target.checked)} /> Hidden (only
            visible to assignee/approver/creator until claimed)
          </label>
        </fieldset>

        <button type="submit" disabled={submitting}>
          Create checklist
        </button>
      </form>
    </div>
  )
}
