import { useCallback, useEffect, useState } from 'react'
import { api, fmt } from '../api'

// Bo'sh forma — yangi promt uchun.
const EMPTY = { id: 0, title: '', promt: '' }

export default function Promts() {
  const [list, setList] = useState([])
  const [form, setForm] = useState(EMPTY)
  const [err, setErr] = useState('')
  const [msg, setMsg] = useState('')
  const [busy, setBusy] = useState(false)

  const load = useCallback(() => {
    api.promts().then((r) => setList(r || [])).catch((e) => setErr(e.message))
  }, [])

  useEffect(load, [load])

  const save = async (e) => {
    e.preventDefault()
    setErr(''); setMsg(''); setBusy(true)
    try {
      if (form.id) {
        await api.updatePromt(form.id, { title: form.title, promt: form.promt })
        setMsg(`Promt #${form.id} saqlandi`)
      } else {
        const created = await api.createPromt({ title: form.title, promt: form.promt })
        setMsg(`Promt #${created.id} yaratildi — zanjirda shu id bilan chaqiriladi`)
      }
      setForm(EMPTY)
      load()
    } catch (e) {
      setErr(e.message)
    } finally {
      setBusy(false)
    }
  }

  const remove = async (id) => {
    setErr(''); setMsg('')
    try {
      await api.deletePromt(id)
      if (form.id === id) setForm(EMPTY)
      setMsg(`Promt #${id} o'chirildi`)
      load()
    } catch (e) {
      setErr(e.message)
    }
  }

  return (
    <>
      <h1>Promtlar</h1>
      <p className="hint">
        Zanjir <strong>#1</strong> dan boshlanadi. Model javobidagi <code>promt</code> kaliti
        keyingi promt id'sini ko'rsatadi; <code>null</code> bo'lsa zanjir tugaydi.
      </p>

      {err && <div className="err">{err}</div>}
      {msg && <div className="ok">{msg}</div>}

      <form className="card" onSubmit={save}>
        <div className="spread">
          <strong>{form.id ? `Promt #${form.id} tahriri` : 'Yangi promt'}</strong>
          {form.id > 0 && <button type="button" className="ghost" onClick={() => setForm(EMPTY)}>Bekor qilish</button>}
        </div>
        <label>Sarlavha</label>
        <input value={form.title} onChange={(e) => setForm({ ...form, title: e.target.value })} />
        <label>Promt matni (modelga JSON qaytarishni buyuring)</label>
        <textarea style={{ minHeight: 220 }} value={form.promt}
                  onChange={(e) => setForm({ ...form, promt: e.target.value })} />
        <div style={{ marginTop: 12 }}>
          <button disabled={busy || !form.title || !form.promt}>
            {form.id ? 'Saqlash' : 'Yaratish'}
          </button>
        </div>
      </form>

      <h2>Ro'yxat</h2>
      <table>
        <thead>
          <tr><th>ID</th><th>Sarlavha</th><th>Matn (boshi)</th><th>Yangilangan</th><th></th></tr>
        </thead>
        <tbody>
          {list.map((p) => (
            <tr key={p.id}>
              <td><strong>{p.id}</strong></td>
              <td>{p.title}</td>
              <td className="muted">{p.promt.slice(0, 70)}…</td>
              <td className="muted">{fmt.date(p.updated_at)}</td>
              <td>
                <div className="row">
                  <button className="ghost" onClick={() => setForm(p)}>Tahrirlash</button>
                  <button className="danger" onClick={() => remove(p.id)}>O'chirish</button>
                </div>
              </td>
            </tr>
          ))}
          {list.length === 0 && <tr><td colSpan="5" className="muted">Promt yo'q</td></tr>}
        </tbody>
      </table>
    </>
  )
}
