function esc(s){return (s||'').replace(/[&<>]/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;'}[c]))}

async function load(){
  try{
    const st = await (await fetch('/api/stats')).json();
    document.getElementById('cards').innerHTML = [
      ['Odamlar', st.UniqueClients],
      ['Suhbatlar', st.UniqueChats],
      ['Javoblar', st.TotalReplies],
      ['Yuborilgan', st.SentReplies],
    ].map(([l,n])=>`<div class="card"><div class="n">${n||0}</div><div class="l">${l}</div></div>`).join('');

    const h = await (await fetch('/api/history')).json();
    document.getElementById('rows').innerHTML = (h||[]).map(r=>{
      const t = new Date(r.time).toLocaleString('uz');
      const who = esc(r.client_name || ('#'+r.client_id)) + ` <span class="msg">·#${r.conversation_id}</span>`;
      const tag = r.sent ? '<span class="tag sent">yuborildi</span>'
                         : '<span class="tag nosent">ko\'rsatildi</span>';
      const cat = r.category ? `<span class="tag cat">${esc(r.category.name)}</span>` : '';
      return `<tr><td>${t}</td><td class="w">${who}</td><td class="w msg">${esc(r.client_message)}</td>`
           + `<td class="w reply">${esc(r.ai_reply)}</td><td>${cat}</td><td>${tag}</td></tr>`;
    }).join('') || '<tr><td colspan="6" class="msg">Hali yozuv yo\'q</td></tr>';
  }catch(e){console.error(e)}
}

load();
setInterval(load, 30000);
