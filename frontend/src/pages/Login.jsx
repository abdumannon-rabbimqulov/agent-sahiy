import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, tokenStore } from '../api'

export default function Login() {
  const [login, setLogin] = useState('')
  const [password, setPassword] = useState('')
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  const nav = useNavigate()

  const submit = async (e) => {
    e.preventDefault()
    setErr('')
    setBusy(true)
    try {
      const { token } = await api.login(login, password)
      tokenStore.set(token)
      nav('/')
    } catch (e) {
      setErr(e.message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="login">
      <form onSubmit={submit}>
        <h1>Sahiy AI agent</h1>
        <p className="hint">Admin panelga kirish</p>
        {err && <div className="err">{err}</div>}
        <label>Login</label>
        <input value={login} onChange={(e) => setLogin(e.target.value)} autoFocus />
        <label>Parol</label>
        <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} />
        <div style={{ marginTop: 18 }}>
          <button disabled={busy || !login || !password}>{busy ? 'Kirilmoqda…' : 'Kirish'}</button>
        </div>
      </form>
    </div>
  )
}
