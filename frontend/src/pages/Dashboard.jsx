import { useEffect, useState } from 'react'
import { api, fmt } from '../api'

function Card({ k, v, s }) {
  return (
    <div className="card">
      <div className="k">{k}</div>
      <div className="v">{v}</div>
      {s && <div className="s">{s}</div>}
    </div>
  )
}

// Kunlik hisobotda ko'rsatiladigan oraliqlar.
const REPORT_RANGES = [7, 14, 30, 90, 180, 365]

export default function Dashboard() {
  const [stats, setStats] = useState(null)
  const [daily, setDaily] = useState([])
  const [clients, setClients] = useState([])
  const [report, setReport] = useState([])
  const [days, setDays] = useState(30)
  const [err, setErr] = useState('')

  useEffect(() => {
    Promise.all([api.stats(), api.daily(14), api.clients(30, 15)])
      .then(([s, d, c]) => {
        setStats(s)
        setDaily([...d].reverse())
        setClients(c)
      })
      .catch((e) => setErr(e.message))
  }, [])

  // Kunlik hisobot alohida yuklanadi: oraliq o'zgarganda qolgan
  // bo'limlar qayta so'ralmasin.
  useEffect(() => {
    api.report(days).then(setReport).catch((e) => setErr(e.message))
  }, [days])

  if (err) return <div className="err">{err}</div>
  if (!stats) return <p className="muted">Yuklanmoqda…</p>

  const max = Math.max(1, ...daily.map((d) => d.total))
  const resolvedPct = stats.total ? Math.round((stats.ai_resolved / stats.total) * 100) : 0
  // Bugun guruhga ketgan yordam so'rovlarining necha foiziga javob kelgan.
  const answeredPct = stats.help_today
    ? Math.round((stats.help_answered_today / stats.help_today) * 100)
    : 0

  return (
    <>
      <h1>Statistika</h1>
      <p className="hint">AI agent qancha murojaatni hal qildi va qancha token sarfladi</p>

      <div className="cards">
        <Card k="Jami murojaat" v={fmt.num(stats.total)} s={`${fmt.num(stats.unique_chats)} suhbat`} />
        <Card k="AI hal qilgan" v={fmt.num(stats.ai_resolved)} s={`${resolvedPct}% — xodimsiz`} />
        <Card k="Xodim kerak bo'lgan" v={fmt.num(stats.needed_staff)} s="help → Telegram" />
        <Card k="Tasdiq kutmoqda" v={fmt.num(stats.pending)} s="navbatda" />
        <Card k="Unikal mijozlar" v={fmt.num(stats.unique_clients)} />
        <Card k="Xato" v={fmt.num(stats.failed)} s={`${fmt.num(stats.rejected)} rad etilgan`} />
      </div>

      <h2>Bugungi hisobot</h2>
      <div className="cards">
        <Card k="Yangi muammoli buyurtma" v={fmt.num(stats.issues_opened_today)} s="bugun aniqlangan" />
        <Card k="Hal qilingan" v={fmt.num(stats.issues_resolved_today)} s="bugun yopilgan" />
        <Card k="Ochiq muammo" v={fmt.num(stats.issues_open)} s="hozir kutmoqda" />
        <Card k="O'rtacha hal qilish" v={stats.issues_avg_hours ? `${stats.issues_avg_hours.toFixed(1)} soat` : '—'} />
        <Card k="Bugungi murojaat" v={fmt.num(stats.total_today)}
              s={`${fmt.num(stats.tokens_today)} token · ${fmt.usd(stats.cost_today)}`} />
        <Card k="Bugun tasdiqlangan" v={fmt.num(stats.approved_today)} s="admin yuborgan" />
        <Card k="Bugun avto yuborilgan" v={fmt.num(stats.sent_today)} s="AI o'zi yuborgan" />
        <Card k="Bugun rad etilgan" v={fmt.num(stats.rejected_today)} />
      </div>

      <h2>Telegram guruh — bugun</h2>
      <p className="hint">Mutaxassislardan necha marta yordam so'ralgan va nechtasiga javob kelgan</p>
      <div className="cards">
        <Card k="Yordam so'ralgan" v={fmt.num(stats.help_today)} s="bugun guruhga ketgan" />
        <Card k="Mutaxassis javob bergan" v={fmt.num(stats.staff_replies_today)}
              s="bugun — «xodim javobidan»" />
        <Card k="So'rovlardan javob olgan" v={fmt.num(stats.help_answered_today)}
              s={`bugungi so'rovlarning ${answeredPct}%`} />
        <Card k="Javobsiz" v={fmt.num(stats.help_unanswered_today)}
              s={`jami javobsiz: ${fmt.num(stats.help_unanswered_total)}`} />
        <Card k="Muammoli buyurtma guruhda" v={fmt.num(stats.issues_notified_today)}
              s={`${fmt.num(stats.issues_telegram_today)} yopilgan · ${fmt.num(stats.issues_reminded_today)} eslatma`} />
      </div>

      <h2>Kunlik hisobot</h2>
      <p className="hint">
        Yuqoridagi kartalarning har kun uchun varianti. Ma'lumot boshidan beri
        saqlangan — oraliqni tanlang. «O'rtacha» shu kuni yopilgan muammolarniki
        (yuqoridagi karta esa butun davr bo'yicha), «Kun oxirida ochiq» — o'sha
        kun tugaganda hali yopilmagan muammolar soni.
      </p>
      <div className="range">
        {REPORT_RANGES.map((n) => (
          <button
            key={n}
            type="button"
            className={n === days ? 'on' : ''}
            onClick={() => setDays(n)}
          >
            {n} kun
          </button>
        ))}
      </div>
      <div className="scroll-x">
        <table className="report">
          <thead>
            <tr>
              <th rowSpan="2">Kun</th>
              <th colSpan="5">Muammoli buyurtmalar</th>
              <th colSpan="5">Murojaatlar</th>
              <th colSpan="3">Telegram guruh</th>
            </tr>
            <tr>
              <th>Yangi</th><th>Hal qilingan</th><th>Kun oxirida ochiq</th>
              <th>O'rtacha</th><th>Guruhda</th>
              <th>Jami</th><th>Avto</th><th>Tasdiqlangan</th><th>Rad</th><th>Token</th>
              <th>So'ralgan</th><th>Javob olgan</th><th>Javobsiz</th>
            </tr>
          </thead>
          <tbody>
            {report.map((d) => (
              <tr key={d.day}>
                <td>{fmt.day(d.day)}</td>
                <td>{fmt.num(d.issues_opened)}</td>
                <td>{fmt.num(d.issues_resolved)}</td>
                <td>{fmt.num(d.issues_open_end)}</td>
                <td className="muted">
                  {d.issues_avg_hours ? `${d.issues_avg_hours.toFixed(1)} s` : '—'}
                </td>
                <td className="muted">{fmt.num(d.issues_notified)}</td>
                <td>{fmt.num(d.total)}</td>
                <td>{fmt.num(d.sent)}</td>
                <td>{fmt.num(d.approved)}</td>
                <td>{fmt.num(d.rejected)}</td>
                <td className="muted">{fmt.num(d.tokens)}</td>
                <td>{fmt.num(d.help_sent)}</td>
                <td>{fmt.num(d.help_answered)}</td>
                <td>{fmt.num(d.help_unanswered)}</td>
              </tr>
            ))}
            {report.length === 0 && (
              <tr><td colSpan="14" className="muted">Ma'lumot yo'q</td></tr>
            )}
          </tbody>
        </table>
      </div>

      <h2>Tokenlar va xarajat</h2>
      <div className="cards">
        <Card k="Jami token" v={fmt.num(stats.total_tokens)} s={`${fmt.num(stats.calls)} so'rov`} />
        <Card k="Kirish" v={fmt.num(stats.prompt_tokens)} s={`kesh: ${fmt.num(stats.cached_tokens)}`} />
        <Card k="Chiqish" v={fmt.num(stats.completion_tokens)} />
        <Card k="Bugun" v={fmt.num(stats.tokens_today)} s={fmt.usd(stats.cost_today)} />
        <Card k="Shu oy" v={fmt.usd(stats.cost_month)} />
        <Card k="Jami xarajat" v={fmt.usd(stats.cost_total)} />
      </div>

      <h2>Oxirgi 14 kun</h2>
      {daily.length === 0 ? (
        <p className="muted">Ma'lumot yo'q</p>
      ) : (
        <div className="chart">
          {daily.map((d) => (
            <div className="bar" key={d.day} title={`${fmt.day(d.day)}: ${d.total} murojaat, ${fmt.num(d.tokens)} token`}>
              <div className="fill" style={{ height: `${(d.total / max) * 100}%` }} />
              <div className="lb">{fmt.day(d.day)}</div>
            </div>
          ))}
        </div>
      )}

      <h2>Mijozlar (30 kun)</h2>
      <table>
        <thead>
          <tr>
            <th>Mijoz</th><th>Murojaat</th><th>AI hal qilgan</th><th>Xodim kerak</th>
            <th>Token</th><th>Xarajat</th><th>Oxirgi</th>
          </tr>
        </thead>
        <tbody>
          {clients.map((c) => (
            <tr key={c.client_id}>
              <td>{c.client_id}</td>
              <td>{c.total}</td>
              <td>{c.ai_resolved}</td>
              <td>{c.needed_help}</td>
              <td>{fmt.num(c.tokens)}</td>
              <td>{fmt.usd(c.cost)}</td>
              <td className="muted">{fmt.date(c.last_at)}</td>
            </tr>
          ))}
          {clients.length === 0 && <tr><td colSpan="7" className="muted">Ma'lumot yo'q</td></tr>}
        </tbody>
      </table>
    </>
  )
}
