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

export default function Dashboard() {
  const [stats, setStats] = useState(null)
  const [daily, setDaily] = useState([])
  const [clients, setClients] = useState([])
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

  if (err) return <div className="err">{err}</div>
  if (!stats) return <p className="muted">Yuklanmoqda…</p>

  const max = Math.max(1, ...daily.map((d) => d.total))
  const resolvedPct = stats.total ? Math.round((stats.ai_resolved / stats.total) * 100) : 0

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
