package latex

// T154：复查页的样式与灯箱脚本（单文件、无外链，file:// 与 http 都能用）。
// 分组的展开/收起用原生 <details>，默认收起；灯箱与 Web 会话预览页同一套手势：
// 滚轮 / 双指缩放都锚在指针位置、拖动平移、点图片或空白处关闭、Esc 关闭。

const reviewStyle = `:root{color-scheme:light dark;--bg:#f7f6f3;--card:#fff;--line:#e4e0d8;--fg:#232323;--dim:#6b6b6b;--chip:#efece5}
*{box-sizing:border-box}
body{margin:0;padding:18px 20px 64px;font:14px/1.55 -apple-system,"Segoe UI",system-ui,"Noto Sans CJK SC","PingFang SC",sans-serif;background:var(--bg);color:var(--fg)}
h1{font-size:18px;margin:0 0 6px}
h2{font-size:15px;margin:0}
.intro{margin:0 0 10px;color:var(--dim);font-size:12.5px}
ul.meta{margin:0 0 4px;padding:0;list-style:none;color:var(--dim);font-size:12.5px}
ul.meta li{margin:2px 0}
code{font:12px/1.4 ui-monospace,Menlo,Consolas,monospace;background:var(--chip);padding:1px 4px;border-radius:3px}
.bar{display:flex;align-items:center;gap:8px;margin:22px 0 10px;padding-bottom:6px;border-bottom:1px solid var(--line)}
.bar .spacer{flex:1}
.gb{font:12px/1 inherit;padding:4px 9px;color:var(--dim);background:var(--card);border:1px solid var(--line);border-radius:999px;cursor:pointer}
.gb:hover{color:var(--fg);border-color:#c9c3b7}
details.group{background:var(--card);border:1px solid var(--line);border-radius:7px;margin:7px 0}
summary.ghead{display:flex;align-items:center;gap:8px;padding:7px 10px;cursor:pointer;list-style:none;font-size:12.5px;color:var(--dim);border-radius:7px}
summary.ghead::-webkit-details-marker{display:none}
summary.ghead::before{content:"▸";flex:none;color:#a09a8d}
details[open]>summary.ghead::before{content:"▾"}
summary.ghead:hover{background:#faf8f4}
summary.ghead .no{font-weight:700;color:var(--fg)}
summary.ghead .cnt{color:var(--fg);font-variant-numeric:tabular-nums}
summary.ghead .name{flex:1;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-family:ui-monospace,Menlo,Consolas,monospace;font-size:11.5px;color:#999}
summary.ghead .min{color:#8a6d3b;font-variant-numeric:tabular-nums}
.chip{flex:none;padding:1px 6px;border-radius:999px;background:var(--chip);color:var(--dim);font-size:11px}
.chip.src{background:#e8f0fb;color:#2c5c96}
.gbody{padding:10px 12px 12px;border-top:1px solid var(--line)}
.key{display:flex;gap:12px;align-items:flex-start;margin:2px 0 12px}
.key figure{width:180px;flex:none}
.key figcaption{color:var(--dim)}
.keystats{min-width:0;padding-top:2px}
.kname{font-family:ui-monospace,Menlo,Consolas,monospace;font-size:11.5px;word-break:break-all}
.sub{color:var(--dim);font-size:12px;margin-top:2px}
.members,.indep{display:flex;flex-wrap:wrap;gap:10px}
figure{margin:0;width:104px;text-align:center}
figure img{display:block;width:100%;height:auto;background:#fff;border:1px solid #eee;border-radius:4px}
figcaption{font-size:11px;color:#555;margin-top:3px}
figcaption .nm{display:block;word-break:break-all}
figcaption .nt{display:block;word-break:normal;overflow-wrap:anywhere;color:var(--dim)}
a{color:#0b6bcb;text-decoration:none}
a.shot{display:block;cursor:zoom-in}
.empty{color:var(--dim);font-size:12.5px}
.lb{position:fixed;inset:0;z-index:50;background:rgba(14,15,18,.93);display:flex;align-items:center;justify-content:center;overflow:hidden}
.lb[hidden]{display:none}
.lb img{max-width:92vw;max-height:88vh;transform-origin:50% 50%;touch-action:none;cursor:grab;user-select:none;-webkit-user-drag:none}
.lb img:active{cursor:grabbing}
.lb-bar{position:fixed;left:50%;bottom:14px;transform:translateX(-50%);display:flex;align-items:center;gap:6px;padding:5px 7px;border-radius:999px;background:rgba(32,34,40,.9);color:#eee;font-size:12px;z-index:51;max-width:96vw}
.lb-bar button{font:12px/1 inherit;min-width:26px;padding:5px 7px;border:0;border-radius:999px;background:rgba(255,255,255,.12);color:#eee;cursor:pointer}
.lb-bar button:hover{background:rgba(255,255,255,.22)}
.lb-bar .pct{min-width:44px;text-align:center;font-variant-numeric:tabular-nums}
.lb-bar .hint{min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:#aaa;padding:0 4px}
body.lb-open{overflow:hidden}
@media (max-width:640px){figure{width:84px}.key figure{width:132px}.lb-bar .hint{display:none}}
@media (prefers-color-scheme:dark){
:root{--bg:#16181c;--card:#1e2126;--line:#2c3036;--fg:#ddd;--dim:#9a9a9a;--chip:#272b31}
figure img{background:#111;border-color:#2c3036}
a{color:#63a9f0}
summary.ghead:hover{background:#22262c}
summary.ghead .name{color:#777}
.chip.src{background:#22334a;color:#9dc3ef}
}`

