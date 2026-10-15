// Test chat: AI agentni mijoz bo'lib sinab ko'rish.
//
// Nega kerak: promt zanjirini tekshirishning yagona yo'li haqiqiy
// suhbatni qayta yurgizish edi — o'z matningni yozib ko'rib bo'lmasdi.
// Bu yerda yozgan xabarlaring to'g'ridan-to'g'ri zanjirga ketadi.
//
// Suhbat tarixi faqat shu komponentda (brauzerda) turadi va har
// so'rovda to'liq yuboriladi: backend hech narsa saqlamaydi, javob
// mijozga ketmaydi, Telegram guruhga ham chiqmaydi. Buyurtma va
// yetkazma ma'lumoti esa ko'rsatilgan client_id bo'yicha HAQIQIY
// tizimdan o'qiladi — shuning uchun javob haqiqiy suhbatdagiday chiqadi.
import { useEffect, useRef, useState } from 'react'
import { api, fmt } from '../api'

// Modelga ketadigan tarix 10 ta xabar bilan cheklangan (backend:
// HistoryLimit). Paneldagi ogohlantirish shunga tayanadi.
const HISTORY_LIMIT = 10

// Bosqichning modelga ketgan matni / javobi — Detail.jsx dagi kabi.
function Step({ s }) {
  const [open, setOpen] = useState(false)
  let pretty = s.raw_response
  try {
    pretty = JSON.stringify(JSON.parse(s.raw_response), null, 2)
  } catch { /* JSON emas — asl matn ko'rsatiladi */ }

  const label = s.promt_id
    ? `promt #${s.promt_id}${s.promt_title ? ` — ${s.promt_title}` : ''}`
    : (s.promt_title || 'model chaqirilmadi')

  return (
    <div className="card" style={{ marginBottom: 8 }}>
      <div className="spread">
        <strong>{s.step_no}-bosqich · {label}</strong>
        <span className="muted" style={{ fontSize: 13 }}>
          {fmt.num(s.prompt_tokens)} kirish · {fmt.num(s.completion_tokens)} chiqish
        </span>
      </div>
      <div className="row" style={{ marginTop: 8 }}>
        <button type="button" className="ghost" onClick={() => setOpen(!open)}>
          {open ? 'Yopish' : 'Modelga ketgan matn'}
        </button>
      </div>
      {open && <pre className="raw" style={{ marginTop: 8 }}>{s.request_context}</pre>}
      <label>Model javobi</label>
      <pre className="raw">{pretty}</pre>
    </div>
  )
}

