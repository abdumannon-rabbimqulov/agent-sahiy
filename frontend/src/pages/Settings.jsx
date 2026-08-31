import { useEffect, useState } from 'react'
import { api } from '../api'

// Sonli sozlama: kiritib "Saqlash" bosiladi.
function Number_({ name, value, onSave, title, desc, unit, min, max }) {
  const [v, setV] = useState(String(value ?? ''))
  const [busy, setBusy] = useState(false)
  const changed = String(value ?? '') !== v

  const save = async () => {
    setBusy(true)
    await onSave(name, Number(v))
    setBusy(false)
  }

  return (
    <div className="card" style={{ marginBottom: 12 }}>
      <div className="spread">
        <div style={{ flex: 1 }}>
          <strong>{title}</strong>
          <div className="muted" style={{ fontSize: 13 }}>{desc}</div>
        </div>
        <div className="row" style={{ flex: 'none' }}>
          <input type="number" min={min} max={max} value={v}
                 onChange={(e) => setV(e.target.value)} style={{ width: 90 }} />
          <span className="muted" style={{ fontSize: 13 }}>{unit}</span>
          <button onClick={save} disabled={busy || !changed}>Saqlash</button>
        </div>
      </div>
    </div>
  )
}

// Bitta yoqib-o'chiriladigan sozlama.
function Toggle({ name, value, onChange, title, desc }) {
  return (
    <div className="card" style={{ marginBottom: 12 }}>
      <div className="spread">
        <div>
          <strong>{title}</strong>
          <div className="muted" style={{ fontSize: 13 }}>{desc}</div>
        </div>
        <button className={value ? '' : 'ghost'} onClick={() => onChange(name, !value)}>
          {value ? 'YOQILGAN' : 'O‘CHIQ'}
        </button>
      </div>
    </div>
  )
}

export default function Settings() {
  const [s, setS] = useState(null)
  const [err, setErr] = useState('')
  const [msg, setMsg] = useState('')

  useEffect(() => {
    api.settings().then(setS).catch((e) => setErr(e.message))
  }, [])

  const change = async (key, value) => {
    setErr(''); setMsg('')
    try {
      setS(await api.saveSettings({ [key]: value }))
      setMsg('Saqlandi')
    } catch (e) {
      setErr(e.message)
    }
  }

  if (err && !s) return <div className="err">{err}</div>
  if (!s) return <p className="muted">Yuklanmoqda…</p>

  return (
    <>
      <h1>Sozlamalar</h1>
      <p className="hint">Global tugmalar — darhol kuchga kiradi, qayta ishga tushirish shart emas.</p>

      {err && <div className="err">{err}</div>}
      {msg && <div className="ok">{msg}</div>}

      <Toggle
        name="agent_enabled" value={s.agent_enabled} onChange={change}
        title="AI agent"
        desc="O'chirilsa agent butunlay to'xtaydi: yangi murojaatlarga javob tayyorlanmaydi, Groq'ga so'rov ketmaydi, token sarflanmaydi. Navbatdagi tayyor javoblarni tasdiqlash esa ishlayveradi."
      />
      {!s.agent_enabled && (
        <div className="err" style={{ marginBottom: 12 }}>
          AI agent hozir <strong>o'chirilgan</strong> — yangi mijoz xabarlariga javob tayyorlanmaydi.
        </div>
      )}
      <Toggle
        name="auto_reply" value={s.auto_reply} onChange={change}
        title="Avtomatik javob"
        desc="Yoqilsa: AI javobi (chat) mijozga darhol ketadi. O'chiq bo'lsa chat tasdiqlash navbatida kutadi. help esa har doim tasdiqsiz, darhol Telegram guruhga yuboriladi."
      />
      <Toggle
        name="auto_resolve" value={s.auto_resolve} onChange={change}
        title="Javobdan keyin suhbatni yopish"
        desc="Mijozga javob ketgach suhbat support tizimida 'hal qilindi' holatiga o'tadi. Mijoz yana yozsa, support o'zi qayta ochadi."
      />

      <h2>Ishlash tezligi</h2>
      <p className="hint">
        Sekinlashtirsangiz model va tashqi API'lar kamroq bosiladi
        (tezlik chegarasiga urilmaydi), lekin javob kechroq tayyorlanadi.
        O'zgarish darhol kuchga kiradi.
      </p>

      <Number_
        name="poll_interval_sec" value={s.poll_interval_sec} onSave={change}
        title="Tekshirish oralig'i" unit="sekund" min={10} max={3600}
        desc="Yangi mijoz xabarlari necha soniyada bir tekshiriladi (masalan 60 — har daqiqada, 300 — har 5 daqiqada)."
      />
      <Number_
        name="batch_size" value={s.batch_size} onSave={change}
        title="Bir siklda nechta suhbat" unit="ta" min={1} max={50}
        desc="Bitta tekshiruvda ko'pi bilan shuncha suhbatga javob tayyorlanadi. Qolganlari yo'qolmaydi — keyingi siklda navbat bilan olinadi."
      />
      <Number_
        name="chat_delay_sec" value={s.chat_delay_sec} onSave={change}
        title="Suhbatlar orasidagi tanaffus" unit="sekund" min={0} max={600}
        desc="Har suhbatdan keyin shuncha kutiladi. 0 — kutmasdan ketma-ket ishlaydi."
      />

      <h2>Boshqa</h2>
      <Toggle
        name="poll_enabled" value={s.poll_enabled} onChange={change}
        title="Fon sikli"
        desc="Yangi mijoz xabarlarini davriy tekshirish (POLL_INTERVAL_SEC). O'chirilsa agent faqat qo'lda ishga tushiriladi. AI agent o'chiq bo'lsa bu sozlama ta'sir qilmaydi."
      />
    </>
  )
}
