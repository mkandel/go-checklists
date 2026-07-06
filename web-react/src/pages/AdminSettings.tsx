import { useEffect, useState } from 'react'
import { api, ApiError } from '../api/client'
import type { ChecklistPolicy, Group, TenantMailConfig } from '../api/types'

export default function AdminSettings() {
  return (
    <div className="admin-settings">
      <h1>Settings</h1>
      <MailConfigForm />
      <ChecklistPolicyForm />
    </div>
  )
}

function MailConfigForm() {
  const [config, setConfig] = useState<TenantMailConfig | null>(null)
  const [host, setHost] = useState('')
  const [port, setPort] = useState('')
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [fromAddress, setFromAddress] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [saved, setSaved] = useState(false)

  useEffect(() => {
    api
      .get<TenantMailConfig>('/api/admin/tenant/mail-config')
      .then((cfg) => {
        setConfig(cfg)
        setHost(cfg.host)
        setPort(cfg.port ? String(cfg.port) : '')
        setUsername(cfg.username)
        setFromAddress(cfg.from_address)
      })
      .catch((err) => setError(err instanceof Error ? err.message : 'Failed to load mail config'))
  }, [])

  async function save(e: React.FormEvent) {
    e.preventDefault()
    setError(null)
    setSaved(false)
    try {
      await api.put('/api/admin/tenant/mail-config', {
        host,
        port: Number(port),
        username,
        password,
        from_address: fromAddress,
      })
      setPassword('')
      setSaved(true)
      const cfg = await api.get<TenantMailConfig>('/api/admin/tenant/mail-config')
      setConfig(cfg)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to save mail config')
    }
  }

  if (!config) return <p className="loading">Loading…</p>

  return (
    <section>
      <h2>Mail (SMTP)</h2>
      {saved && <p className="success">Saved.</p>}
      {error && <div className="error">{error}</div>}
      <form onSubmit={save} className="stacked card">
        <label htmlFor="host">SMTP host</label>
        <input id="host" value={host} onChange={(e) => setHost(e.target.value)} required />
        <label htmlFor="port">SMTP port</label>
        <input id="port" type="number" value={port} onChange={(e) => setPort(e.target.value)} required />
        <label htmlFor="username">SMTP username</label>
        <input id="username" value={username} onChange={(e) => setUsername(e.target.value)} required />
        <label htmlFor="password">SMTP password{config.configured && ' (leave blank to keep existing)'}</label>
        <input
          id="password"
          type="password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          required={!config.configured}
        />
        <label htmlFor="from_address">From address</label>
        <input id="from_address" value={fromAddress} onChange={(e) => setFromAddress(e.target.value)} required />
        <button type="submit">Save</button>
      </form>
    </section>
  )
}

function ChecklistPolicyForm() {
  const [policy, setPolicy] = useState<ChecklistPolicy | null>(null)
  const [groups, setGroups] = useState<Group[]>([])
  const [restrict, setRestrict] = useState(false)
  const [creatorGroupId, setCreatorGroupId] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [saved, setSaved] = useState(false)

  useEffect(() => {
    Promise.all([api.get<ChecklistPolicy>('/api/admin/tenant/checklist-policy'), api.get<Group[]>('/api/groups')])
      .then(([p, gs]) => {
        setPolicy(p)
        setGroups(gs)
        setRestrict(p.restrict)
        setCreatorGroupId(p.creator_group_id != null ? String(p.creator_group_id) : '')
      })
      .catch((err) => setError(err instanceof Error ? err.message : 'Failed to load checklist policy'))
  }, [])

  async function save(e: React.FormEvent) {
    e.preventDefault()
    setError(null)
    setSaved(false)
    if (restrict && !creatorGroupId) {
      setError('Creator group is required when restrict is enabled')
      return
    }
    try {
      await api.put('/api/admin/tenant/checklist-policy', {
        restrict,
        creator_group_id: creatorGroupId ? Number(creatorGroupId) : null,
      })
      setSaved(true)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to save checklist policy')
    }
  }

  if (!policy) return <p className="loading">Loading…</p>

  return (
    <section>
      <h2>Checklist creation policy</h2>
      {saved && <p className="success">Saved.</p>}
      {error && <div className="error">{error}</div>}
      <form onSubmit={save} className="stacked card">
        <label>
          <input type="checkbox" checked={restrict} onChange={(e) => setRestrict(e.target.checked)} />
          Restrict checklist creation to admins and members of a designated group
        </label>
        {restrict && (
          <div>
            <label htmlFor="creator_group_id">Creator group</label>
            <select id="creator_group_id" value={creatorGroupId} onChange={(e) => setCreatorGroupId(e.target.value)}>
              <option value="">Select a group</option>
              {groups.map((g) => (
                <option key={g.ID} value={g.ID}>
                  {g.Name}
                </option>
              ))}
            </select>
          </div>
        )}
        <button type="submit">Save</button>
      </form>
    </section>
  )
}
