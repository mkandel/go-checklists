// Ported from web-react/src/pages/PasswordResetRequest.tsx. Uses raw fetch
// with a form-urlencoded body — this endpoint decodes via r.FormValue, not
// JSON, unlike almost everything else in the API. See NOTES-QWIK.md #4.
import { $, component$, useSignal } from '@builder.io/qwik'
import type { DocumentHead } from '@builder.io/qwik-city'
import { Link } from '@builder.io/qwik-city'

export default component$(() => {
  const username = useSignal('')
  const submitted = useSignal(false)
  const error = useSignal<string | null>(null)
  const submitting = useSignal(false)

  const handleSubmit = $(async () => {
    error.value = null
    submitting.value = true
    try {
      const res = await fetch('/password-reset/request', {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
        body: new URLSearchParams({ username: username.value }).toString(),
      })
      if (!res.ok) throw new Error(res.status === 429 ? 'Too many requests, try again later' : 'Request failed')
      submitted.value = true
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Request failed'
    } finally {
      submitting.value = false
    }
  })

  return (
    <div class="login-page">
      <div class="login-form">
        <h1>Reset password</h1>
        {submitted.value ? (
          <p>If that account has an email on file, a reset link has been sent.</p>
        ) : (
          <form onSubmit$={handleSubmit} preventdefault:submit>
            {error.value && <p class="error">{error.value}</p>}
            <label for="username">Username</label>
            <input
              id="username"
              value={username.value}
              onInput$={(_, el) => (username.value = el.value)}
              autoComplete="username"
              required
            />
            <button type="submit" disabled={submitting.value}>
              {submitting.value ? 'Sending…' : 'Send reset link'}
            </button>
          </form>
        )}
        <p>
          <Link href="/login">Back to sign in</Link>
        </p>
      </div>
    </div>
  )
})

export const head: DocumentHead = {
  title: 'Reset password - ChecklistHQ',
}
