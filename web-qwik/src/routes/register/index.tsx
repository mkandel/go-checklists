// Ported from web-react/src/pages/Register.tsx.
import { $, component$, useContext, useSignal } from '@builder.io/qwik'
import type { DocumentHead } from '@builder.io/qwik-city'
import { Link, useNavigate } from '@builder.io/qwik-city'
import { AuthContext, login as doLogin } from '../../lib/auth/auth-context'
import { ApiError, api } from '../../lib/api/client'

export default component$(() => {
  const auth = useContext(AuthContext)
  const nav = useNavigate()
  const username = useSignal('')
  const name = useSignal('')
  const email = useSignal('')
  const password = useSignal('')
  const error = useSignal<string | null>(null)
  const submitting = useSignal(false)

  const handleSubmit = $(async () => {
    error.value = null
    submitting.value = true
    try {
      await api.post('/register', {
        username: username.value,
        name: name.value,
        email: email.value || undefined,
        password: password.value,
      })
      await doLogin(auth, username.value, password.value)
      await nav('/checklists')
    } catch (err) {
      error.value = err instanceof ApiError ? err.message : 'Registration failed'
    } finally {
      submitting.value = false
    }
  })

  return (
    <div class="login-page">
      <form onSubmit$={handleSubmit} preventdefault:submit class="login-form">
        <h1>Create account</h1>
        {error.value && <p class="error">{error.value}</p>}
        <label for="username">Username</label>
        <input
          id="username"
          value={username.value}
          onInput$={(_, el) => (username.value = el.value)}
          autoComplete="username"
          required
        />
        <label for="name">Name</label>
        <input id="name" value={name.value} onInput$={(_, el) => (name.value = el.value)} autoComplete="name" required />
        <label for="email">Email (optional)</label>
        <input
          id="email"
          type="email"
          value={email.value}
          onInput$={(_, el) => (email.value = el.value)}
          autoComplete="email"
        />
        <label for="password">Password</label>
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
          {submitting.value ? 'Creating account…' : 'Create account'}
        </button>
        <p>
          <Link href="/login">Already have an account? Sign in</Link>
        </p>
      </form>
    </div>
  )
})

export const head: DocumentHead = {
  title: 'Create account - ChecklistHQ',
}
