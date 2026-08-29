import { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, fmt, STATUS } from '../api'

// Bitta murojaat kartasi: matnni tahrirlab, tasdiqlash yoki rad etish.
function Item({ item, onDone }) {
  const [chat, setChat] = useState(item.chat_reply || '')
  const [help, setHelp] = useState(item.help_text || '')
  const [busy, setBusy] = useState('')
  const [err, setErr] = useState('')
  const edited = chat !== (item.chat_reply || '') || help !== (item.help_text || '')

  const act = async (what) => {
    setErr('')
    setBusy(what)
    try {
      if (what === 'approve') {
        if (edited) await api.patchInteraction(item.id, { chat_reply: chat, help_text: help })
        await api.approve(item.id)
      } else if (what === 'reject') {
        await api.reject(item.id)
      } else {
        await api.patchInteraction(item.id, { chat_reply: chat, help_text: help })
      }
      onDone()
    } catch (e) {
      setErr(e.message)
    } finally {
      setBusy('')
    }
  }

  const st = STATUS[item.status] || { label: item.status, cls: '' }

  return (
    <div className="card" style={{ marginBottom: 14 }}>
      <div className="spread">
        <div>
          <strong>Suhbat #{item.conversation_id}</strong>{' '}
          <span className="muted">· mijoz {item.client_id} · {fmt.date(item.created_at)}</span>
        </div>
        <div className="row">
          <span className={`badge ${st.cls}`}>{st.label}</span>
          <Link to={`/interactions/${item.id}`}>Tafsilot</Link>
        </div>
      </div>

      {item.client_message && (
        <>
          <label>Mijoz xabari</label>
          <div className="muted">{item.client_message}</div>
        </>
      )}

      <label>Mijozga javob (chat)</label>
      <textarea value={chat} onChange={(e) => setChat(e.target.value)} />

      <label>
        Xodimlarga (help → Telegram)
        {item.help_sent
          ? <span className="muted"> — allaqachon yuborilgan, tasdiq kutmaydi</span>
          : item.help_text
            ? <span className="muted"> — yuborilmadi, tasdiqlaganda qayta uriniladi</span>
            : null}
      </label>
      <textarea value={help} onChange={(e) => setHelp(e.target.value)} disabled={item.help_sent} />

      {item.error && <div className="err">{item.error}</div>}
      {err && <div className="err">{err}</div>}

      <div className="row" style={{ marginTop: 12 }}>
        <button onClick={() => act('approve')} disabled={!!busy || (!chat && !help)}>
          {busy === 'approve' ? 'Yuborilmoqda…' : 'Tasdiqlash va mijozga yuborish'}
        </button>
        <button className="ghost" onClick={() => act('save')} disabled={!!busy || !edited}>Saqlash</button>
        <button className="danger" onClick={() => act('reject')} disabled={!!busy}>Rad etish</button>
        <span className="muted" style={{ marginLeft: 'auto', fontSize: 13 }}>
          {item.help_sent && <span title="help xodimlar guruhiga yuborilgan">✈︎ help ketdi · </span>}
          {item.read_marked && <span title="Javob yetib bordi — xabarlar o'qilgan deb belgilangan">✓ o'qilgan · </span>}
          {item.steps_count} bosqich · {fmt.num(item.prompt_tokens + item.completion_tokens)} token · {fmt.usd(item.cost_usd)}
        </span>
      </div>
    </div>
  )
}

export default function Queue() {
  const [status, setStatus] = useState('pending')
  const [data, setData] = useState({ items: [], total: 0 })
  const [err, setErr] = useState('')
  const [loading, setLoading] = useState(true)

  const load = useCallback(() => {
    setLoading(true)
    api.interactions(status, 1, 50)
      .then(setData)
      .catch((e) => setErr(e.message))
      .finally(() => setLoading(false))
  }, [status])

  useEffect(load, [load])

  return (
    <>
      <div className="spread">
        <div>
          <h1>Tasdiqlash navbati</h1>
          <p className="hint">
          Tasdiqlash faqat <strong>mijozga</strong> ketadigan javobga tegishli.
          <code>help</code> xodimlar guruhiga tasdiqsiz, darhol yuboriladi.
        </p>
        </div>
        <div className="row">
          <select value={status} onChange={(e) => setStatus(e.target.value)} style={{ width: 190 }}>
            <option value="pending">Kutayotganlar</option>
            <option value="">Hammasi</option>
            <option value="sent">Avto yuborilgan</option>
            <option value="approved">Tasdiqlangan</option>
            <option value="rejected">Rad etilgan</option>
            <option value="failed">Xato</option>
          </select>
          <button className="ghost" onClick={load}>Yangilash</button>
        </div>
      </div>

      {err && <div className="err">{err}</div>}
      {loading && <p className="muted">Yuklanmoqda…</p>}
      {!loading && data.items.length === 0 && <p className="muted">Bo'sh — hozircha hech narsa yo'q.</p>}
      {data.items.map((it) => <Item key={it.id} item={it} onDone={load} />)}
    </>
  )
}
