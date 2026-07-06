import { NavLink, Outlet, useNavigate } from 'react-router-dom'
import { useAuth } from '../auth/AuthContext'

export default function Layout() {
  const { me, logout } = useAuth()
  const navigate = useNavigate()

  async function handleLogout() {
    await logout()
    navigate('/login', { replace: true })
  }

  return (
    <div className="app-shell">
      <header className="app-nav">
        <span className="app-name">ChecklistHQ</span>
        <nav>
          <NavLink to="/checklists">Checklists</NavLink>
          {me?.IsAdmin && <NavLink to="/admin">Admin</NavLink>}
        </nav>
        <div className="app-nav-user">
          <span>{me?.Name}</span>
          <button type="button" onClick={handleLogout}>
            Log out
          </button>
        </div>
      </header>
      <main>
        <Outlet />
      </main>
    </div>
  )
}
