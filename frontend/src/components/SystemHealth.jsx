// Tashqi tizimlar holati. Alohida komponent: tekshiruv tashqi API'larga
// so'rov yuboradi (bir necha sekund ketishi mumkin), shuning uchun
// statistikani kutib turmaydi va o'zini har daqiqada yangilaydi.
//
// Nega kerak: adminka tokeni .env dan qo'lda qo'yiladi va o'zi
// yangilanmaydi. U tugasa agent ishlashda davom etadi, lekin buyurtma
// ma'lumotini ko'rmay qoladi — buni panelda darhol ko'rish kerak.
import { useCallback, useEffect, useState } from 'react'
import { api } from '../api'

// Yangilanish oralig'i (backend natijani 60s keshlaydi).
const REFRESH_MS = 60_000

const LABEL = {
  ok: { text: 'Ishlayapti', cls: 'st-sent' },
  warn: { text: 'E’tibor bering', cls: 'st-pending' },
  fail: { text: 'Ishlamayapti', cls: 'st-failed' },
}

// daysText - token tugashiga qancha qolgani.
function daysText(s) {
  if (s.days_left == null) return null
  const d = s.days_left
  if (d <= 0) return 'muddati tugagan'
  if (d < 1) return `${Math.round(d * 24)} soat qoldi`
  return `${Math.floor(d)} kun qoldi`
}

export default function SystemHealth() {
  const [rep, setRep] = useState(null)
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)

  const load = useCallback((force = false) => {
    setBusy(true)
    api
      .health(force)
      .then((r) => {
        setRep(r)
        setErr('')
      })
      .catch((e) => setErr(e.message))
      .finally(() => setBusy(false))
  }, [])

  useEffect(() => {
    load()
    const t = setInterval(() => load(), REFRESH_MS)
    return () => clearInterval(t)
  }, [load])

  if (err) return <div className="err">Tizimlar holati o‘qilmadi: {err}</div>
  if (!rep) return <p className="muted">Tizimlar tekshirilmoqda…</p>

  const broken = rep.systems.filter((s) => s.status !== 'ok')

  return (
    <>
      <div className="spread">
        <h2 style={{ margin: 0 }}>Tizimlar holati</h2>
        <button type="button" className="ghost" disabled={busy} onClick={() => load(true)}>
          {busy ? 'Tekshirilmoqda…' : 'Qayta tekshirish'}
        </button>
      </div>

      {/* Banner: biror tizim ishlamasa, nima ishlamay qolgani ham yozilsin. */}
      {broken.length > 0 && (
        <div className={rep.status === 'fail' ? 'err' : 'ok warn-box'}>
          <b>
            {rep.status === 'fail'
              ? `${broken.length} ta tizim ishlamayapti`
              : `${broken.length} ta tizimga e’tibor kerak`}
          </b>
          <ul className="health-list">
            {broken.map((s) => (
              <li key={s.key}>
                <b>{s.name}</b> — {s.message}
                {s.affects && <span className="muted"> · ishlamaydi: {s.affects}</span>}
              </li>
            ))}
          </ul>
        </div>
      )}

      <div className="health">
        {rep.systems.map((s) => {
          const l = LABEL[s.status] || LABEL.fail
          return (
            <div className="health-row" key={s.key}>
              <span className={`badge ${l.cls}`}>{l.text}</span>
              <div className="health-name">
                {s.name}
                {s.manual && <span className="muted"> · token qo‘lda yangilanadi</span>}
              </div>
              <div className="health-msg muted">{s.message}</div>
              <div className="health-exp muted">{daysText(s) || ''}</div>
            </div>
          )
        })}
      </div>
    </>
  )
}
