import { useState } from 'react'
import type { FormEvent } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { ApiError, api } from '../api/client'

export default function PasswordResetConfirm() {
  const [searchParams] = useSearchParams()
  const token = searchParams.get('token') ?? ''
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    setSubmitting(true)
    try {
      await api.post('/password-reset/confirm', { token, password })
      // The API logs the requester into a fresh session on success; reload
      // AuthContext's state to pick that up rather than re-submitting
      // credentials through the login form.
      window.location.assign('/checklists')
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Reset failed — the link may be invalid or expired')
    } finally {
      setSubmitting(false)
    }
  }

  if (!token) {
    return (
      <div className="login-page">
        <div className="login-form">
          <h1>Reset password</h1>
          <p className="error">Missing or invalid reset link.</p>
          <p>
            <Link to="/password-reset/request">Request a new link</Link>
          </p>
        </div>
      </div>
    )
  }

  return (
    <div className="login-page">
      <form onSubmit={handleSubmit} className="login-form">
        <h1>Choose a new password</h1>
        {error && <p className="error">{error}</p>}
        <label htmlFor="password">New password</label>
        <input
          id="password"
          type="password"
          minLength={8}
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          autoComplete="new-password"
          required
        />
        <button type="submit" disabled={submitting}>
          {submitting ? 'Saving…' : 'Save new password'}
        </button>
      </form>
    </div>
  )
}
