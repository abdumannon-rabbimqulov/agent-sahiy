function esc(s){return (s||'').replace(/[&<>]/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;'}[c]))}
const $ = id => document.getElementById(id);

// Kalit → tushunarli nom va izoh.
const INFO = {
  base: ["Asosiy prompt (1-qadam)",
    "Murojaatni kategoriyaga ajratadi va JSON qaytaradi: dashboard/adminka (buyurtma holati), incorrect_order (muammo), deliver (umumiy savol) yoki category:false. Javob mijozga YUBORILMAYDI — agent unga qarab keyingi promptni tanlaydi. {{DATE}} — bugungi sana."],
  order: ["Buyurtma holati (2-qadam)",
    "1-kategoriya uchun. Tizimdan olingan buyurtma ma'lumoti bilan chaqiriladi. Javob JSON: client — mijozga yoziladigan matn, help — xodimlar guruhiga izoh (kerak bo'lmasa bo'sh)."],
  'cat:xato-mahsulot-kelganda': ["Buyurtmada muammo (2-qadam)",
    "2-kategoriya uchun: yo'qolgan, shikastlangan, noto'g'ri tovar, pul qaytarish. Javob JSON: client va help."],
  'cat:yetkazib-berish': ["Yetkazib berish (2-qadam)",
    "3-kategoriya uchun: shartlar, muddat, punktlar, narx. Javob — oddiy matn, mijozga yuboriladi."],
  'block:order': ["Blok: buyurtma ma'lumoti",
    "Tizimdan olingan buyurtmalar promptga qo'shilganda beriladigan ko'rsatma. {{ORDERS}} — buyurtmalar ro'yxati joyi. Bo'lmasa ma'lumot minimal sarlavha bilan qo'shiladi."],
  'block:category': ["Blok: kategoriya bilimi",
    "Kategoriya bilimlari qo'shilganda beriladigan ko'rsatma. {{CATEGORY}} — bilim matni joyi."],
  'block:image': ["Blok: rasm (buyurtmasiz)",
    "Mijoz rasm yuborgan, lekin tizimdan buyurtma topilmagan holat."],
  'block:image_order': ["Blok: rasm (buyurtma bilan)",
    "Mijoz rasm yuborgan va tizimda buyurtmasi topilgan holat."],
};

// Bazada bo'lishi SHART bo'lgan promptlar — bittasi yo'q bo'lsa agent
// ishga tushmaydi (kodda zaxira matn yo'q).
const REQUIRED = ['base'];
const OPTIONAL = ['block:order', 'block:category', 'block:image', 'block:image_order'];

function info(key){
  if(INFO[key]) return INFO[key];
  if(key.startsWith('cat:')) return ["Kategoriya: " + key.slice(4), "Shu kategoriya tanlanganda ishlatiladigan bilim."];
  return [key, ""];
}

// Taxminiy token soni: 4 belgi ≈ 1 token.
const tok = s => Math.ceil((s||'').length / 4);
const num = v => (Number(v)||0).toLocaleString('uz');

let items = [];

function render(){
  $('list').innerHTML = items.map(p => {
    const [title, hint] = info(p.key);
    const required = REQUIRED.includes(p.key);
    return `
    <form class="box" data-key="${esc(p.key)}">
      <div class="row" style="justify-content:space-between">
        <div>
          <b>${esc(title)}</b> <span class="cp inline">${esc(p.key)}</span>
          <span class="tag ${p.enabled ? 'sent' : 'nosent'}">${p.enabled ? 'yoqilgan' : "o'chiq"}</span>
          <span class="tag cat">v${p.version}</span>
          <div class="msg" style="margin-top:4px">${esc(hint)}</div>
        </div>
        <div class="msg" data-tok>≈ ${num(tok(p.content))} token</div>
      </div>
      <textarea name="content" rows="10">${esc(p.content)}</textarea>
      <div class="row">
        <button type="submit">Saqlash</button>
        <button type="button" class="ghost" data-hist>Tarix</button>
        <button type="button" class="ghost" data-rename>Kalitni o'zgartirish</button>
        ${required ? '' : '<button type="button" class="ghost" data-del>O\'chirish</button>'}
        <label class="row" style="gap:6px;margin-left:auto">
          <input type="checkbox" name="enabled" ${p.enabled ? 'checked' : ''}> yoqilgan
        </label>
      </div>
      <div class="hist" hidden></div>
    </form>`;
  }).join('') || '<div class="msg">Prompt yo\'q — quyidagi shakl orqali qo\'shing.</div>';

  const have = new Set(items.map(p => p.key));
  const miss = REQUIRED.filter(k => !have.has(k));
  const opt  = OPTIONAL.filter(k => !have.has(k));
  let warn = '';
  if(miss.length) warn += `<div class="err" style="display:block">❌ Majburiy promptlar yo'q: <b>${esc(miss.join(', '))}</b> — ularsiz agent ishga tushmaydi.</div>`;
  if(opt.length)  warn += `<div class="msg">ℹ️ Ixtiyoriy promptlar yo'q (blok qo'shilmaydi): ${esc(opt.join(', '))}</div>`;
  $('warn').innerHTML = warn;
}

async function load(){
  try{
    items = await (await fetch('/api/prompts')).json() || [];
    render();
  }catch(e){ $('err').textContent = 'Promptlar yuklanmadi: ' + e.message; }
}

// So'rov yuborish uchun umumiy yordamchi: muvaffaqiyatda ro'yxat yangilanadi,
// xatoda server matni ko'rsatiladi.
async function call(url, opt, failMsg){
  $('err').textContent = '';
  try{
    const res = await fetch(url, opt);
    if(!res.ok) throw new Error((await res.text()).trim());
    await load();
    return true;
  }catch(err){
    $('err').textContent = failMsg + ': ' + err.message;
    return false;
  }
}

const jsonBody = (method, body) => ({
  method, headers: {'Content-Type': 'application/json'}, body: JSON.stringify(body),
});

// Token hisoblagichi yozayotganda yangilanadi.
document.addEventListener('input', e => {
  const ta = e.target.closest('textarea[name=content]');
  if(!ta) return;
  ta.closest('form').querySelector('[data-tok]').textContent = `≈ ${num(tok(ta.value))} token`;
});

// Saqlash (tahrirlash).
document.addEventListener('submit', async e => {
  const form = e.target.closest('form[data-key]');
  if(!form) return;
  e.preventDefault();
  const btn = form.querySelector('button[type=submit]');
  btn.disabled = true;
  await call('/api/prompts/' + encodeURIComponent(form.dataset.key),
    jsonBody('PUT', {content: form.content.value, enabled: form.enabled.checked}), 'Saqlanmadi');
  btn.disabled = false;
});

// Tarix · rollback · kalitni o'zgartirish · o'chirish.
document.addEventListener('click', async e => {
  const form = e.target.closest('form[data-key]');
  if(!form) return;
  const key = form.dataset.key;

  // Tarixni ochish/yopish.
  if(e.target.closest('[data-hist]')){
    const box = form.querySelector('.hist');
    if(!box.hidden){ box.hidden = true; return; }
    try{
      const h = await (await fetch('/api/prompt-history/' + encodeURIComponent(key))).json() || [];
      box.innerHTML = h.length ? h.map(x => `
        <div class="row hist-row">
          <span class="tag cat">v${x.version}</span>
          <span class="msg">${new Date(x.changed_at).toLocaleString('uz')} · ≈${num(tok(x.content))} token</span>
          <button type="button" class="ghost" data-roll="${x.version}">Qaytarish</button>
          <pre class="content">${esc(x.content.slice(0, 400))}${x.content.length > 400 ? '…' : ''}</pre>
        </div>`).join('') : '<div class="msg">Tarix bo\'sh — hali tahrirlanmagan.</div>';
      box.hidden = false;
    }catch(err){ $('err').textContent = 'Tarix olinmadi: ' + err.message; }
    return;
  }

  // Eski versiyaga qaytarish.
  const roll = e.target.closest('[data-roll]');
  if(roll){
    if(!confirm(`v${roll.dataset.roll} ga qaytarilsinmi? Hozirgi matn tarixda saqlanadi.`)) return;
    await call(`/api/prompt-rollback/${encodeURIComponent(key)}?version=${roll.dataset.roll}`,
      {method: 'POST'}, 'Qaytarilmadi');
    return;
  }

  // Kalitni o'zgartirish — tarix yozuvlari ham ko'chadi.
  if(e.target.closest('[data-rename]')){
    const next = prompt(`"${key}" kalitini nimaga o'zgartiramiz?`, key);
    if(next === null) return;
    const val = next.trim();
    if(!val || val === key) return;
    await call('/api/prompt-rename/' + encodeURIComponent(key),
      jsonBody('POST', {key: val}), "Kalit o'zgartirilmadi");
    return;
  }

  // O'chirish — prompt ham, uning butun tarixi ham.
  if(e.target.closest('[data-del]')){
    if(!confirm(`"${key}" prompti va uning butun tarixi o'chiriladi. Davom etamizmi?`)) return;
    await call('/api/prompts/' + encodeURIComponent(key), {method: 'DELETE'}, "O'chirilmadi");
  }
});

// Yangi prompt qo'shish.
$('add').addEventListener('submit', async e => {
  e.preventDefault();
  const key = $('newkey').value.trim();
  const content = $('newcontent').value;
  if(!key || !content.trim()){ $('err').textContent = "Kalit va matn bo'sh bo'lmasligi kerak."; return; }
  if(await call('/api/prompts/' + encodeURIComponent(key), jsonBody('PUT', {content, enabled: true}), "Qo'shilmadi")){
    $('newkey').value = ''; $('newcontent').value = '';
  }
});

load();
