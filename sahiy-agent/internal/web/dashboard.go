package web

// dashboardHTML — o'zi yetarli (self-contained) sahifa. Ma'lumotni
// /api/stats va /api/history dan oladi va har 30s da yangilaydi.
const dashboardHTML = `<!doctype html>
<html lang="uz">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Sahiy AI Agent — Dashboard</title>
<style>
  :root{--bg:#0f1420;--card:#1a2130;--tx:#e8edf5;--mut:#8b98ad;--acc:#4f9cff;--ok:#3ecf8e;--warn:#ffb454;}
  *{box-sizing:border-box}
  body{margin:0;font-family:system-ui,-apple-system,Segoe UI,Roboto,sans-serif;background:var(--bg);color:var(--tx)}
  header{padding:20px 24px;border-bottom:1px solid #26304200;background:#131a28}
  h1{margin:0;font-size:18px}
  .sub{color:var(--mut);font-size:13px;margin-top:4px}
  .wrap{max-width:1000px;margin:0 auto;padding:20px 16px}
  .cards{display:grid;grid-template-columns:repeat(auto-fit,minmax(160px,1fr));gap:12px;margin-bottom:20px}
  .card{background:var(--card);border:1px solid #26304a;border-radius:12px;padding:16px}
  .card .n{font-size:30px;font-weight:700}
  .card .l{color:var(--mut);font-size:13px;margin-top:4px}
  table{width:100%;border-collapse:collapse;background:var(--card);border-radius:12px;overflow:hidden}
  th,td{text-align:left;padding:10px 12px;font-size:13px;border-bottom:1px solid #26304a;vertical-align:top}
  th{color:var(--mut);font-weight:600;background:#151c2b}
  .msg{color:var(--mut)}
  .reply{color:var(--tx)}
  .tag{display:inline-block;padding:2px 8px;border-radius:20px;font-size:11px}
  .sent{background:rgba(62,207,142,.15);color:var(--ok)}
  .nosent{background:rgba(255,180,84,.15);color:var(--warn)}
  td.w{max-width:320px;overflow-wrap:anywhere}
</style>
</head>
<body>
<header>
  <h1>🤖 Sahiy AI Agent</h1>
  <div class="sub">Yordam agenti — real vaqt statistikasi (har 30s yangilanadi)</div>
</header>
<div class="wrap">
  <div class="cards" id="cards"></div>
  <h3 style="margin:8px 0 10px">So'nggi suhbatlar</h3>
  <table>
    <thead><tr><th>Vaqt</th><th>Kim</th><th>Mijoz xabari</th><th>AI javobi</th><th>Holat</th></tr></thead>
    <tbody id="rows"></tbody>
  </table>
</div>
<script>
function esc(s){return (s||'').replace(/[&<>]/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;'}[c]))}
async function load(){
  try{
    const st = await (await fetch('/api/stats')).json();
    document.getElementById('cards').innerHTML = [
      ['Odamlar', st.UniqueClients],
      ['Suhbatlar', st.UniqueChats],
      ['Javoblar', st.TotalReplies],
      ['Yuborilgan', st.SentReplies],
    ].map(([l,n])=>'<div class="card"><div class="n">'+(n||0)+'</div><div class="l">'+l+'</div></div>').join('');

    const h = await (await fetch('/api/history')).json();
    document.getElementById('rows').innerHTML = (h||[]).map(r=>{
      const t = new Date(r.time).toLocaleString('uz');
      const who = esc(r.client_name||('#'+r.client_id)) + ' <span class="msg">·#'+r.conversation_id+'</span>';
      const tag = r.sent ? '<span class="tag sent">yuborildi</span>' : '<span class="tag nosent">ko\'rsatildi</span>';
      return '<tr><td>'+t+'</td><td class="w">'+who+'</td><td class="w msg">'+esc(r.client_message)+'</td><td class="w reply">'+esc(r.ai_reply)+'</td><td>'+tag+'</td></tr>';
    }).join('') || '<tr><td colspan="5" class="msg">Hali yozuv yo\'q</td></tr>';
  }catch(e){console.error(e)}
}
load(); setInterval(load, 30000);
</script>
</body>
</html>`
