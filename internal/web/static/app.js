function esc(s){return (s||'').replace(/[&<>]/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;'}[c]))}

// safeURL — havola mijoz xabaridan keladi (ishonchsiz). Faqat http(s) ga
// ruxsat beriladi va qo'shtirnoq atributdan chiqib ketmasligi uchun kodlanadi.
function safeURL(u){
  if(!/^https?:\/\//i.test(u||'')) return '';
  return esc(u).replace(/"/g,'&quot;');
}

// Nusxalanadigan "chip" — bosilganda qiymat clipboard'ga tushadi.
function chip(value, label, cls){
  const v = String(value);
  return `<span class="cp ${cls||''}" data-copy="${esc(v)}" title="Nusxalash uchun bosing">${esc(label||v)}</span>`;
}

// Markdownni tozalash — Gemini ba'zan **qalin**, ## sarlavha yoki `kod`
// yozadi; dashboardda ular yulduzcha bo'lib ko'rinmasligi kerak.
function stripMd(s){
  return (s||'')
    .replace(/```[\s\S]*?```/g, ' ')      // kod bloklari
    .replace(/[*_]{1,3}([^*_\n]+)[*_]{1,3}/g, '$1')  // **qalin**, _kursiv_
    .replace(/`([^`\n]+)`/g, '$1')         // `kod`
    .replace(/^\s{0,3}#{1,6}\s+/gm, '')    // ## sarlavha (# raqamdan oldin saqlanadi)
    .replace(/^\s{0,3}[-*+]\s+/gm, '• ')   // ro'yxat belgilari
    .replace(/[*_`]/g, '')                 // qolgan yolg'iz belgilar
    .replace(/#(?!\d)/g, '')               // # faqat raqam oldida qoladi
    .replace(/[ \t]{2,}/g, ' ')
    .replace(/\n{3,}/g, '\n\n')
    .trim();
}

// Faqat ID, buyurtma raqami va track raqami nusxalanadigan chip bo'ladi.
// Sana, og'irlik, narx kabi qisqa sonlar tegilmaydi.
//   #4175802 / №4417      → buyurtma raqami
//   79017498359954        → track (8+ xonali)
//   YT7594703873671, DG60582375 → harfli track kodi
const NUM_RE = /(?:№|#)\s?\d{3,}|\b[A-Z]{2}[A-Z0-9]*\d[A-Z0-9]*\b|\b\d{8,}\b/g;
function copyNums(escaped){
  return (escaped||'').replace(NUM_RE, m => {
    if(/^[A-Z]/.test(m) && m.length < 8) return m;   // qisqa harfli so'z emas
    const val = m.replace(/^[№#]\s?/, '');           // nusxaga belgisiz qiymat
    return chip(val, m, 'inline');
  });
}

// Uzun matnni qisqartiradi; to'lig'i bosilganda ochiladi.
function clamp(html, raw, limit){
  if((raw||'').length <= limit) return html;
  return `<div class="clip">${html}</div>`;
}

async function copyText(text){
  try{
    if(navigator.clipboard && window.isSecureContext){
      await navigator.clipboard.writeText(text);
      return true;
    }
  }catch(e){/* pastdagi zaxira usulga o'tamiz */}
  // Zaxira: HTTP orqali ochilganda clipboard API ishlamaydi.
  const ta = document.createElement('textarea');
  ta.value = text;
  ta.setAttribute('readonly','');
  ta.style.cssText = 'position:fixed;top:-1000px;opacity:0';
  document.body.appendChild(ta);
  ta.select();
  let ok = false;
  try{ ok = document.execCommand('copy'); }catch(e){ ok = false; }
  ta.remove();
  return ok;
}

// Qisqartirilgan matnni ochish/yopish.
document.addEventListener('click', e => {
  const c = e.target.closest('.clip');
  if(c && !e.target.closest('[data-copy]')) c.classList.toggle('open');
});

document.addEventListener('click', async e => {
  const el = e.target.closest('[data-copy]');
  if(!el) return;
  const ok = await copyText(el.dataset.copy);
  el.classList.add(ok ? 'copied' : 'copyfail');
  setTimeout(()=>el.classList.remove('copied','copyfail'), 900);
});

// Xarajat: kichik summalar ham ko'rinsin (≈$0.0004).
function usd(v){
  const n = Number(v)||0;
  if(n === 0) return '$0';
  return '$' + (n < 0.01 ? n.toFixed(4) : n.toFixed(2));
}
function num(v){ return (Number(v)||0).toLocaleString('uz'); }

// --- Boshqaruv: AI va avto-javobni yoqish/o'chirish ---
// Sozlama bazada saqlanadi, agentni qayta ishga tushirish shart emas.

const SW = {ai_enabled: 'swAI', auto_reply: 'swAuto'};

function paintSettings(st){
  for(const [key, id] of Object.entries(SW)){
    const b = document.getElementById(id);
    if(b){ b.setAttribute('aria-checked', String(!!st[key])); b.disabled = false; }
  }
  document.getElementById('aiHint').textContent = st.ai_enabled
    ? "Yangi xabarlarga javob yozadi"
    : "O'chirilgan — token sarflanmaydi, xabarlar navbatda kutadi";
  document.getElementById('autoHint').textContent = st.auto_reply
    ? "Javoblar mijozga DARHOL yuboriladi"
    : "Javoblar quyida tasdiqlashingizni kutadi";
}

async function loadSettings(){
  try{ paintSettings(await (await fetch('/api/settings')).json()); }
  catch(e){ console.error('sozlamalar:', e); }
}

for(const [key, id] of Object.entries(SW)){
  document.getElementById(id)?.addEventListener('click', async e => {
    const b = e.currentTarget;
    const next = b.getAttribute('aria-checked') !== 'true';
    b.disabled = true;
    try{
      const res = await fetch('/api/settings', {
        method: 'PUT',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({[key]: next}),
      });
      if(!res.ok) throw new Error(await res.text());
      paintSettings(await res.json());
    }catch(err){
      alert('Sozlama saqlanmadi: ' + err.message);
      b.disabled = false;
    }
  });
}

// Javobni tasdiqlash / rad etish.
document.addEventListener('click', async e => {
  const b = e.target.closest('[data-act]');
  if(!b) return;
  const {act, id} = b.dataset;
  if(act === 'reject' && !confirm('Bu javob mijozga yuborilmaydi. Rad etilsinmi?')) return;
  b.disabled = true;
  try{
    const res = await fetch(`/api/interactions/${id}/${act === 'send' ? 'send' : 'reject'}`, {method: 'POST'});
    if(!res.ok) throw new Error(await res.text());
    await load();
  }catch(err){
    alert((act === 'send' ? 'Yuborilmadi: ' : 'Rad etilmadi: ') + err.message);
    b.disabled = false;
  }
});

// --- Xarajat kesimlari: kunlik · mijozlar · muammolar ---
// Token soni provayder javobidan olingan aniq son; xarajat = token × narx.

let costGroup = 'day', costDays = '30';

document.getElementById('costGroup')?.addEventListener('click', e => {
  const b = e.target.closest('.chipbtn');
  if(!b) return;
  costGroup = b.dataset.g;
  document.querySelectorAll('#costGroup .chipbtn').forEach(x => x.classList.toggle('on', x === b));
  loadCosts();
});
document.getElementById('costDays')?.addEventListener('click', e => {
  const b = e.target.closest('.chipbtn');
  if(!b) return;
  costDays = b.dataset.d;
  document.querySelectorAll('#costDays .chipbtn').forEach(x => x.classList.toggle('on', x === b));
  loadCosts();
});

const COST_HEAD = {
  day:          ["Sana","Model","Javoblar","So'rovlar","Kirish (kesh)","Chiqish","Xarajat"],
  client:       ["Mijoz","Suhbatlar","Javoblar","Kirish (kesh)","Chiqish","Jami token","Xarajat","Oxirgi"],
  conversation: ["Suhbat","Mijoz","Holat","Javoblar","Jami token","Xarajat","Oxirgi"],
};

function costRow(r){
  const tokens = (r.prompt_tokens||0) + (r.completion_tokens||0);
  const cached = r.cached_tokens ? ` <span class="msg">(${num(r.cached_tokens)})</span>` : '';
  if(costGroup === 'client'){
    // Mijoz nomi endi saqlanmaydi — faqat id ko'rsatiladi.
    const who = r.client_id ? chip(r.client_id, 'ID ' + r.client_id) : '—';
    return `<tr><td class="w">${who}</td><td>${num(r.conversations)}</td><td>${num(r.replies)}</td>`
         + `<td>${num(r.prompt_tokens)}${cached}</td><td>${num(r.completion_tokens)}</td>`
         + `<td>${num(tokens)}</td><td>${usd(r.cost_usd)}</td><td class="msg">${when(r.last_at)}</td></tr>`;
  }
  if(costGroup === 'conversation'){
    const stt = STATUS[r.status] || STATUS.ai_sent;
    const esc9 = r.escalated ? ' <span title="Xodimlar guruhiga chiqqan">🆘</span>' : '';
    return `<tr><td>${chip(r.conversation_id, '#' + r.conversation_id, 'inline')}</td>`
         + `<td class="w">${r.client_id ? chip(r.client_id, 'ID ' + r.client_id) : '—'}</td>`
         + `<td><span class="tag ${stt.cls}">${stt.label}</span>${esc9}</td>`
         + `<td>${num(r.replies)}</td><td>${num(tokens)}</td><td>${usd(r.cost_usd)}</td>`
         + `<td class="msg">${when(r.last_at)}</td></tr>`;
  }
  return `<tr><td>${esc(r.day)}</td><td class="msg">${esc(r.model)}</td>`
       + `<td>${num(r.replies)}</td><td>${num(r.ai_calls)}</td>`
       + `<td>${num(r.prompt_tokens)}${cached}</td><td>${num(r.completion_tokens)}</td>`
       + `<td>${usd(r.cost_usd)}</td></tr>`;
}

function when(ts){
  if(!ts) return '—';
  const d = new Date(ts);
  return isNaN(d) ? '—' : d.toLocaleString('uz');
}

async function loadCosts(){
  const head = document.getElementById('costHead');
  const body = document.getElementById('costRows');
  try{
    const rows = await (await fetch(`/api/costs?group=${costGroup}&days=${costDays}`)).json() || [];
    const cols = COST_HEAD[costGroup];
    head.innerHTML = `<tr>${cols.map(c=>`<th>${c}</th>`).join('')}</tr>`;

    const sumCost = rows.reduce((a,r)=>a+(r.cost_usd||0), 0);
    const sumTok  = rows.reduce((a,r)=>a+(r.prompt_tokens||0)+(r.completion_tokens||0), 0);
    const label = costDays === 'all' ? 'Butun davr' : `Oxirgi ${costDays} kun`;
    document.getElementById('costTotal').textContent =
      `(${label}: ${usd(sumCost)} · ${num(sumTok)} token)`;

    body.innerHTML = rows.map(costRow).join('')
      || `<tr><td colspan="${cols.length}" class="msg">Bu davrda yozuv yo'q</td></tr>`;
  }catch(err){
    console.error('xarajat:', err);
    body.innerHTML = '<tr><td colspan="8" class="msg">Xarajat ma\'lumotini olib bo\'lmadi</td></tr>';
  }
}

// Status → dashboarddagi belgi. Backend'dagi models.Status* bilan bir xil.
const STATUS = {
  ai_sent:    {label:"AI hal qildi",       cls:"sent"},
  ai_draft:   {label:"AI javobi (yuborilmadi)", cls:"nosent"},
  pending:    {label:"Jarayonda",          cls:"pending"},
  staff_sent: {label:"Xodim hal qildi",    cls:"staff"},
  failed:     {label:"Xatolik",            cls:"failed"},
  rejected:   {label:"Admin rad etdi",     cls:"rejected"},
  unclear:    {label:"AI tushunmadi — ko'rib chiqing", cls:"nosent"},
};

const LEVEL = {yuqori:"🔴 Yuqori", "o'rta":"🟡 O'rta", past:"🟢 Past"};

let filter = 'all';
document.getElementById('filters')?.addEventListener('click', e => {
  const b = e.target.closest('.chipbtn');
  if(!b) return;
  filter = b.dataset.f;
  document.querySelectorAll('.chipbtn').forEach(x => x.classList.toggle('on', x === b));
  render();
});

let history = [];

async function load(){
  loadSettings();
  try{
    const st = await (await fetch('/api/stats')).json();
    document.getElementById('cards').innerHTML = [
      ['Odamlar', st.UniqueClients],
      ['Suhbatlar', st.UniqueChats],
      ['AI hal qildi', st.AIResolved],
      ['Jarayonda', st.Pending],
      ['Xodim hal qildi', st.StaffResolved],
      ['Bugun', usd(st.CostToday)],
      ['Shu oy', usd(st.CostMonth)],
    ].map(([l,n])=>`<div class="card"><div class="n">${n||0}</div><div class="l">${l}</div></div>`).join('');

    // Javob kutayotgan muammolar — eng tepada alohida jadval.
    try{
      const p = await (await fetch('/api/escalations?status=pending')).json() || [];
      document.getElementById('pendingBox').hidden = p.length === 0;
      document.getElementById('pendingCount').textContent = p.length ? `(${p.length} ta)` : '';
      document.getElementById('pendingRows').innerHTML = p.map(e=>{
        const uid = e.client_id ? ' ' + chip(e.client_id, 'ID ' + e.client_id) : '';
        const who = `${esc(e.client_name||'Noma\'lum')}${uid}`
          + `<div class="msg">Suhbat ${chip(e.conversation_id, '#' + e.conversation_id, 'inline')}</div>`;
        const sum = stripMd(e.summary || e.question || '');
        return `<tr><td>${when(e.created_at)}</td><td class="w">${who}</td>`
             + `<td>${esc(LEVEL[e.level] || e.level || '—')}</td>`
             + `<td class="w msg">${clamp(copyNums(esc(sum)), sum, 220)}</td></tr>`;
      }).join('');
    }catch(err){console.error('muammolar:', err)}

    loadCosts();

    history = await (await fetch('/api/history')).json() || [];
    render();
  }catch(e){console.error(e)}
}

function render(){
  const rows = history.filter(r => filter === 'all' || (r.status||'ai_sent') === filter);
  document.getElementById('rows').innerHTML = rows.map(r=>{
    const t = new Date(r.time).toLocaleString('uz');
    const name = esc(r.client_name || 'Noma\'lum');
    // Ism, keyin bir probel tashlab nusxalanadigan "ID 7235".
    const uid = r.client_id ? ' ' + chip(r.client_id, 'ID ' + r.client_id) : '';
    const conv = chip(r.conversation_id, '#' + r.conversation_id, 'inline');
    const who = `${name}${uid}<div class="msg">Suhbat ${conv}</div>`;
    // Holat + kim hal qilgani (AI yoki xodim).
    const stt = STATUS[r.status] || STATUS.ai_sent;
    const by = r.handled_by ? `<div class="msg by">${esc(r.handled_by)}</div>` : '';
    const tag = `<span class="tag ${stt.cls}">${stt.label}</span>${by}`;
    // Agent shu javobga qanday kelgani — bosqichma-bosqich.
    const steps = (r.steps||'').trim();
    const stepsCell = steps
      ? `<details class="steps"><summary>${steps.split('\n').length} qadam</summary>`
        + `<ol>${steps.split('\n').map(l=>`<li>${copyNums(esc(stripMd(l.replace(/^\d+\.\s*/,''))))}</li>`).join('')}</ol></details>`
      : '<span class="msg">—</span>';
    // Rasm tahlil qilinmaydi — faqat "rasm bor" belgisi. Havolasi bo'lsa
    // xodim bosib asl rasmni ko'ra oladi.
    const urls = (r.image_urls||'').split('\n').map(safeURL).filter(Boolean);
    const shotsHtml = r.image_count
      ? `<div class="shots">` + (urls.length
          ? urls.map((u,i)=>`<a class="hasimg" href="${u}" target="_blank" rel="noopener">📷 rasm ${i+1}</a>`).join('')
          : `<span class="hasimg">📷 ${r.image_count} ta rasm</span>`)
        + `</div>`
      : '';

    const cmsg = stripMd(r.client_message), areply = stripMd(r.ai_reply);
    const msgHtml = clamp(copyNums(esc(cmsg)), cmsg, 160);
    const repHtml = clamp(copyNums(esc(areply)), areply, 160);
    // Shu javobning tannarxi (token soni provayderdan olingan aniq son).
    // Tekshirishni kutayotgan javob — tasdiqlash/rad etish tugmalari.
    const actHtml = (r.status === 'ai_draft' && !r.sent)
      ? `<div class="act"><button class="ok-btn" data-act="send" data-id="${r.id}">✓ Yuborish</button>`
        + `<button class="no-btn" data-act="reject" data-id="${r.id}">✕ Rad etish</button></div>`
      : '';
    const tokens = (r.prompt_tokens||0) + (r.completion_tokens||0);
    const costHtml = tokens
      ? `<div class="cost" title="${esc(r.model||'')} — kirish ${num(r.prompt_tokens)} · chiqish ${num(r.completion_tokens)}">`
        + `${num(tokens)} tk · ${usd(r.cost_usd)}</div>`
      : '';

    return `<tr><td>${t}</td><td class="w">${who}</td><td class="w msg">${msgHtml}${shotsHtml}</td>`
         + `<td class="w reply">${repHtml}${costHtml}${actHtml}</td><td class="w">${stepsCell}</td>`
         + `<td>${tag}</td></tr>`;
  }).join('') || '<tr><td colspan="6" class="msg">Bu holatda yozuv yo\'q</td></tr>';
}

load();
setInterval(load, 30000);
