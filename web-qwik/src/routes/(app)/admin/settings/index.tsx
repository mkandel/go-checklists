// Ported from web-react/src/pages/AdminSettings.tsx.
import { $, component$, useSignal, useVisibleTask$ } from '@builder.io/qwik'
import type { DocumentHead } from '@builder.io/qwik-city'
import { ApiError, api } from '../../../../lib/api/client'
import type { ChecklistPolicy, Group, TenantMailConfig } from '../../../../lib/api/types'

export default component$(() => {
  return (
    <div class="admin-settings">
      <h1>Settings</h1>
      <MailConfigForm />
      <ChecklistPolicyForm />
    </div>
  )
})

const MailConfigForm = component$(() => {
  const config = useSignal<TenantMailConfig | null>(null)
  const host = useSignal('')
  const port = useSignal('')
  const username = useSignal('')
  const password = useSignal('')
  const fromAddress = useSignal('')
  const error = useSignal<string | null>(null)
  const saved = useSignal(false)

  useVisibleTask$(async () => {
    try {
      const cfg = await api.get<TenantMailConfig>('/api/admin/tenant/mail-config')
      config.value = cfg
      host.value = cfg.host
      port.value = cfg.port ? String(cfg.port) : ''
      username.value = cfg.username
      fromAddress.value = cfg.from_address
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to load mail config'
    }
  })

  const save = $(async () => {
    error.value = null
    saved.value = false
    try {
      await api.put('/api/admin/tenant/mail-config', {
        host: host.value,
        port: Number(port.value),
        username: username.value,
        password: password.value,
        from_address: fromAddress.value,
      })
      password.value = ''
      saved.value = true
      config.value = await api.get<TenantMailConfig>('/api/admin/tenant/mail-config')
    } catch (err) {
      error.value = err instanceof ApiError ? err.message : 'Failed to save mail config'
    }
  })

  if (!config.value) return <p class="loading">Loading…</p>

  return (
    <section>
      <h2>Mail (SMTP)</h2>
      {saved.value && <p class="success">Saved.</p>}
      {error.value && <div class="error">{error.value}</div>}
      <form onSubmit$={save} preventdefault:submit class="stacked card">
        <label for="host">SMTP host</label>
        <input id="host" value={host.value} onInput$={(_, el) => (host.value = el.value)} required />
        <label for="port">SMTP port</label>
        <input id="port" type="number" value={port.value} onInput$={(_, el) => (port.value = el.value)} required />
        <label for="username">SMTP username</label>
        <input id="username" value={username.value} onInput$={(_, el) => (username.value = el.value)} required />
        <label for="password">{`SMTP password${config.value.configured ? ' (leave blank to keep existing)' : ''}`}</label>
        <input
          id="password"
          type="password"
          value={password.value}
          onInput$={(_, el) => (password.value = el.value)}
          required={!config.value.configured}
        />
        <label for="from_address">From address</label>
        <input
          id="from_address"
          value={fromAddress.value}
          onInput$={(_, el) => (fromAddress.value = el.value)}
          required
        />
        <button type="submit">Save</button>
      </form>
    </section>
  )
})

const ChecklistPolicyForm = component$(() => {
  const policy = useSignal<ChecklistPolicy | null>(null)
  const groups = useSignal<Group[]>([])
  const restrict = useSignal(false)
  const creatorGroupId = useSignal('')
  const error = useSignal<string | null>(null)
  const saved = useSignal(false)

  useVisibleTask$(async () => {
    try {
      const [p, gs] = await Promise.all([
        api.get<ChecklistPolicy>('/api/admin/tenant/checklist-policy'),
        api.get<Group[]>('/api/groups'),
      ])
      policy.value = p
      groups.value = gs
      restrict.value = p.restrict
      creatorGroupId.value = p.creator_group_id != null ? String(p.creator_group_id) : ''
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to load checklist policy'
    }
  })

  const save = $(async () => {
    error.value = null
    saved.value = false
    if (restrict.value && !creatorGroupId.value) {
      error.value = 'Creator group is required when restrict is enabled'
      return
    }
    try {
      await api.put('/api/admin/tenant/checklist-policy', {
        restrict: restrict.value,
        creator_group_id: creatorGroupId.value ? Number(creatorGroupId.value) : null,
      })
      saved.value = true
    } catch (err) {
      error.value = err instanceof ApiError ? err.message : 'Failed to save checklist policy'
    }
  })

  if (!policy.value) return <p class="loading">Loading…</p>

  return (
    <section>
      <h2>Checklist creation policy</h2>
      {saved.value && <p class="success">Saved.</p>}
      {error.value && <div class="error">{error.value}</div>}
      <form onSubmit$={save} preventdefault:submit class="stacked card">
        <label>
          <input type="checkbox" checked={restrict.value} onChange$={(_, el) => (restrict.value = el.checked)} />
          Restrict checklist creation to admins and members of a designated group
        </label>
        {restrict.value && (
          <div>
            <label for="creator_group_id">Creator group</label>
            <select
              id="creator_group_id"
              value={creatorGroupId.value}
              onChange$={(_, el) => (creatorGroupId.value = el.value)}
            >
              <option value="">Select a group</option>
              {groups.value.map((g) => (
                <option key={g.ID} value={String(g.ID)}>
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
})

export const head: DocumentHead = {
  title: 'Admin settings - ChecklistHQ',
}
