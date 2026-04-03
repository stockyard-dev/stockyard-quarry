package server

import "net/http"

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(dashHTML))
}

const dashHTML = `<!DOCTYPE html>
<html lang="en"><head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Quarry</title>
<style>
:root{--bg:#1a1410;--bg2:#241e18;--bg3:#2e261e;--rust:#c45d2c;--rl:#e8753a;--leather:#a0845c;--ll:#c4a87a;--cream:#f0e6d3;--cd:#bfb5a3;--cm:#7a7060;--gold:#d4a843;--green:#4a9e5c;--red:#c44040;--blue:#4a7ec4;--mono:'JetBrains Mono',Consolas,monospace;--serif:'Libre Baskerville',Georgia,serif}
*{margin:0;padding:0;box-sizing:border-box}body{background:var(--bg);color:var(--cream);font-family:var(--mono);font-size:13px;line-height:1.6}
a{color:var(--rl);text-decoration:none}a:hover{color:var(--gold)}
.hdr{padding:.6rem 1.2rem;border-bottom:1px solid var(--bg3);display:flex;justify-content:space-between;align-items:center}
.hdr h1{font-family:var(--serif);font-size:1rem}.hdr h1 span{color:var(--rl)}
.hdr-right{display:flex;gap:1rem;align-items:center;font-size:.7rem;color:var(--leather)}.hdr-right b{color:var(--cream)}
.main{max-width:1100px;margin:0 auto;padding:1rem 1.2rem}
.overview{display:flex;gap:1.5rem;margin-bottom:1rem;font-size:.7rem;color:var(--leather);flex-wrap:wrap}
.overview .stat b{display:block;font-size:1.2rem;color:var(--cream)}
.toolbar{display:flex;gap:.5rem;margin-bottom:.8rem;flex-wrap:wrap;align-items:center}
.toolbar select,.toolbar input{background:var(--bg);border:1px solid var(--bg3);color:var(--cream);padding:.3rem .5rem;font-family:var(--mono);font-size:.72rem;outline:none}
.toolbar input{flex:1;min-width:150px}
.toolbar select:focus,.toolbar input:focus{border-color:var(--rust)}
.btn{font-family:var(--mono);font-size:.68rem;padding:.3rem .6rem;border:1px solid;cursor:pointer;background:transparent;transition:.15s;white-space:nowrap}
.btn-p{border-color:var(--rust);color:var(--rl)}.btn-p:hover{background:var(--rust);color:var(--cream)}
.btn-d{border-color:var(--bg3);color:var(--cm)}.btn-d:hover{border-color:var(--red);color:var(--red)}

.log-line{display:flex;gap:.5rem;padding:.2rem .5rem;font-size:.72rem;border-bottom:1px solid var(--bg3);font-family:var(--mono)}
.log-line:hover{background:var(--bg2)}
.log-ts{color:var(--cm);white-space:nowrap;flex-shrink:0;width:75px;font-size:.65rem}
.log-level{width:45px;flex-shrink:0;font-weight:600;text-transform:uppercase;font-size:.6rem;padding-top:2px}
.log-level.debug{color:var(--cm)}.log-level.info{color:var(--blue)}.log-level.warn{color:var(--gold)}.log-level.error{color:var(--red)}.log-level.fatal{color:var(--red);background:rgba(196,64,64,.15)}
.log-source{color:var(--leather);width:80px;flex-shrink:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-size:.68rem}
.log-msg{color:var(--cd);flex:1;word-break:break-all}
.log-meta{color:var(--cm);font-size:.6rem;cursor:pointer}

.empty{text-align:center;padding:2rem;color:var(--cm);font-style:italic;font-family:var(--serif)}
.saved-item{display:inline-flex;align-items:center;gap:.3rem;padding:.15rem .5rem;background:var(--bg2);border:1px solid var(--bg3);font-size:.68rem;cursor:pointer;margin-right:.3rem;margin-bottom:.3rem}
.saved-item:hover{border-color:var(--rust)}

.modal-bg{position:fixed;top:0;left:0;right:0;bottom:0;background:rgba(0,0,0,.65);display:flex;align-items:center;justify-content:center;z-index:100}
.modal{background:var(--bg2);border:1px solid var(--bg3);padding:1.5rem;width:90%;max-width:400px}
.modal h2{font-family:var(--serif);font-size:.9rem;margin-bottom:.8rem}
label.fl{display:block;font-size:.65rem;color:var(--leather);text-transform:uppercase;letter-spacing:1px;margin-bottom:.2rem;margin-top:.5rem}
input[type=text]{background:var(--bg);border:1px solid var(--bg3);color:var(--cream);padding:.35rem .5rem;font-family:var(--mono);font-size:.78rem;width:100%;outline:none}
</style>
<link href="https://fonts.googleapis.com/css2?family=Libre+Baskerville:ital@0;1&family=JetBrains+Mono:wght@400;600&display=swap" rel="stylesheet">
</head><body>
<div class="hdr">
<h1><span>Quarry</span></h1>
<div class="hdr-right">
<span>Logs: <b id="sTotal">-</b></span>
<span>24h: <b id="s24h">-</b></span>
<label style="font-size:.65rem;display:flex;align-items:center;gap:.3rem"><input type="checkbox" id="autoRefresh" checked onchange="toggleAuto()"> Live</label>
</div>
</div>
<div class="main"><div id="upgrade-banner" style="display:none;background:#241e18;border:1px solid #8b3d1a;border-left:3px solid #c45d2c;padding:.6rem 1rem;font-size:.78rem;color:#bfb5a3;margin-bottom:.8rem"><strong style="color:#f0e6d3">Free tier</strong> — 10 items max. <a href="https://stockyard.dev/quarry/" target="_blank" style="color:#e8753a">Upgrade to Pro →</a></div>
<div class="overview" id="overview"></div>
<div id="savedBar" style="margin-bottom:.5rem"></div>
<div class="toolbar">
<select id="fSource"><option value="">Source</option></select>
<select id="fLevel"><option value="">Level</option><option>debug</option><option>info</option><option>warn</option><option>error</option><option>fatal</option></select>
<input type="text" id="fSearch" placeholder="Search logs..." onkeydown="if(event.key==='Enter')loadLogs()">
<button class="btn btn-p" onclick="loadLogs()">Search</button>
<button class="btn btn-d" onclick="saveSearch()">Save</button>
<button class="btn btn-d" onclick="clearFilters()">Clear</button>
</div>
<div id="logStream" style="max-height:calc(100vh - 250px);overflow-y:auto"></div>
</div>
<div id="modal"></div>

<script>
let logs=[],sources=[],autoTimer=null;

async function api(url,opts){const r=await fetch(url,opts);return r.json()}
function esc(s){return String(s||'').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;')}

async function init(){
  const [sd,src,ss]=await Promise.all([api('/api/stats'),api('/api/sources'),api('/api/searches')]);
  document.getElementById('sTotal').textContent=sd.total_logs.toLocaleString();
  document.getElementById('s24h').textContent=sd.last_24h.toLocaleString();
  sources=src.sources||[];
  const sel=document.getElementById('fSource');
  sel.innerHTML='<option value="">Source ('+sources.length+')</option>'+sources.map(s=>'<option>'+esc(s.name)+'</option>').join('');
  const levels=sd.by_level||{};
  const ov=document.getElementById('overview');
  ov.innerHTML=Object.entries(levels).map(([l,c])=>'<div class="stat"><b>'+c+'</b>'+l+'</div>').join('');
  const saved=ss.searches||[];
  document.getElementById('savedBar').innerHTML=saved.map(s=>'<span class="saved-item" onclick="applySaved(\''+esc(s.query)+'\')">'+esc(s.name)+' <span style="color:var(--cm);cursor:pointer" onclick="event.stopPropagation();delSaved(\''+s.id+'\')">&times;</span></span>').join('');
  loadLogs();
  if(document.getElementById('autoRefresh').checked)startAuto();
}

async function loadLogs(){
  const p=new URLSearchParams();
  const src=document.getElementById('fSource').value;if(src)p.set('source',src);
  const lvl=document.getElementById('fLevel').value;if(lvl)p.set('level',lvl);
  const q=document.getElementById('fSearch').value;if(q)p.set('search',q);
  p.set('limit','200');
  const d=await api('/api/logs?'+p);
  logs=d.logs||[];
  renderLogs();
}

function renderLogs(){
  const el=document.getElementById('logStream');
  if(!logs.length){el.innerHTML='<div class="empty">No logs found. Send logs via POST /api/ingest</div>';return}
  el.innerHTML=logs.map(l=>{
    const ts=new Date(l.timestamp);
    const time=ts.toLocaleTimeString();
    const meta=l.meta&&Object.keys(l.meta).length?'<span class="log-meta" title="'+esc(JSON.stringify(l.meta))+'">meta</span>':'';
    return '<div class="log-line">'+
      '<span class="log-ts">'+time+'</span>'+
      '<span class="log-level '+l.level+'">'+l.level+'</span>'+
      '<span class="log-source">'+esc(l.source)+'</span>'+
      '<span class="log-msg">'+esc(l.message)+'</span>'+
      meta+'</div>'
  }).join('')
}

function clearFilters(){
  document.getElementById('fSource').value='';
  document.getElementById('fLevel').value='';
  document.getElementById('fSearch').value='';
  loadLogs();
}

function applySaved(q){
  document.getElementById('fSearch').value=q;
  loadLogs();
}

async function saveSearch(){
  const q=document.getElementById('fSearch').value;
  if(!q){alert('Enter a search query first');return}
  const name=prompt('Name this search:');
  if(!name)return;
  await api('/api/searches',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({name,query:q})});
  init();
}

async function delSaved(id){
  await api('/api/searches/'+id,{method:'DELETE'});
  init();
}

function toggleAuto(){
  if(document.getElementById('autoRefresh').checked){startAuto()}else{stopAuto()}
}
function startAuto(){stopAuto();autoTimer=setInterval(()=>{loadLogs()},5000)}
function stopAuto(){if(autoTimer){clearInterval(autoTimer);autoTimer=null}}

init();
fetch('/api/tier').then(r=>r.json()).then(j=>{if(j.tier==='free'){var b=document.getElementById('upgrade-banner');if(b)b.style.display='block'}}).catch(()=>{var b=document.getElementById('upgrade-banner');if(b)b.style.display='block'});
</script></body></html>`
