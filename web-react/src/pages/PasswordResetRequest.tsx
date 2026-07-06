import { useState } from 'react'
import type { FormEvent } from 'react'
import { Link } from 'react-router-dom'

export default function PasswordResetRequest() {
  const [username, setUsername] = useState('')
  const [submitted, setSubmitted] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    setSubmitting(true)
    try {
      const res = await fetch('/password-reset/request', {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
        body: new URLSearchParams({ username }).toString(),
      })
      if (!res.ok) throw new Error(res.status === 429 ? 'Too many requests, try again later' : 'Request failed')
      setSubmitted(true)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Request failed')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="login-page">
      <div className="login-form">
        <h1>Reset password</h1>
        {submitted ? (
          <p>If that account has an email on file, a reset link has been sent.</p>
        ) : (
          <form onSubmit={handleSubmit}>
            {error && <p className="error">{error}</p>}
            <label htmlFor="username">Username</label>
            <input id="username" value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="username" required />
            <button type="submit" disabled={submitting}>
              {submitting ? 'Sending…' : 'Send reset link'}
            </button>
          </form>
        )}
        <p>
          <Link to="/login">Back to sign in</Link>
        </p>
      </div>
    </div>
  )
}
