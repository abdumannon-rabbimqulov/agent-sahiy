import { useCallback, useEffect, useState } from 'react'
import { api, fmt } from '../api'

// Bo'sh forma — yangi promt uchun.
const EMPTY = { id: 0, title: '', promt: '' }

// NEW - "yangi promt" formasining ochiq holati. Tahrir holatida bu
// yerda promt id'si turadi, yopiq holatda — null. Sahifada bir vaqtda
// FAQAT bitta forma ochiq bo'ladi.
const NEW = 'new'

// PromtForm - ham yangi promt, ham tahrir uchun bitta forma. Tahrir
// qilinayotgan promt qatorining ICHIDA chiziladi: ilgari forma
// sahifaning tepasida turardi va "Tahrirlash" bosilganda ma'lumot
// ko'rinmaydigan joyda — ekranning yuqorisida — ochilardi.
function PromtForm({ value, onChange, onSubmit, onCancel, busy, isNew }) {
  return (
    <form onSubmit={onSubmit}>
      <label>Sarlavha</label>
      <input autoFocus value={value.title}
             onChange={(e) => onChange({ ...value, title: e.target.value })} />
      <label>Promt matni (modelga JSON qaytarishni buyuring)</label>
      <textarea style={{ minHeight: 220 }} value={value.promt}
                onChange={(e) => onChange({ ...value, promt: e.target.value })} />
      <div className="row" style={{ marginTop: 12 }}>
        <button disabled={busy || !value.title || !value.promt}>
          {isNew ? 'Yaratish' : 'Saqlash'}
        </button>
        <button type="button" className="ghost" onClick={onCancel}>Bekor qilish</button>
      </div>
    </form>
  )
}

export default function Promts() {
  const [list, setList] = useState([])
  const [form, setForm] = useState(EMPTY)
  // open - hozir qaysi forma ochiq: NEW, promt id'si yoki null.
  const [open, setOpen] = useState(null)
  const [err, setErr] = useState('')
  const [msg, setMsg] = useState('')
  const [busy, setBusy] = useState(false)

  const load = useCallback(() => {
    api.promts().then((r) => setList(r || [])).catch((e) => setErr(e.message))
  }, [])

  useEffect(load, [load])

  const close = () => { setOpen(null); setForm(EMPTY) }

  // openNew / openEdit - formani ochadi. Ikkinchisini ochish birinchisini
  // yopadi: ikkita ochiq forma bo'lsa qaysi biri saqlanayotgani
  // chalkashadi.
  const openNew = () => { setErr(''); setMsg(''); setForm(EMPTY); setOpen(NEW) }
  const openEdit = (p) => { setErr(''); setMsg(''); setForm(p); setOpen(p.id) }

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
      close()
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
      if (open === id) close()
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

      <div className="spread" style={{ margin: '18px 0 10px' }}>
        <h2 style={{ margin: 0 }}>Ro'yxat</h2>
        {/* Yangi promt formasi doim ochiq turmaydi — shu ikonchadan ochiladi. */}
        <button className="icon-btn" onClick={openNew} title="Yangi promt"
                aria-label="Yangi promt">+</button>
      </div>

      {open === NEW && (
        <div className="card" style={{ marginBottom: 12 }}>
          <strong>Yangi promt</strong>
          <PromtForm isNew value={form} onChange={setForm} onSubmit={save}
                     onCancel={close} busy={busy} />
        </div>
      )}

      <table>
        <thead>
          <tr><th>ID</th><th>Sarlavha</th><th>Matn (boshi)</th><th>Yangilangan</th><th></th></tr>
        </thead>
        <tbody>
          {list.map((p) => (
            // Tahrir formasi shu promtning o'z qatori ostida ochiladi,
            // shuning uchun qator va forma bitta guruhda turadi.
            <Rows key={p.id} p={p} open={open} form={form} setForm={setForm}
                  save={save} close={close} busy={busy}
                  openEdit={openEdit} remove={remove} />
          ))}
          {list.length === 0 && <tr><td colSpan="5" className="muted">Promt yo'q</td></tr>}
        </tbody>
      </table>
    </>
  )
}

// Rows - bitta promtning jadval qatori va (ochiq bo'lsa) uning ostidagi
// tahrir qatori. Ikkalasi <tbody> ning bevosita bolasi bo'lishi kerak —
// shuning uchun fragment ichida qaytariladi.
function Rows({ p, open, form, setForm, save, close, busy, openEdit, remove }) {
  const editing = open === p.id
  return (
    <>
      <tr>
        <td><strong>{p.id}</strong></td>
        <td>{p.title}</td>
        <td className="muted">{p.promt.slice(0, 70)}…</td>
        <td className="muted">{fmt.date(p.updated_at)}</td>
        <td>
          <div className="row">
            <button className="ghost" onClick={() => (editing ? close() : openEdit(p))}>
              {editing ? 'Yopish' : 'Tahrirlash'}
            </button>
            <button className="danger" onClick={() => remove(p.id)}>O'chirish</button>
          </div>
        </td>
      </tr>
      {editing && (
        <tr className="edit-row">
          <td colSpan="5">
            <PromtForm value={form} onChange={setForm} onSubmit={save}
                       onCancel={close} busy={busy} />
          </td>
        </tr>
      )}
    </>
  )
}
