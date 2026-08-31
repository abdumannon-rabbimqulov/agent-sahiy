import { HashRouter, Routes, Route, Navigate, NavLink, useNavigate } from 'react-router-dom'
import { tokenStore } from './api'
import Login from './pages/Login.jsx'
import Dashboard from './pages/Dashboard.jsx'
import Queue from './pages/Queue.jsx'
import Detail from './pages/Detail.jsx'
import Issues from './pages/Issues.jsx'
import Promts from './pages/Promts.jsx'
import Settings from './pages/Settings.jsx'

// Himoyalangan yo'l: token bo'lmasa login sahifasiga.
function Protected({ children }) {
  return tokenStore.get() ? children : <Navigate to="/login" replace />
}

function Layout({ children }) {
  const nav = useNavigate()
  const logout = () => {
    tokenStore.clear()
    nav('/login')
  }
  return (
    <div className="app">
      <aside className="side">
        <div className="brand">Sahiy <span>AI agent</span></div>
        <nav>
          <NavLink to="/">Statistika</NavLink>
          <NavLink to="/queue">Tasdiqlash navbati</NavLink>
          <NavLink to="/issues">Muammoli buyurtmalar</NavLink>
          <NavLink to="/promts">Promtlar</NavLink>
          <NavLink to="/settings">Sozlamalar</NavLink>
        </nav>
        <button className="ghost" onClick={logout}>Chiqish</button>
      </aside>
      <main className="main">{children}</main>
    </div>
  )
}

export default function App() {
  return (
    <HashRouter>
      <Routes>
        <Route path="/login" element={<Login />} />
        <Route
          path="*"
          element={
            <Protected>
              <Layout>
                <Routes>
                  <Route path="/" element={<Dashboard />} />
                  <Route path="/queue" element={<Queue />} />
                  <Route path="/interactions/:id" element={<Detail />} />
                  <Route path="/issues" element={<Issues />} />
                  <Route path="/promts" element={<Promts />} />
                  <Route path="/settings" element={<Settings />} />
                </Routes>
              </Layout>
            </Protected>
          }
        />
      </Routes>
    </HashRouter>
  )
}
