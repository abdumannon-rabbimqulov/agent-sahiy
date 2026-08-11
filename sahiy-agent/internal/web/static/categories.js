function esc(s){return (s||'').replace(/[&<>]/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;'}[c]))}

const $ = id => document.getElementById(id);
let items = [];

function reset(){
  $('id').value = ''; $('name').value = ''; $('description').value = '';
  $('content').value = ''; $('active').checked = true;
  $('submitBtn').textContent = "Qo'shish";
  $('cancelBtn').hidden = true;
  $('err').textContent = '';
}

function render(){
  $('rows').innerHTML = items.map(c => `
    <tr class="${c.active ? '' : 'off'}">
      <td>${c.id}</td>
      <td class="w"><b>${esc(c.name)}</b><div class="msg">${esc(c.description)}</div></td>
      <td class="w"><pre class="content">${esc(c.content)}</pre></td>
      <td>${c.active ? '<span class="tag sent">aktiv</span>' : '<span class="tag nosent">o\'chiq</span>'}</td>
      <td class="row">
        <button class="ghost" data-edit="${c.id}">Tahrirlash</button>
        <button class="del" data-del="${c.id}">O'chirish</button>
      </td>
    </tr>`).join('') || '<tr><td colspan="5" class="msg">Hali kategoriya yo\'q</td></tr>';
}

async function load(){
  const res = await fetch('/api/categories');
  items = await res.json() || [];
  render();
}

$('form').addEventListener('submit', async e => {
  e.preventDefault();
  $('err').textContent = '';
  const id = $('id').value;
  const body = {
    name: $('name').value.trim(),
    description: $('description').value.trim(),
    content: $('content').value.trim(),
    active: $('active').checked,
  };
  const res = await fetch(id ? `/api/categories/${id}` : '/api/categories', {
    method: id ? 'PUT' : 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify(body),
  });
  if(!res.ok){ $('err').textContent = await res.text(); return; }
  reset();
  load();
});

$('cancelBtn').addEventListener('click', reset);

$('rows').addEventListener('click', async e => {
  const editID = e.target.dataset.edit;
  const delID = e.target.dataset.del;

  if(editID){
    const c = items.find(x => String(x.id) === editID);
    if(!c) return;
    $('id').value = c.id; $('name').value = c.name;
    $('description').value = c.description || ''; $('content').value = c.content;
    $('active').checked = c.active;
    $('submitBtn').textContent = 'Saqlash';
    $('cancelBtn').hidden = false;
    window.scrollTo({top: 0, behavior: 'smooth'});
  }

  if(delID){
    const c = items.find(x => String(x.id) === delID);
    if(!confirm(`"${c ? c.name : delID}" o'chirilsinmi?`)) return;
    const res = await fetch(`/api/categories/${delID}`, {method: 'DELETE'});
    if(!res.ok){ $('err').textContent = await res.text(); return; }
    load();
  }
});

load();
