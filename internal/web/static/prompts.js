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

// Kalitlar ro'yxati Go tomonidan beriladi (/api/prompts-meta) — aks holda
// ai.RequiredKeys o'zgarganda bu fayl jimgina eskirib qolardi.
let META = {required: [], optional: [], placeholders: {}, known: []};

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
    const required = (META.required || []).includes(p.key);
    return `
    <form class="box" data-key="${esc(p.key)}">
      <div class="row" style="justify-content:space-between">
        <div>
          <b>${esc(title)}</b> <span class="cp inline">${esc(p.key)}</span>
          <span class="tag ${p.enabled ? 'sent' : 'nosent'}">${p.enabled ? 'yoqilgan' : "o'chiq"}</span>
          <div class="msg" style="margin-top:4px">${esc(hint)}</div>
        </div>
        <div class="msg" data-tok>≈ ${num(tok(p.content))} token</div>
      </div>
      <textarea name="content" rows="10">${esc(p.content)}</textarea>
      <div class="row">
        <button type="submit">Saqlash</button>
        <button type="button" class="ghost" data-try>Sinab ko'rish</button>
        <button type="button" class="ghost" data-backups>Nusxalar</button>
        <button type="button" class="ghost" data-rename>Kalitni o'zgartirish</button>
        ${required ? '' : '<button type="button" class="ghost" data-del>O\'chirish</button>'}
        <label class="row" style="gap:6px;margin-left:auto">
          <input type="checkbox" name="enabled" ${p.enabled ? 'checked' : ''}> yoqilgan
        </label>
      </div>
      <div data-panel></div>
    </form>`;
  }).join('') || '<div class="msg">Prompt yo\'q — quyidagi shakl orqali qo\'shing.</div>';

  const have = new Set(items.map(p => p.key));
  const miss = (META.required || []).filter(k => !have.has(k));
  const opt  = (META.optional || []).filter(k => !have.has(k));
  let warn = '';
  if(miss.length) warn += `<div class="err" style="display:block">❌ Majburiy promptlar yo'q: <b>${esc(miss.join(', '))}</b> — ularsiz agent ishga tushmaydi.</div>`;
  if(opt.length)  warn += `<div class="msg">ℹ️ Ixtiyoriy promptlar yo'q (blok qo'shilmaydi): ${esc(opt.join(', '))}</div>`;
  $('warn').innerHTML = warn;
}

async function load(){
  try{
    const [list, meta] = await Promise.all([
      fetch('/api/prompts').then(r => r.json()),
      fetch('/api/prompts-meta').then(r => r.json()).catch(() => META),
    ]);
    items = list || [];
    META = meta || META;
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
    const body = res.status === 204 ? {} : await res.json().catch(() => ({}));
    await load();
    showWarnings(body.key, body.warnings);
    return true;
  }catch(err){
    $('err').textContent = failMsg + ': ' + err.message;
    return false;
  }
}

// Saqlash to'xtamagan, lekin e'tibor berish kerak bo'lgan holatlar
// (masalan block:order da {{ORDERS}} yozilmagan).
function showWarnings(key, warns){
  if(!key || !warns || !warns.length) return;
  const form = document.querySelector(`form[data-key="${CSS.escape(key)}"]`);
  if(!form) return;
  panel(form).innerHTML = `<div class="msg" style="margin-top:8px">⚠️ ${
    warns.map(esc).join('<br>')}</div>`;
}

// panel — formadagi natija/ogohlantirish maydoni.
const panel = form => form.querySelector('[data-panel]');

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

// Kalitni o'zgartirish · o'chirish.
document.addEventListener('click', async e => {
  const form = e.target.closest('form[data-key]');
  if(!form) return;
  const key = form.dataset.key;

  // Kalitni o'zgartirish — yangi kalit PUT tanasida yuboriladi.
  if(e.target.closest('[data-rename]')){
    const next = prompt(`"${key}" kalitini nimaga o'zgartiramiz?`, key);
    if(next === null) return;
    const val = next.trim();
    if(!val || val === key) return;
    await call('/api/prompts/' + encodeURIComponent(key),
      jsonBody('PUT', {key: val}), "Kalit o'zgartirilmadi");
    return;
  }

  // Sinab ko'rish — saqlanmagan matnni haqiqiy model orqali o'tkazadi.
  if(e.target.closest('[data-try]')){ openTry(form); return; }

  // Nusxalar — oxirgi saqlangan matnlar va tiklash.
  if(e.target.closest('[data-backups]')){ await openBackups(form, key); return; }

  // O'chirish.
  if(e.target.closest('[data-del]')){
    if(!confirm(`"${key}" prompti o'chiriladi. Davom etamizmi?`)) return;
    await call('/api/prompts/' + encodeURIComponent(key), {method: 'DELETE'}, "O'chirilmadi");
  }
});

// --- Sinab ko'rish ---

