import { useCallback, useEffect, useState } from 'react'
import { api, fmt } from '../api'

// Panel shu oraliqda yangilanadi: natija har suhbatdan keyin bazaga tushadi.
const REFRESH_MS = 3000

const TYPE_LABELS = {
  sotib_olingan: 'Sotib olingan, status 3',
  status_12: 'Status 12',
  yolda: "Yo'lga chiqib kelmagan",
  status_6_kelmagan: 'Status 6, 25 kundan oshgan',
  boshqa_kelmagan: 'Boshqa, 25 kundan oshgan',
  xato_kelgan: 'Xato kelgan',
  kuryer_olmagan: 'Kuryer olmagan',
  manzilga_yetmagan: 'Manzilga yetmagan',
}
const TYPE_ORDER = Object.keys(TYPE_LABELS)

const AI_LABELS = {
  sotib_olingan: 'Sotib olingan',
  status_12: 'Status 12',
  yolda: "Yo'lda",
  kelmagan: 'Kelmagan',
  xato_kelgan: 'Xato kelgan',
  kuryer_olmagan: 'Kuryer olmagan',
  manzilga_yetmagan: 'Manzilga yetmagan',
  boshqa: 'Boshqa',
  muammo_yoq: "Muammo yo'q",
}

const ANCHOR_LABELS = {
  created_at: 'yaratilganidan',
  shipped_at: "yo'lga chiqqanidan",
  yetkazma_created_at: 'punktga kelganidan',
}

const bucketName = (b) => `${b}+ kun`

const typeName = (t) => {
  const m = t.match(/^(.*)_(5|20|30)$/)
  if (m && TYPE_LABELS[m[1]]) return `${TYPE_LABELS[m[1]]}: ${bucketName(m[2])}`
  return TYPE_LABELS[t] || t
}

const duration = (sec) => {
  if (!sec) return '—'
  const h = Math.floor(sec / 3600)
  const m = Math.round((sec % 3600) / 60)
  return h ? `${h} soat ${m} daq` : `${m} daq`
}

function Stat({ k, v, s }) {
  return (
    <div className="card">
      <div className="k">{k}</div>
      <div className="v">{v}</div>
      {s && <div className="s">{s}</div>}
    </div>
  )
}