export default function TestChat() {
  const [open, setOpen] = useState(false)
  const [clientID, setClientID] = useState('')
  const [convID, setConvID] = useState('1')
  // msgs - {type:'client'|'agent', message, empty?} — `empty` agent
  // mijozga hech narsa yozmagan holat (faqat xodimga topshirgan).
  const [msgs, setMsgs] = useState([])
  const [draft, setDraft] = useState('')
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')
  const [last, setLast] = useState(null)

  const boxRef = useRef(null)
  // Yangi xabar kelganda oynani pastga surib qo'yamiz.
  useEffect(() => {
    const el = boxRef.current
    if (el) el.scrollTop = el.scrollHeight
  }, [msgs, busy])

  // ask - `list` ni backendga yuboradi va javobni tarixga qo'shadi.
  // Qayta yuborishda ham shu ishlatiladi, shuning uchun ro'yxat
  // tashqaridan beriladi (state hali yangilanmagan bo'lishi mumkin).
  const ask = (list) => {
    setBusy(true)
    setErr('')
    api
      .sandbox(Number(convID) || 0, Number(clientID) || 0, list)
      .then((res) => {
        setLast(res)
        setMsgs([
          ...list,
          { type: 'agent', message: res.chat_reply || '', help: res.help_text || '', empty: !res.chat_reply },
        ])
      })
      .catch((e) => {
        setErr(e.message)
        setMsgs(list) // yozilgan xabar yo'qolmasin — qayta urinish mumkin
      })
      .finally(() => setBusy(false))
  }

  const send = () => {
    const text = draft.trim()
    if (!text || busy) return
    setDraft('')
    ask([...msgs, { type: 'client', message: text }])
  }

  // Qayta yuborish: oxirgi so'z mijozda qolgan bo'lsa (xato bo'lgan).
  const retry = () => ask(msgs)

  // Enter — yuborish, Shift+Enter — yangi qator.
  const onKey = (e) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      send()
    }
  }

  const lastIsClient = msgs.length > 0 && msgs[msgs.length - 1].type === 'client'
  // Backend faqat oxirgi 10 ta xabarni modelga beradi.
  const trimmed = msgs.length > HISTORY_LIMIT

  return (
    <div className="tchat-wrap">
      <div className="spread">
        <h2 style={{ margin: 0 }}>Test chat</h2>
        <div className="row">
          {msgs.length > 0 && (
            <button type="button" className="ghost" onClick={() => { setMsgs([]); setLast(null); setErr('') }}>
              Tozalash
            </button>
          )}
          <button type="button" className="ghost" onClick={() => setOpen(!open)}>
            {open ? 'Yopish' : 'Ochish'}
          </button>
        </div>
      </div>

      {open && (
        <>
          <p className="hint">
            Mijoz bo‘lib yozing — agent xuddi haqiqiy suhbatdagidek javob beradi.
            Javob mijozga yuborilmaydi, bazaga ham, Telegram guruhga ham tushmaydi.
            Buyurtma ma’lumoti esa ko‘rsatilgan mijoz id’si bo‘yicha haqiqiy tizimdan olinadi.
          </p>

          <div className="row">
            <div>
              <label>Mijoz id (client_id)</label>
              <input
                className="tchat-id"
                value={clientID}
                onChange={(e) => setClientID(e.target.value)}
                placeholder="masalan 12345"
              />
            </div>
            <div>
              <label>Suhbat id (conversation_id)</label>
              <input
                className="tchat-id"
                value={convID}
                onChange={(e) => setConvID(e.target.value)}
                placeholder="1"
              />
            </div>
          </div>
          <p className="muted" style={{ fontSize: 13 }}>
            Mijoz id’sini o‘zgartirib, xuddi shu savolni boshqa mijoz ma’lumoti bilan
            qayta berib ko‘rish mumkin — suhbat tarixi saqlanib qoladi.
          </p>

          <div className="tchat" ref={boxRef}>
            {msgs.length === 0 && !busy && (
              <p className="muted">Hali xabar yo‘q. Pastga yozing va «Yuborish» bosing.</p>
            )}
            {msgs.map((m, i) => (
              <div key={i} className={`tmsg ${m.type}`}>
                {m.empty ? (
                  <span className="muted">— agent mijozga javob yozmadi, xodimga topshirdi</span>
                ) : (
                  m.message
                )}
                {m.help && (
                  <div className="tmsg-help">
                    <b>Xodimga (Telegram guruhga ketardi):</b>
                    <div>{m.help}</div>
                  </div>
                )}
              </div>
            ))}
            {busy && <div className="tmsg agent muted">Agent o‘ylamoqda…</div>}
          </div>

          {err && (
            <div className="err">
              {err}
              {lastIsClient && (
                <div className="row" style={{ marginTop: 8 }}>
                  <button type="button" className="ghost" disabled={busy} onClick={retry}>
                    Qayta yuborish
                  </button>
                </div>
              )}
            </div>
          )}

          {trimmed && (
            <p className="muted" style={{ fontSize: 13 }}>
              Agent oxirgi {HISTORY_LIMIT} ta xabarni ko‘radi — undan oldingilari hisobga olinmaydi.
            </p>
          )}

          <div className="tchat-form">
            <textarea
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              onKeyDown={onKey}
              placeholder="Mijoz bo‘lib yozing… (Enter — yuborish, Shift+Enter — yangi qator)"
            />
            <button type="button" disabled={busy || !draft.trim()} onClick={send}>
              {busy ? 'Kutilmoqda…' : 'Yuborish'}
            </button>
          </div>

          {last && (
            <>
              <div className="row" style={{ marginTop: 4 }}>
                <span className="badge st-pending">{last.steps_count} bosqich</span>
                <span className="muted" style={{ fontSize: 13 }}>
                  {fmt.num((last.prompt_tokens || 0) + (last.completion_tokens || 0))} token
                  {' · '}{fmt.usd(last.cost_usd)}
                  {last.error ? ` · xato: ${last.error}` : ''}
                </span>
              </div>
              {/* Kod topgan holatlar: ular model nima yozganidan qat'i
                  nazar xodimga yetkaziladi, shuning uchun testda ham
                  ko'rinib tursin. */}
              {(last.alerts || []).length > 0 && (
                <ul className="health-list">
                  {last.alerts.map((a, i) => <li key={i}>{a}</li>)}
                </ul>
              )}
              <details style={{ marginTop: 8 }}>
                <summary className="muted">Zanjir tafsiloti ({(last.steps || []).length} bosqich)</summary>
                <div style={{ marginTop: 8 }}>
                  {(last.steps || []).map((s, i) => <Step key={s.id || i} s={s} />)}
                </div>
              </details>
            </>
          )}
        </>
      )}
    </div>
  )
}
