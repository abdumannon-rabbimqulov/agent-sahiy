function esc(s){return (s||'').replace(/[&<>]/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;'}[c]))}
const $ = id => document.getElementById(id);

// Kalit → tushunarli nom va izoh.
const INFO = {
  base:      ["Asosiy prompt", "Agentning xarakteri va qat'iy qoidalari. Faqat mijozga javob yozishda ishlatiladi. {{DATE}} — bugungi sana."],
  classify:  ["Router (birinchi prompt)", "Murojaatni kategoriyaga ajratadi. {{CATEGORIES}} o'rniga ro'yxat avtomatik qo'yiladi. Javob JSON: {\"category\":\"...\",\"escalate\":false,\"order\":false} — order:true bo'lsa agent Dashboard'ga GET so'rov yuborib buyurtmani tekshiradi."],
  summarize: ["Xulosa", "Xodimlar guruhiga yuboriladigan qisqa xulosa."],
  'block:order':       ["Blok: buyurtma ma'lumoti", "Tizimdan olingan buyurtmalar javobga qo'shilganda beriladigan ko'rsatma. {{ORDERS}} — buyurtmalar ro'yxati joyi."],
  'block:category':    ["Blok: kategoriya bilimi", "Kategoriya bilimlari qo'shilganda beriladigan ko'rsatma. {{CATEGORY}} — bilim matni joyi."],
  'block:image':       ["Blok: rasm (buyurtmasiz)", "Mijoz rasm yuborgan, lekin tizimdan buyurtma topilmagan holat."],
  'block:image_order': ["Blok: rasm (buyurtma bilan)", "Mijoz rasm yuborgan va tizimda buyurtmasi topilgan holat."],
};

// Bazada bo'lishi SHART bo'lgan promptlar — bittasi yo'q bo'lsa agent
// ishga tushmaydi (kodda zaxira matn yo'q).
const REQUIRED = ['base', 'classify', 'summarize'];
const OPTIONAL = ['block:order', 'block:category', 'block:image', 'block:image_order'];

function info(key){
  if(INFO[key]) return INFO[key];
  if(key.startsWith('cat:')) return ["Kategoriya: " + key.slice(4), "Shu kategoriya tanlanganda javobga qo'shiladigan bilim."];
  return [key, ""];
}

// Taxminiy token soni: 4 belgi ≈ 1 token.
const tok = s => Math.ceil((s||'').length / 4);
const num = v => (Number(v)||0).toLocaleString('uz');

let items = [];

function render(){
  $('list').innerHTML = items.map(p => {
    const [title, hint] = info(p.key);
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

// Token hisoblagichi yozayotganda yangilanadi.
document.addEventListener('input', e => {
  const ta = e.target.closest('textarea[name=content]');
  if(!ta) return;
  ta.closest('form').querySelector('[data-tok]').textContent = `≈ ${num(tok(ta.value))} token`;
});

// Saqlash.
document.addEventListener('submit', async e => {
  const form = e.target.closest('form[data-key]');
  if(!form) return;
  e.preventDefault();
  $('err').textContent = '';
  const btn = form.querySelector('button[type=submit]');
  btn.disabled = true;
  try{
    const res = await fetch('/api/prompts/' + encodeURIComponent(form.dataset.key), {
      method: 'PUT',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({
        content: form.content.value,
        enabled: form.enabled.checked,
      }),
    });
    if(!res.ok) throw new Error(await res.text());
    await // Yangi prompt qo'shish.
$('add').addEventListener('submit', async e => {
  e.preventDefault();
  $('err').textContent = '';
  const key = $('newkey').value.trim();
  const content = $('newcontent').value;
  if(!key || !content.trim()){ $('err').textContent = 'Kalit va matn bo\'sh bo\'lmasligi kerak.'; return; }
  try{
    const res = await fetch('/api/prompts/' + encodeURIComponent(key), {
      method: 'PUT',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({content, enabled: true}),
    });
    if(!res.ok) throw new Error(await res.text());
    $('newkey').value = ''; $('newcontent').value = '';
    await load();
  }catch(err){ $('err').textContent = 'Qo\'shilmadi: ' + err.message; }
});

load();
  }catch(err){ $('err').textContent = 'Saqlanmadi: ' + err.message; }
  finally{ btn.disabled = false; }
});

// Tarix va rollback.
document.addEventListener('click', async e => {
  const form = e.target.closest('form[data-key]');
  if(!form) return;
  const key = form.dataset.key;

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

  const roll = e.target.closest('[data-roll]');
  if(roll){
    if(!confirm(`v${roll.dataset.roll} ga qaytarilsinmi? Hozirgi matn tarixda saqlanadi.`)) return;
    try{
      const res = await fetch(`/api/prompt-rollback/${encodeURIComponent(key)}?version=${roll.dataset.roll}`, {method:'POST'});
      if(!res.ok) throw new Error(await res.text());
      await // Yangi prompt qo'shish.
$('add').addEventListener('submit', async e => {
  e.preventDefault();
  $('err').textContent = '';
  const key = $('newkey').value.trim();
  const content = $('newcontent').value;
  if(!key || !content.trim()){ $('err').textContent = 'Kalit va matn bo\'sh bo\'lmasligi kerak.'; return; }
  try{
    const res = await fetch('/api/prompts/' + encodeURIComponent(key), {
      method: 'PUT',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({content, enabled: true}),
    });
    if(!res.ok) throw new Error(await res.text());
    $('newkey').value = ''; $('newcontent').value = '';
    await load();
  }catch(err){ $('err').textContent = 'Qo\'shilmadi: ' + err.message; }
});

load();
    }catch(err){ $('err').textContent = 'Qaytarilmadi: ' + err.message; }
  }
});

// Yangi prompt qo'shish.
$('add').addEventListener('submit', async e => {
  e.preventDefault();
  $('err').textContent = '';
  const key = $('newkey').value.trim();
  const content = $('newcontent').value;
  if(!key || !content.trim()){ $('err').textContent = 'Kalit va matn bo\'sh bo\'lmasligi kerak.'; return; }
  try{
    const res = await fetch('/api/prompts/' + encodeURIComponent(key), {
      method: 'PUT',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({content, enabled: true}),
    });
    if(!res.ok) throw new Error(await res.text());
    $('newkey').value = ''; $('newcontent').value = '';
    await load();
  }catch(err){ $('err').textContent = 'Qo\'shilmadi: ' + err.message; }
});

load();
