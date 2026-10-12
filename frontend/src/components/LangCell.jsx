// Mijoz bilan yozishish tili: bazadagi lang+script juftligi bitta
// tanlovga aylantiriladi. Ikki joyda ishlatiladi — Dashboard jadvali va
// Tasdiqlash navbatidagi karta.
import { useState } from 'react'
import { api } from '../api'

const LANGS = [
  { v: 'uzb:lotin', label: "O'zbekcha (lotin)" },
  { v: 'uzb:kirill', label: "O'zbekcha (kirill)" },
  { v: 'rus:', label: 'Ruscha' },
]

// langValue - qatordagi tilning tanlov qiymati ("uzb:lotin").
// Til hali aniqlanmagan bo'lsa bo'sh satr.
export function langValue(c) {
  if (!c.lang) return ''
  return c.lang === 'uzb' ? `uzb:${c.script || 'lotin'}` : 'rus:'
}

// LangCell - tilni ko'rsatadi va shu yerda o'zgartirishga ruxsat beradi.
// Saqlangach qator yangilanadi (onSaved), xato bo'lsa eski qiymat qoladi.
export default function LangCell({ client, onSaved }) {
  const [saving, setSaving] = useState(false)
  const [err, setErr] = useState('')
  const value = langValue(client)

  async function change(next) {
    if (!next || next === value) return
    const [lang, script] = next.split(':')
    setSaving(true)
    setErr('')
    try {
      const row = await api.setClientLang(client.client_id, lang, script)
      onSaved(row)
    } catch (e) {
      setErr(e.message)
    } finally {
      setSaving(false)
    }
  }

  return (
    <>
      <select
        value={value}
        disabled={saving}
        onChange={(e) => change(e.target.value)}
      >
        {!value && <option value="">aniqlanmagan</option>}
        {LANGS.map((l) => (
          <option key={l.v} value={l.v}>{l.label}</option>
        ))}
      </select>
      {err && <div className="err" style={{ marginTop: 4 }}>{err}</div>}
    </>
  )
}