// Har tur uchun karta: guruhlari alohida bosiladi.
function TypeCard({ type, rows, active, onPick }) {
  const orders = rows.reduce((a, r) => a + r.orders, 0)
  const users = rows.reduce((a, r) => a + r.users, 0)
  const on = active.type === type
  return (
    <div className="card" style={on ? { borderColor: 'var(--accent)' } : undefined}>
      <button className="link-card" onClick={() => onPick(type, '')}>
        <div className="k">{TYPE_LABELS[type]}</div>
        <div className="v">{orders}</div>
        <div className="s">{users} mijoz</div>
      </button>
      {rows.some((r) => r.bucket) && (
        <div className="range" style={{ marginTop: 8 }}>
          {[...rows].sort((a, b) => Number(a.bucket) - Number(b.bucket)).map((r) => (
            <button key={r.bucket}
                    className={on && active.bucket === r.bucket ? 'on' : ''}
                    onClick={() => onPick(type, r.bucket)}>
              {bucketName(r.bucket)}: {r.orders} ({r.users} mijoz)
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

export default function Collect() {
  const [status, setStatus] = useState(null)
  const [sum, setSum] = useState(null)
  const [recent, setRecent] = useState([])
  const [pick, setPick] = useState({ type: '', bucket: '' })
  const [orders, setOrders] = useState({ items: [], total: 0 })
  const [noNum, setNoNum] = useState({ items: [], total: 0 })
  const [clients, setClients] = useState({ items: [], total: 0 })
  const [aiPick, setAiPick] = useState({ type: '', after: false })
  const [tab, setTab] = useState('clients')
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)

  const load = useCallback(async () => {
    try {
      const [st, su, re, or, nn, cl] = await Promise.all([
        api.collectStatus(),
        api.collectSummary(),
        api.collectRecent(30),
        api.collectOrders(pick.type, pick.bucket, '', 1, 100),
        api.collectChats('no_numbers', 1, 100),
        api.collectClients(aiPick.type, aiPick.after),
      ])
      setClients({ items: cl.items || [], total: cl.total })
      setStatus(st); setSum(su); setRecent(re.items || [])
      setOrders({ items: or.items || [], total: or.total })
      setNoNum({ items: nn.items || [], total: nn.total })
      setErr('')
    } catch (e) {
      setErr(e.message)
    }
  }, [pick, aiPick])

  useEffect(() => {
    load()
    const id = setInterval(load, REFRESH_MS)
    return () => clearInterval(id)
  }, [load])

  const act = async (fn) => {
    setBusy(true)
    try { await fn(); await load() } catch (e) { setErr(e.message) } finally { setBusy(false) }
  }

  const removeOrder = (sn) => {
    if (window.confirm(`${sn} buyurtmasi hisobotdan butunlay o'chirilsinmi? Hamma muammo turlari va AI xulosasi o'chadi, keyingi tekshiruvlarda ham qaytmaydi.`)) {
      act(() => api.collectDeleteOrder(sn))
    }
  }
  const removeChat = (id) => {
    if (window.confirm(`#${id} murojaati hisobotdan o'chirilsinmi? Mijoz yangi xabar yozsa yana chiqadi.`)) {
      act(() => api.collectDeleteChat(id))
    }
  }

  const onPick = (type, bucket) => {
    setPick((p) => (p.type === type && p.bucket === bucket ? { type: '', bucket: '' } : { type, bucket }))
    setTab('orders')
  }

  if (!status || !sum) return err ? <div className="err">{err}</div> : <div className="muted">Yuklanmoqda…</div>

  const byType = {}
  for (const r of sum.types || []) (byType[r.type] ||= []).push(r)
  const pct = status.total ? Math.round((status.done / status.total) * 100) : 0

  return (
    <div>
      <div className="spread" style={{ marginBottom: 14 }}>
        <div>
          <h2 style={{ margin: 0 }}>Yig'ilgan ma'lumot</h2>
          <div className="muted" style={{ fontSize: 13 }}>
            {status.mode
              ? "Yig'ish rejimi yoqiq: AI mijozga javob yozmaydi. Xodimning Telegram javobi odatdagidek ketadi."
              : "Yig'ish rejimi o'chiq: agent oddiy javob zanjirida ishlaydi."}
          </div>
        </div>
        <div className="row">
          {status.running || status.wanted
            ? <button className="ghost" disabled={busy} onClick={() => act(api.collectStop)}>To'xtatish</button>
            : <button disabled={busy || !status.mode} onClick={() => act(() => api.collectStart(0))}>Boshlash</button>}
        </div>
      </div>

      {err && <div className="err">{err}</div>}
      {status.note && <div className="warn-box card" style={{ marginBottom: 12 }}>{status.note}</div>}

      <div className="cards">
        <Stat k="O'rganildi" v={sum.chats} s={status.running ? `Yurishda ${status.done} / ${status.total} (${pct}%)` : 'Hozir yurish yo\'q'} />
        <Stat k="Qoldi" v={status.remaining} s={status.running ? `Taxminan ${duration(status.eta_sec)}` : undefined} />
        <Stat k="Tezlik" v={status.running ? `${status.per_min.toFixed(1)}/daq` : '—'}
              s={status.running ? `Oraliq ${(status.gap_ms / 1000).toFixed(1)}s, ${status.workers} worker` : `Maqsad ${status.target_min} daqiqa`} />
        <Stat k="Raqamli" v={sum.with_numbers} s={`Raqamsiz: ${sum.no_numbers}`} />
        <Stat k="Muammoli buyurtma" v={sum.problem_orders} s={`${sum.problem_users} mijoz, takrorsiz`} />
        <Stat k="Xato" v={sum.partial + sum.failed} s={`Qayta tekshiriladi. Yurishda: ${status.errors}`} />
      </div>

      <h3>Muammo turlari</h3>
      <div className="cards">
        {TYPE_ORDER.map((t) => (
          <TypeCard key={t} type={t} rows={byType[t] || [{ bucket: '', orders: 0, users: 0 }]} active={pick} onPick={onPick} />
        ))}
      </div>

      <h3>AI xulosasi: mijozlar muammosi</h3>
      <div className="muted" style={{ fontSize: 13, marginBottom: 8 }}>
        Kod hisoblagan turdan alohida. Mijoz yozgan gapga tayanadi.
        {sum.ai_blank > 0 && ` Xulosasiz buyurtma: ${sum.ai_blank}.`}
      </div>
      <div className="cards">
        {(sum.ai_types || []).map((r) => (
          <div key={r.type} className="card" style={aiPick.type === r.type ? { borderColor: 'var(--accent)' } : undefined}>
            <button className="link-card" onClick={() => { setAiPick((p) => ({ ...p, type: p.type === r.type ? '' : r.type })); setTab('clients') }}>
              <div className="k">{AI_LABELS[r.type] || r.type}</div>
              <div className="v">{r.clients}</div>
              <div className="s">mijoz, {r.items} ta murojaat/buyurtma{r.arrived_after > 0 && `, ${r.arrived_after} tasi keyin kelgan`}</div>
            </button>
          </div>
        ))}
        <div className="card" style={aiPick.after ? { borderColor: 'var(--accent)' } : undefined}>
          <button className="link-card" onClick={() => { setAiPick((p) => ({ ...p, after: !p.after })); setTab('clients') }}>
            <div className="k">Yozganidan keyin tovari kelgan</div>
            <div className="v">{sum.arrived_after}</div>
            <div className="s">mijoz yozgan sanadan keyin yetkazmada chiqqan</div>
          </button>
        </div>
      </div>

      <div className="range" style={{ marginBottom: 12 }}>
        <button className={tab === 'clients' ? 'on' : ''} onClick={() => setTab('clients')}>
          Mijozlar ({clients.total}){aiPick.type && `: ${AI_LABELS[aiPick.type] || aiPick.type}`}{aiPick.after && ', keyin kelgan'}
        </button>
        <button className={tab === 'orders' ? 'on' : ''} onClick={() => setTab('orders')}>
          Buyurtmalar ({orders.total}){pick.type && `: ${TYPE_LABELS[pick.type]}${pick.bucket ? ` ${pick.bucket}+` : ''}`}
        </button>
        <button className={tab === 'live' ? 'on' : ''} onClick={() => setTab('live')}>Jonli lenta</button>
        <button className={tab === 'none' ? 'on' : ''} onClick={() => setTab('none')}>Raqamsiz ({noNum.total})</button>
      </div>

      {tab === 'clients' && (
        <div className="card scroll-x">
          <table>
            <thead>
              <tr><th>Mijoz</th><th>Buyurtma</th><th>Muammo turi (AI)</th><th>AI izohi</th><th>Yozgan sana</th><th>Tovar kelgan sana</th><th>Topshirilgan</th><th></th></tr>
            </thead>
            <tbody>
              {clients.items.map((c) => {
                const after = c.arrived_at && c.client_wrote_at && new Date(c.arrived_at) > new Date(c.client_wrote_at)
                return (
                  <tr key={`${c.conversation_id}-${c.order_sn}`}>
                    <td>{c.client_id}<div className="muted" style={{ fontSize: 12 }}>suhbat #{c.conversation_id}</div></td>
                    <td>{c.order_sn || <span className="muted">raqamsiz</span>}{c.order_sn && <div className="muted" style={{ fontSize: 12 }}>status {c.status}</div>}</td>
                    <td><strong>{AI_LABELS[c.ai_type] || c.ai_type}</strong></td>
                    <td style={{ fontSize: 13, minWidth: 220 }}>{c.ai_note}</td>
                    <td style={{ whiteSpace: 'nowrap' }}>{fmt.date(c.client_wrote_at)}</td>
                    <td style={{ whiteSpace: 'nowrap' }}>
                      {c.arrived_at ? fmt.date(c.arrived_at) : <span className="muted">{c.order_sn ? 'kelmagan' : '—'}</span>}
                      {after && <div><span className="badge st-sent">yozganidan keyin</span></div>}
                      {c.branch && <div className="muted" style={{ fontSize: 12 }}>{c.branch}</div>}
                    </td>
                    <td style={{ whiteSpace: 'nowrap' }}>{c.delivered_at ? fmt.date(c.delivered_at) : <span className="muted">—</span>}{c.received && <div className="muted" style={{ fontSize: 12 }}>mijoz olgan</div>}</td>
                    <td><button className="ghost" disabled={busy} onClick={() => (c.order_sn ? removeOrder(c.order_sn) : removeChat(c.conversation_id))}>O'chirish</button></td>
                  </tr>
                )
              })}
              {!clients.items.length && <tr><td colSpan="8" className="muted">Hali xulosa yo'q</td></tr>}
            </tbody>
          </table>
          {clients.total > clients.items.length && <div className="muted" style={{ fontSize: 13, marginTop: 8 }}>Birinchi {clients.items.length} tasi ko'rsatildi, jami {clients.total}.</div>}
        </div>
      )}

      {tab === 'orders' && (
        <div className="card scroll-x">
          <table>
            <thead>
              <tr><th>Buyurtma</th><th>Mijoz</th><th>Status</th><th>Necha kun</th><th>Manba</th><th>Yetkazma</th><th>Kod turi</th><th>AI xulosasi</th><th></th></tr>
            </thead>
            <tbody>
              {orders.items.map((o) => (
                <tr key={`${o.order_sn}-${o.type}`}>
                  <td><strong>{o.order_sn}</strong><div className="muted" style={{ fontSize: 12 }}>{o.express_num}</div></td>
                  <td>{o.user_id}<div className="muted" style={{ fontSize: 12 }}>suhbat #{o.conversation_id}</div></td>
                  <td>{o.status}<div className="muted" style={{ fontSize: 12 }}>{o.status_label}</div></td>
                  <td>{o.days ?? '—'}<div className="muted" style={{ fontSize: 12 }}>{ANCHOR_LABELS[o.anchor_kind] || 'yaratilganidan'}</div></td>
                  <td>{o.source === 'akkaunt' ? <span className="muted">akkauntdan, raqam aytmagan</span> : o.source}</td>
                  <td>
                    {!o.verified ? 'tekshirilmadi' : o.received ? 'mijoz olgan' : o.arrived ? (o.branch || 'kelgan') : "yo'q"}
                  </td>
                  <td>{(o.types || []).map((t) => <div key={t} style={{ fontSize: 13 }}>{typeName(t)}</div>)}{!(o.types || []).length && <span className="muted">—</span>}</td>
                  <td>{o.ai_type ? <><strong>{AI_LABELS[o.ai_type] || o.ai_type}</strong><div className="muted" style={{ fontSize: 12 }}>{o.ai_note}</div></> : <span className="muted">—</span>}</td>
                  <td><button className="ghost" disabled={busy} onClick={() => removeOrder(o.order_sn)}>O'chirish</button></td>
                </tr>
              ))}
              {!orders.items.length && <tr><td colSpan="9" className="muted">Hali buyurtma yo'q</td></tr>}
            </tbody>
          </table>
          {orders.total > orders.items.length && <div className="muted" style={{ fontSize: 13, marginTop: 8 }}>Birinchi {orders.items.length} tasi ko'rsatildi, jami {orders.total}.</div>}
        </div>
      )}

      {tab === 'live' && (
        <div className="card scroll-x">
          <table>
            <thead><tr><th>Vaqt</th><th>Suhbat</th><th>Raqamlar</th><th>Natija</th><th>AI xulosasi</th></tr></thead>
            <tbody>
              {recent.map((c) => (
                <tr key={c.conversation_id}>
                  <td style={{ whiteSpace: 'nowrap' }}>{fmt.date(c.processed_at)}</td>
                  <td>#{c.conversation_id}<div className="muted" style={{ fontSize: 12 }}>mijoz {c.client_id}</div></td>
                  <td style={{ fontSize: 13 }}>{c.numbers || <span className="muted">raqamsiz</span>}{c.not_found && <div className="muted">topilmadi: {c.not_found}</div>}</td>
                  <td style={{ fontSize: 13 }}>
                    {(c.orders || []).map((o) => (
                      <div key={o.order_sn}>{o.order_sn}, status {o.status}: {(o.types || []).map(typeName).join('; ') || "muammo yo'q"}</div>
                    ))}
                    {c.error && <div className="muted">xato, qayta ko'riladi</div>}
                  </td>
                  <td style={{ fontSize: 13 }}>
                    {(c.orders || []).filter((o) => o.ai_type).map((o) => <div key={o.order_sn}>{AI_LABELS[o.ai_type] || o.ai_type}: {o.ai_note}</div>)}
                    {c.ai_type && <div>{AI_LABELS[c.ai_type] || c.ai_type}: {c.ai_note}</div>}
                  </td>
                </tr>
              ))}
              {!recent.length && <tr><td colSpan="5" className="muted">Hali o'rganilgan suhbat yo'q</td></tr>}
            </tbody>
          </table>
        </div>
      )}

      {tab === 'none' && (
        <div className="card scroll-x">
          <table>
            <thead><tr><th>Suhbat</th><th>Mijoz yozgani</th><th>AI xulosasi</th><th></th></tr></thead>
            <tbody>
              {noNum.items.map((c) => (
                <tr key={c.conversation_id}>
                  <td>#{c.conversation_id}<div className="muted" style={{ fontSize: 12 }}>mijoz {c.client_id}</div></td>
                  <td style={{ fontSize: 13 }}>{c.last_text}</td>
                  <td style={{ fontSize: 13 }}>{c.ai_type ? <><strong>{AI_LABELS[c.ai_type] || c.ai_type}</strong><div className="muted">{c.ai_note}</div></> : <span className="muted">—</span>}</td>
                  <td><button className="ghost" disabled={busy} onClick={() => removeChat(c.conversation_id)}>O'chirish</button></td>
                </tr>
              ))}
              {!noNum.items.length && <tr><td colSpan="4" className="muted">Raqamsiz suhbat yo'q</td></tr>}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