// openTry — sinov murojaati uchun maydon ochadi (yoki yopadi).
function openTry(form){
  const box = panel(form);
  if(box.querySelector('[data-tryrun]')){ box.innerHTML = ''; return; }
  box.innerHTML = `
    <div class="msg" style="margin-top:8px">
      Sinov murojaati — mijoz yozgan matn. Natija bazaga saqlanmaydi.
    </div>
    <textarea data-transcript rows="3" placeholder="Mijoz: buyurtmam qachon keladi? SN12345"></textarea>
    <textarea data-orderinfo rows="2" placeholder="(ixtiyoriy) tizimdan olingan buyurtma ma'lumoti — JSON"></textarea>
    <div class="row"><button type="button" data-tryrun>Yuborish</button></div>
    <div data-tryout></div>`;
}

// runTry — /api/prompt-try ga so'rov.
async function runTry(form, key){
  const box = panel(form);
  const out = box.querySelector('[data-tryout]');
  const btn = box.querySelector('[data-tryrun]');
  const transcript = box.querySelector('[data-transcript]').value.trim();
  if(!transcript){ out.innerHTML = '<div class="msg">Sinov murojaatini yozing.</div>'; return; }

  btn.disabled = true;
  out.innerHTML = '<div class="msg">⏳ Model javob yozmoqda (lokal model sekin)...</div>';
  try{
    const res = await fetch('/api/prompt-try/' + encodeURIComponent(key),
      jsonBody('POST', {
        content: form.content.value,
        transcript,
        order_info: box.querySelector('[data-orderinfo]').value.trim(),
      }));
    if(!res.ok) throw new Error((await res.text()).trim());
    out.innerHTML = tryResult(await res.json());
  }catch(err){
    out.innerHTML = `<div class="err" style="display:block">Sinov o'tmadi: ${esc(err.message)}</div>`;
  }finally{ btn.disabled = false; }
}

// tryResult — natijani ko'rsatish: xom javob, o'qilgani va sarflangan token.
function tryResult(r){
  const t = r.tokens || {};
  const parsed = r.parsed ? `<b>O'qilgani</b><pre>${esc(JSON.stringify(r.parsed, null, 2))}</pre>` : '';
  const bad = r.parse_error ? `<div class="err" style="display:block">${esc(r.parse_error)}</div>` : '';
  const warn = (r.warnings || []).length
    ? `<div class="msg">⚠️ ${r.warnings.map(esc).join('<br>')}</div>` : '';
  return `
    <div class="msg" style="margin-top:8px">
      Yo'l: <b>${esc(r.path || '')}</b>${r.kind ? ' · ' + esc(r.kind) : ''} ·
      ${num(t.prompt_tokens)} + ${num(t.completion_tokens)} token · ${num(t.duration_ms)} ms
    </div>
    ${bad}${warn}
    <b>Model javobi</b><pre>${esc(r.raw || '')}</pre>
    ${parsed}
    <details><summary class="msg">Modelga ketgan system prompt</summary><pre>${esc(r.system || '')}</pre></details>`;
}

// --- Nusxalar (tahrirni qaytarish) ---

async function openBackups(form, key){
  const box = panel(form);
  if(box.querySelector('[data-backuplist]')){ box.innerHTML = ''; return; }
  box.innerHTML = '<div class="msg" data-backuplist>Yuklanmoqda...</div>';
  try{
    const list = await (await fetch('/api/prompt-backups/' + encodeURIComponent(key))).json();
    box.innerHTML = list.length ? `
      <div data-backuplist style="margin-top:8px">
        <div class="msg">Tahrirdan oldingi matnlar (oxirgi ${list.length} ta):</div>
        ${list.map(b => `
          <div class="row" style="align-items:flex-start;gap:8px;margin-top:6px">
            <button type="button" class="ghost" data-restore="${b.id}">Tiklash</button>
            <div class="msg">${esc(new Date(b.saved_at).toLocaleString('uz'))} ·
              ${num(tok(b.content))} token</div>
          </div>
          <pre>${esc(b.content.slice(0, 400))}${b.content.length > 400 ? '…' : ''}</pre>`).join('')}
      </div>` : '<div class="msg" data-backuplist>Hali nusxa yo\'q — birinchi tahrirdan keyin paydo bo\'ladi.</div>';
  }catch(err){
    box.innerHTML = `<div class="err" style="display:block">Nusxalar olinmadi: ${esc(err.message)}</div>`;
  }
}

// Panel ichidagi tugmalar (sinov yuborish · tiklash).
document.addEventListener('click', async e => {
  const form = e.target.closest('form[data-key]');
  if(!form) return;
  if(e.target.closest('[data-tryrun]')){ await runTry(form, form.dataset.key); return; }
  const restore = e.target.closest('[data-restore]');
  if(restore){
    if(!confirm('Shu nusxa tiklanadi. Joriy matn ham nusxaga tushadi. Davom etamizmi?')) return;
    await call('/api/prompt-restore/' + encodeURIComponent(form.dataset.key),
      jsonBody('POST', {id: Number(restore.dataset.restore)}), 'Tiklanmadi');
  }
});

// Yangi prompt qo'shish.
$('add').addEventListener('submit', async e => {
  e.preventDefault();
  const key = $('newkey').value.trim();
  const content = $('newcontent').value;
  if(!key || !content.trim()){ $('err').textContent = "Kalit va matn bo'sh bo'lmasligi kerak."; return; }
  if(await call('/api/prompts', jsonBody('POST', {key, content, enabled: true}), "Qo'shilmadi")){
    $('newkey').value = ''; $('newcontent').value = '';
  }
});

load();
