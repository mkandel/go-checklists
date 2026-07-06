// Thin fetch wrapper for the JSON API under /api/* plus the unprefixed auth
// routes (/login, /register, /logout). All calls are same-origin: in dev,
// vite.config.ts proxies these paths to the API server (:8080); in
// production the built SPA is served by internal/webqwik on the same
// origin as the API. See docs/architecture.md for the CSRF/session model
// this mirrors from internal/web. Ported verbatim from web-react/src/api/client.ts
// — plain fetch/TS, no framework dependency.

const CSRF_COOKIE = 'checklists_csrf'

function readCookie(name: string): string | null {
  const match = document.cookie
    .split('; ')
    .find((row) => row.startsWith(`${name}=`))
  return match ? decodeURIComponent(match.slice(name.length + 1)) : null
}

export class ApiError extends Error {
  status: number
  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

async function request<T>(
  method: string,
  path: string,
  body?: unknown,
): Promise<T> {
  const headers: Record<string, string> = {}
  const isMutating = method !== 'GET'
  if (isMutating) {
    const csrf = readCookie(CSRF_COOKIE)
    if (csrf) headers['X-CSRF-Token'] = csrf
  }
  if (body !== undefined) headers['Content-Type'] = 'application/json'

  const res = await fetch(path, {
    method,
    headers,
    credentials: 'include',
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })

  if (!res.ok) {
    const text = await res.text()
    throw new ApiError(res.status, text || res.statusText)
  }
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

export const api = {
  get: <T>(path: string) => request<T>('GET', path),
  post: <T>(path: string, body?: unknown) => request<T>('POST', path, body),
  put: <T>(path: string, body?: unknown) => request<T>('PUT', path, body),
  del: <T>(path: string) => request<T>('DELETE', path),
}

export async function login(username: string, password: string): Promise<void> {
  const form = new URLSearchParams({ username, password })
  const res = await fetch('/login', {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
    body: form.toString(),
  })
  if (!res.ok) {
    throw new ApiError(res.status, res.status === 401 ? 'Invalid username or password' : res.statusText)
  }
}

export async function logout(): Promise<void> {
  await api.post('/logout')
}