const reviewLightboxHTML = `<div class="lb" id="lb" hidden>
<img id="lb-img" alt="">
<div class="lb-bar">
<button type="button" data-lb="out" title="缩小">−</button>
<span class="pct" id="lb-pct">100%</span>
<button type="button" data-lb="in" title="放大">+</button>
<button type="button" data-lb="reset" title="复位">复位</button>
<span class="hint">滚轮 / 双指缩放 · 拖动平移 · 点图片或 Esc 关闭</span>
<button type="button" data-lb="close" title="关闭">✕</button>
</div>
</div>
`

const reviewScript = `(function(){
var lb=document.getElementById('lb'),im=document.getElementById('lb-img'),pct=document.getElementById('lb-pct');
if(!lb||!im)return;
var MIN=0.1,MAX=8,scale=1,tx=0,ty=0,moved=false,drag=null,pinch=null,ids=Object.create(null);
function apply(){im.style.transform='translate('+tx+'px,'+ty+'px) scale('+scale+')';if(pct)pct.textContent=Math.round(scale*100)+'%';}
function reset(){scale=1;tx=0;ty=0;apply();}
function zoomAt(f,cx,cy){
  var s=scale*f;if(s<MIN)s=MIN;if(s>MAX)s=MAX;if(Math.abs(s-scale)<1e-4)return;
  var r=im.getBoundingClientRect(),ox=cx-(r.left+r.width/2),oy=cy-(r.top+r.height/2),k=s/scale;
  tx=ox+k*(tx-ox);ty=oy+k*(ty-oy);scale=s;apply();
}
function count(){var n=0;for(var k in ids)n++;return n;}
function pair(){var a=[];for(var k in ids)a.push(ids[k]);return a.length<2?null:a;}
function mid(p){return{x:(p[0].x+p[1].x)/2,y:(p[0].y+p[1].y)/2};}
function dist(p){var dx=p[0].x-p[1].x,dy=p[0].y-p[1].y;return Math.sqrt(dx*dx+dy*dy)||1;}
function open(a){
  var full=a.getAttribute('data-full')||a.getAttribute('href');
  if(!full)return;
  var thumb=a.getAttribute('data-thumb');
  im.onerror=function(){if(thumb&&im.getAttribute('src')!==thumb){im.onerror=null;im.src=thumb;}};
  im.src=full;im.alt=a.getAttribute('title')||'';
  lb.hidden=false;document.body.classList.add('lb-open');reset();
}
function close(){
  lb.hidden=true;document.body.classList.remove('lb-open');
  im.onerror=null;im.removeAttribute('src');drag=null;pinch=null;ids=Object.create(null);
}
document.addEventListener('click',function(e){
  var t=e.target;
  if(t&&t.closest){
    var a=t.closest('a.shot');
    if(a){e.preventDefault();open(a);return;}
    var b=t.closest('[data-lb]');
    if(b&&!lb.hidden){
      e.preventDefault();var k=b.getAttribute('data-lb');
      if(k==='in')zoomAt(1/0.8,window.innerWidth/2,window.innerHeight/2);
      else if(k==='out')zoomAt(0.8,window.innerWidth/2,window.innerHeight/2);
      else if(k==='reset')reset();
      else close();
      return;
    }
    var all=t.closest('[data-all]');
    if(all){
      e.preventDefault();
      var want=all.getAttribute('data-all')==='open',gs=document.querySelectorAll('details.group');
      for(var i=0;i<gs.length;i++)gs[i].open=want;
      return;
    }
  }
  if(t===lb)close();
});
im.addEventListener('click',function(){if(!moved)close();moved=false;});
lb.addEventListener('wheel',function(e){
  if(lb.hidden)return;
  e.preventDefault();zoomAt(e.deltaY>0?0.9:1/0.9,e.clientX,e.clientY);
},{passive:false});
im.addEventListener('pointerdown',function(e){
  if(im.setPointerCapture){try{im.setPointerCapture(e.pointerId);}catch(_){}}
  ids[e.pointerId]={x:e.clientX,y:e.clientY};
  if(count()===1){drag={id:e.pointerId,x:e.clientX,y:e.clientY,tx:tx,ty:ty};moved=false;pinch=null;}
  else{var p=pair();if(p){pinch={d:dist(p)};drag=null;moved=true;}}
});
im.addEventListener('pointermove',function(e){
  if(!ids[e.pointerId])return;
  ids[e.pointerId]={x:e.clientX,y:e.clientY};
  var p=pair();
  if(p){
    var d=dist(p),m=mid(p);
    if(pinch&&pinch.d>0)zoomAt(d/pinch.d,m.x,m.y);
    pinch={d:d};moved=true;return;
  }
  if(drag&&drag.id===e.pointerId){
    var dx=e.clientX-drag.x,dy=e.clientY-drag.y;
    if(Math.abs(dx)+Math.abs(dy)>3)moved=true;
    tx=drag.tx+dx;ty=drag.ty+dy;apply();
  }
});
function up(e){
  delete ids[e.pointerId];
  if(count()<2)pinch=null;
  if(drag&&drag.id===e.pointerId)drag=null;
}
im.addEventListener('pointerup',up);
im.addEventListener('pointercancel',up);
document.addEventListener('keydown',function(e){
  if(lb.hidden)return;
  if(e.key==='Escape'){close();return;}
  if(e.key==='+'||e.key==='='){zoomAt(1/0.8,window.innerWidth/2,window.innerHeight/2);return;}
  if(e.key==='-'){zoomAt(0.8,window.innerWidth/2,window.innerHeight/2);return;}
  if(e.key==='0')reset();
});
})();`
