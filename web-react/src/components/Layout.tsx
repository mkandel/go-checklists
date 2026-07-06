import { NavLink, Outlet, useNavigate } from 'react-router-dom'
import { useAuth } from '../auth/AuthContext'
import NotificationBadge from './NotificationBadge'

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
          <NavLink to="/groups">Groups</NavLink>
          <NavLink to="/templates">Templates</NavLink>
          <NotificationBadge />
          {me?.IsAdmin && <NavLink to="/admin/users">Admin</NavLink>}
          {me?.IsAdmin && <NavLink to="/admin/settings">Settings</NavLink>}
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
