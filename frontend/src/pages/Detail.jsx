import { useEffect, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { api, fmt, STATUS } from '../api'

// Zanjirning bitta bosqichi: modelga nima ketdi va nima qaytdi.
function Step({ s }) {
  const [open, setOpen] = useState(false)
  let pretty = s.raw_response
  try {
    pretty = JSON.stringify(JSON.parse(s.raw_response), null, 2)
  } catch { /* JSON emas — asl matn ko'rsatiladi */ }

  return (
    <div className="card" style={{ marginBottom: 10 }}>
      <div className="spread">
        <strong>{s.step_no}-bosqich · promt #{s.promt_id} {s.promt_title && `— ${s.promt_title}`}</strong>
        <span className="muted" style={{ fontSize: 13 }}>
          {fmt.num(s.prompt_tokens)} kirish · {fmt.num(s.completion_tokens)} chiqish · {(s.duration_ms / 1000).toFixed(1)}s
        </span>
      </div>
      <div className="row" style={{ marginTop: 8 }}>
        <button className="ghost" onClick={() => setOpen(!open)}>
          {open ? 'Yopish' : 'Modelga ketgan matnni ko\'rish'}
        </button>
      </div>
      {open && <pre className="raw" style={{ marginTop: 8 }}>{s.request_context}</pre>}
      <label>Model javobi</label>
      <pre className="raw">{pretty}</pre>
    </div>
  )
}

export default function Detail() {
  const { id } = useParams()
  const [item, setItem] = useState(null)
  const [err, setErr] = useState('')

  useEffect(() => {
    api.interaction(id).then(setItem).catch((e) => setErr(e.message))
  }, [id])

  if (err) return <div className="err">{err}</div>
  if (!item) return <p className="muted">Yuklanmoqda…</p>

  const st = STATUS[item.status] || { label: item.status, cls: '' }

  return (
    <>
      <p><Link to="/queue">← Navbatga qaytish</Link></p>
      <div className="spread">
        <h1>Suhbat #{item.conversation_id}</h1>
        <span className={`badge ${st.cls}`}>{st.label}</span>
      </div>
      <p className="hint">
        Mijoz {item.client_id} · {fmt.date(item.created_at)} · model: {item.model || '—'}
        {item.source === 'telegram' && ' · manba: xodimning Telegramdagi javobi'}
      </p>

      <div className="cards">
        <div className="card"><div className="k">Bosqich</div><div className="v">{item.steps_count}</div></div>
        <div className="card"><div className="k">Kirish token</div><div className="v">{fmt.num(item.prompt_tokens)}</div>
          <div className="s">kesh: {fmt.num(item.cached_tokens)}</div></div>
        <div className="card"><div className="k">Chiqish token</div><div className="v">{fmt.num(item.completion_tokens)}</div></div>
        <div className="card"><div className="k">Xarajat</div><div className="v">{fmt.usd(item.cost_usd)}</div>
          <div className="s">{item.calls} so'rov</div></div>
      </div>

      {item.error && <div className="err" style={{ marginTop: 14 }}>{item.error}</div>}

      <h2>Mijoz xabari</h2>
      <div className="card">{item.client_message || <span className="muted">—</span>}</div>

      <h2>Mijozga javob (chat)</h2>
      <div className="card">
        {item.chat_reply || <span className="muted">—</span>}
        <div className="muted" style={{ fontSize: 13, marginTop: 8 }}>
          {item.read_marked
            ? `Javob yetib bordi — xabarlar o'qilgan deb belgilandi (${item.message_ids || '—'})`
            : `O'qilgan deb belgilanmagan${item.message_ids ? ` — kutayotgan xabarlar: ${item.message_ids}` : ''}`}
        </div>
      </div>

      <h2>Xodimlarga (help)</h2>
      <div className="card">
        {item.help_text || <span className="muted">—</span>}
        {item.help_text && (
          <div className="muted" style={{ fontSize: 13, marginTop: 8 }}>
            {item.help_sent
              ? 'Telegram guruhga yuborilgan (tasdiq kutmaydi)'
              : 'Telegramga yuborilmadi — tasdiqlaganda qayta uriniladi'}
          </div>
        )}
      </div>

      <h2>Zanjir bosqichlari</h2>
      {(item.steps || []).map((s) => <Step key={s.id} s={s} />)}
    </>
  )
}
