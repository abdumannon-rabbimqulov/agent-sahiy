import { useEffect, useState } from 'react'
import { api } from '../api'

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
        name="poll_enabled" value={s.poll_enabled} onChange={change}
        title="Fon sikli"
        desc="Yangi mijoz xabarlarini davriy tekshirish (POLL_INTERVAL_SEC). O'chirilsa agent faqat qo'lda ishga tushiriladi. AI agent o'chiq bo'lsa bu sozlama ta'sir qilmaydi."
      />
    </>
  )
}
