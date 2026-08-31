import { useCallback, useEffect, useState } from 'react'
import { api, fmt } from '../api'

// Bitta muammo qatori: yechim yozib yopish yoki yozilgan yechimni ko'rish.
function Row({ item, onDone }) {
  const [text, setText] = useState('')
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')
  const open = item.state === 'open'

  const resolve = async () => {
    setErr(''); setBusy(true)
    try {
      await api.resolveIssue(item.id, text)
      onDone()
    } catch (e) {
      setErr(e.message)
    } finally {
      setBusy(false)
    }
  }

  const via = { telegram: 'Telegram', auto: 'avtomatik', panel: 'panel' }[item.resolved_via] || item.resolved_via

  return (
    <div className="card" style={{ marginBottom: 12 }}>
      <div className="spread">
        <div>
          <strong>{item.order_sn}</strong>{' '}
          <span className="muted">
            · {item.status_label} · to'langaniga {item.days_since_paid} kun
          </span>
        </div>
        <span className={`badge ${open ? 'st-pending' : 'st-sent'}`}>
          {open ? 'Ochiq' : 'Hal qilindi'}
        </span>
      </div>

      <div className="muted" style={{ fontSize: 13, marginTop: 4 }}>
        Mijoz {item.client_id} · suhbat #{item.conversation_id} ·
        aniqlangan {fmt.date(item.created_at)}
        {open && item.notify_count > 0 && ` · guruhga ${item.notify_count} marta yozilgan`}
      </div>
      {item.package_name && (
        <div className="muted" style={{ fontSize: 13 }}>{item.package_name}</div>
      )}

      {!open && (
        <>
          <label>Qanday hal qilindi ({via}{item.resolved_by && ` — ${item.resolved_by}`})</label>
          <div>{item.resolution || <span className="muted">—</span>}</div>
          <div className="muted" style={{ fontSize: 13, marginTop: 4 }}>
            {fmt.date(item.resolved_at)}
          </div>
        </>
      )}

      {open && (
        <>
          <label>Yechimni qo'lda yozish (odatda Telegram guruhda reply qilinadi)</label>
          <div className="row">
            <input value={text} onChange={(e) => setText(e.target.value)}
                   placeholder="Nima qilindi?" style={{ flex: 1 }} />
            <button onClick={resolve} disabled={busy || !text}>Hal qilindi</button>
          </div>
        </>
      )}
      {err && <div className="err">{err}</div>}
    </div>
  )
}

export default function Issues() {
  const [state, setState] = useState('open')
  const [data, setData] = useState({ items: [], total: 0 })
  const [err, setErr] = useState('')
  const [msg, setMsg] = useState('')
  const [loading, setLoading] = useState(true)

  const load = useCallback(() => {
    setLoading(true)
    api.issues(state, 1, 50)
      .then(setData)
      .catch((e) => setErr(e.message))
      .finally(() => setLoading(false))
  }, [state])

  useEffect(load, [load])

  const review = async () => {
    setErr(''); setMsg('')
    try {
      await api.reviewIssues()
      setMsg('Qayta tekshirildi')
      load()
    } catch (e) {
      setErr(e.message)
    }
  }

  return (
    <>
      <div className="spread">
        <div>
          <h1>Muammoli buyurtmalar</h1>
          <p className="hint">
            To'lovdan keyin uzoq qotib qolgan buyurtmalar. Guruhdagi xabarga
            <strong> reply</strong> qilinsa, javob yechim sifatida saqlanadi.
          </p>
        </div>
        <div className="row">
          <select value={state} onChange={(e) => setState(e.target.value)} style={{ width: 170 }}>
            <option value="open">Ochiqlari</option>
            <option value="">Hammasi</option>
            <option value="resolved">Hal qilinganlar</option>
          </select>
          <button className="ghost" onClick={review}>Qayta tekshirish</button>
        </div>
      </div>

      {err && <div className="err">{err}</div>}
      {msg && <div className="ok">{msg}</div>}
      {loading && <p className="muted">Yuklanmoqda…</p>}
      {!loading && data.items.length === 0 && <p className="muted">Muammo yo'q.</p>}
      {data.items.map((it) => <Row key={it.id} item={it} onDone={load} />)}
    </>
  )
}
