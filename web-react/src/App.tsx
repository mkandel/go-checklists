import { Navigate, Route, Routes } from 'react-router-dom'
import { AuthProvider } from './auth/AuthContext'
import RequireAuth from './components/RequireAuth'
import Layout from './components/Layout'
import Login from './pages/Login'
import ChecklistList from './pages/ChecklistList'
import ChecklistDetail from './pages/ChecklistDetail'

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
          <Route path="/checklists/:id" element={<ChecklistDetail />} />
        </Route>
      </Routes>
    </AuthProvider>
  )
}

export default App
