// Ported from web-react/src/pages/PasswordResetConfirm.tsx.
import { $, component$, useSignal } from '@builder.io/qwik'
import type { DocumentHead } from '@builder.io/qwik-city'
import { Link, useLocation } from '@builder.io/qwik-city'
import { ApiError, api } from '../../../lib/api/client'

export default component$(() => {
  const loc = useLocation()
  const token = loc.url.searchParams.get('token') ?? ''
  const password = useSignal('')
  const error = useSignal<string | null>(null)
  const submitting = useSignal(false)

  const handleSubmit = $(async () => {
    error.value = null
    submitting.value = true
    try {
      await api.post('/password-reset/confirm', { token, password: password.value })
      // The API logs the requester into a fresh session on success; a hard
      // navigation picks that cookie up rather than re-running the client
      // auth-context refresh through the login form.
      window.location.assign('/checklists')
    } catch (err) {
      error.value = err instanceof ApiError ? err.message : 'Reset failed — the link may be invalid or expired'
    } finally {
      submitting.value = false
    }
  })

  if (!token) {
    return (
      <div class="login-page">
        <div class="login-form">
          <h1>Reset password</h1>
          <p class="error">Missing or invalid reset link.</p>
          <p>
            <Link href="/password-reset/request">Request a new link</Link>
          </p>
        </div>
      </div>
    )
  }

  return (
    <div class="login-page">
      <form onSubmit$={handleSubmit} preventdefault:submit class="login-form">
        <h1>Choose a new password</h1>
        {error.value && <p class="error">{error.value}</p>}
        <label for="password">New password</label>
        <input
          id="password"
          type="password"
          minLength={8}
          value={password.value}
          onInput$={(_, el) => (password.value = el.value)}
          autoComplete="new-password"
          required
        />
        <button type="submit" disabled={submitting.value}>
          {submitting.value ? 'Saving…' : 'Save new password'}
        </button>
      </form>
    </div>
  )
})

export const head: DocumentHead = {
  title: 'Reset password - ChecklistHQ',
}
