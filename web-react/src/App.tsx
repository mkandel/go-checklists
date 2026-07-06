import { Navigate, Route, Routes } from 'react-router-dom'
import { AuthProvider } from './auth/AuthContext'
import RequireAuth from './components/RequireAuth'
import RequireAdmin from './components/RequireAdmin'
import Layout from './components/Layout'
import Login from './pages/Login'
import ChecklistList from './pages/ChecklistList'
import ChecklistDetail from './pages/ChecklistDetail'
import ChecklistCreate from './pages/ChecklistCreate'
import GroupsList from './pages/GroupsList'
import TemplateList from './pages/TemplateList'
import TemplateDetail from './pages/TemplateDetail'
import TemplateCreate from './pages/TemplateCreate'
import NotificationsList from './pages/NotificationsList'
import AdminUsersList from './pages/AdminUsersList'
import AdminSettings from './pages/AdminSettings'

function App() {
  return (
    <AuthProvider>
      <Routes>
        <Route path="/login" element={<Login />} />
        <Route
          element={
            <RequireAuth>
              <Layout />
            </RequireAuth>
          }
        >
          <Route path="/" element={<Navigate to="/checklists" replace />} />
          <Route path="/checklists" element={<ChecklistList />} />
          <Route path="/checklists/new" element={<ChecklistCreate />} />
          <Route path="/checklists/:id" element={<ChecklistDetail />} />
          <Route path="/groups" element={<GroupsList />} />
          <Route path="/templates" element={<TemplateList />} />
          <Route path="/templates/new" element={<TemplateCreate />} />
          <Route path="/templates/:id" element={<TemplateDetail />} />
          <Route path="/notifications" element={<NotificationsList />} />
          <Route
            path="/admin/users"
            element={
              <RequireAdmin>
                <AdminUsersList />
              </RequireAdmin>
            }
          />
          <Route
            path="/admin/settings"
            element={
              <RequireAdmin>
                <AdminSettings />
              </RequireAdmin>
            }
          />
        </Route>
      </Routes>
    </AuthProvider>
  )
}

export default App
