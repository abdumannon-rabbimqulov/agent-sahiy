function esc(s){return (s||'').replace(/[&<>]/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;'}[c]))}

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

async function load(){
  try{
    const st = await (await fetch('/api/stats')).json();
    document.getElementById('cards').innerHTML = [
      ['Odamlar', st.UniqueClients],
      ['Suhbatlar', st.UniqueChats],
      ['Javoblar', st.TotalReplies],
      ['Yuborilgan', st.SentReplies],
    ].map(([l,n])=>`<div class="card"><div class="n">${n||0}</div><div class="l">${l}</div></div>`).join('');

    // Chat rasmlari — suhbat id bo'yicha guruhlanadi.
    const imgs = {};
    try{
      for(const im of (await (await fetch('/api/images')).json())||[]){
        (imgs[im.conversation_id] ||= []).push(im);
      }
    }catch(e){console.error('rasmlar:', e)}

    const h = await (await fetch('/api/history')).json();
    document.getElementById('rows').innerHTML = (h||[]).map(r=>{
      const t = new Date(r.time).toLocaleString('uz');
      const name = esc(r.client_name || 'Noma\'lum');
      // Ism, keyin bir probel tashlab nusxalanadigan "ID 7235".
      const uid = r.client_id ? ' ' + chip(r.client_id, 'ID ' + r.client_id) : '';
      const conv = chip(r.conversation_id, '#' + r.conversation_id, 'inline');
      const who = `${name}${uid}<div class="msg">Suhbat ${conv}</div>`;
      const tag = r.sent ? '<span class="tag sent">yuborildi</span>'
                         : '<span class="tag nosent">ko\'rsatildi</span>';
      const cat = r.category ? `<span class="tag cat">${esc(r.category.name)}</span>` : '';
      // Agent shu javobga qanday kelgani — bosqichma-bosqich.
      const steps = (r.steps||'').trim();
      const stepsCell = steps
        ? `<details class="steps"><summary>${steps.split('\n').length} qadam</summary>`
          + `<ol>${steps.split('\n').map(l=>`<li>${copyNums(esc(stripMd(l.replace(/^\d+\.\s*/,''))))}</li>`).join('')}</ol></details>`
        : '<span class="msg">—</span>';
      // Shu suhbatdagi rasmlar — kichik ko'rinishda, bosilsa asl hajmda ochiladi.
      const shots = (imgs[r.conversation_id]||[]).slice(0,4).map(im =>
        `<a class="shot" href="/media/${esc(im.file)}" target="_blank" rel="noopener"`
        + ` title="${esc(im.analysis||'')}"><img src="/media/${esc(im.file)}" loading="lazy" alt="rasm"></a>`
      ).join('');
      const shotsHtml = shots ? `<div class="shots">${shots}</div>` : '';

      const cmsg = stripMd(r.client_message), areply = stripMd(r.ai_reply);
      const msgHtml = clamp(copyNums(esc(cmsg)), cmsg, 160);
      const repHtml = clamp(copyNums(esc(areply)), areply, 160);

      return `<tr><td>${t}</td><td class="w">${who}</td><td class="w msg">${msgHtml}${shotsHtml}</td>`
           + `<td class="w reply">${repHtml}</td><td class="w">${stepsCell}</td>`
           + `<td>${cat}</td><td>${tag}</td></tr>`;
    }).join('') || '<tr><td colspan="7" class="msg">Hali yozuv yo\'q</td></tr>';
  }catch(e){console.error(e)}
}

load();
setInterval(load, 30000);
