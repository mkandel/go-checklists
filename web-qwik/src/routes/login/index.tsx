// Ported from web-react/src/pages/Login.tsx.
import { $, component$, useContext, useSignal } from '@builder.io/qwik'
import type { DocumentHead } from '@builder.io/qwik-city'
import { Link, useNavigate } from '@builder.io/qwik-city'
import { AuthContext, login as doLogin } from '../../lib/auth/auth-context'

export default component$(() => {
  const auth = useContext(AuthContext)
  const nav = useNavigate()
  const username = useSignal('')
  const password = useSignal('')
  const error = useSignal<string | null>(null)
  const submitting = useSignal(false)

  const handleSubmit = $(async () => {
    error.value = null
    submitting.value = true
    try {
      await doLogin(auth, username.value, password.value)
      await nav('/checklists')
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Login failed'
    } finally {
      submitting.value = false
    }
  })

  return (
    <div class="login-page">
      <form onSubmit$={handleSubmit} preventdefault:submit class="login-form">
        <h1>Sign in</h1>
        {error.value && <p class="error">{error.value}</p>}
        <label for="username">Username</label>
        <input
          id="username"
          value={username.value}
          onInput$={(_, el) => (username.value = el.value)}
          autoComplete="username"
          required
        />
        <label for="password">Password</label>
        <input
          id="password"
          type="password"
          value={password.value}
          onInput$={(_, el) => (password.value = el.value)}
          autoComplete="current-password"
          required
        />
        <button type="submit" disabled={submitting.value}>
          {submitting.value ? 'Signing in…' : 'Sign in'}
        </button>
        <p>
          <Link href="/register">Create an account</Link> · <Link href="/password-reset/request">Forgot password?</Link>
        </p>
      </form>
    </div>
  )
})

export const head: DocumentHead = {
  title: 'Sign in - ChecklistHQ',
}
