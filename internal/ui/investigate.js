'use strict';
(() => {
  const $ = id => document.getElementById(id);
  const fragment = location.hash.slice(1);
  if (fragment) { sessionStorage.setItem('ariadne-session', fragment); history.replaceState(null, '', location.pathname); }
  const token = fragment || sessionStorage.getItem('ariadne-session') || '';
  let busy = false, initial = true, lastEvidence = '', lastMarkers = '';
  const node = (tag, text, className) => { const element = document.createElement(tag); if (text !== undefined) element.textContent = text; if (className) element.className = className; return element; };
  const showError = error => { $('message').textContent = error.message; $('message').className = 'error'; $('message').hidden = false; };
  async function request(path, body) {
    const response = await fetch('/api/' + path, {method: body === undefined ? 'GET' : 'POST', credentials:'omit', cache:'no-store', headers:{'Authorization':'Bearer ' + token, ...(body === undefined ? {} : {'Content-Type':'application/json'})}, ...(body === undefined ? {} : {body:JSON.stringify(body)})});
    if (!response.ok) throw new Error((await response.text()).slice(0,1024));
    return response;
  }
  const gaps = {
    'instrumentation-unavailable':'Some browser activity cannot be verified by this recorder. Absence remains unknown.',
    'worker-unavailable':'A worker started outside a fully instrumented interval.',
    'frame-unavailable':'A frame could not be fully instrumented.',
    'unsupported-payload':'A payload or interaction used an unsupported format.',
    'event-limit':'The recording reached its event limit. Later activity may be missing.',
    'size-limit':'An observation exceeded a size limit and was not retained.',
    'request-failed':'A request failed before a response could be observed.',
    'browser-crashed':'The browser connection ended unexpectedly.',
    'cancelled':'Recording was cancelled.',
    'out-of-scope-navigation':'Navigation outside the chosen website was blocked.',
    'manual-checkpoint':'An interaction requires a manual checkpoint.',
    'unmatched-information':'Information other than the supplied synthetic markers is outside this investigation.',
    'server-side-unobservable':'Onward handling by servers cannot be seen from the browser.'
  };
  function render(state) {
    const recording = state.phase === 'recording' || state.phase === 'interrupted';
    const saved = state.phase === 'saved';
    $('status').textContent = ({ready:'Ready to investigate.',recording:'Recording in a fresh browser profile. Return here to stop or cancel.',interrupted:'The browser recording ended. Stop to save the partial evidence, or cancel to discard it.',saved:'Investigation saved. Review the observations and visibility limits.',cancelled:'Recording cancelled. The browser profile was removed and no investigation was saved.',error:'The investigation could not be saved.'})[state.phase] || 'Investigation unavailable.';
    $('capture-panel').hidden = state.read_only;
    $('site').disabled = recording || busy;
    $('start').hidden = recording;
    $('stop').hidden = !recording; $('cancel').hidden = !recording;
    $('test-inputs').hidden = !recording;
    $('less-panel').hidden = !saved || state.read_only;
    $('export').hidden = !saved;
    if (initial) {
      $('site').value = state.initial_url || '';
      initial=false;
    }
    const markerSignature=JSON.stringify(state.markers);
    if (markerSignature!==lastMarkers) {
      lastMarkers=markerSignature;
      $('markers').replaceChildren();
      for (const marker of state.markers || []) {
        const row = node('div',undefined,'marker panel');
        const label = node('label',marker.category === 'email' ? 'Test email' : 'Test account identifier');
        const field = node('input');field.readOnly=true;field.value=marker.value;field.id='marker-'+marker.id;label.htmlFor=field.id;
        const button = node('button','Fill selected field');button.type='button';button.addEventListener('click',()=>act('fill',{marker:marker.id}));
        row.append(label,field,button);$('markers').append(row);
      }
    }
    $('saved-path').textContent = state.saved_path ? 'Saved locally: '+state.saved_path : 'This recording has not been saved.';
    if (!state.journey) return;
    const signature=JSON.stringify([state.journey,state.destinations]);if(signature===lastEvidence)return;lastEvidence=signature;
    const names=new Map((state.destinations||[]).map(d=>[d.alias,d.origin]));
    const categories=new Map((state.categories||[]).map(c=>[c.id||c.ID,c.label||c.Label]));
    $('cards').replaceChildren();$('timeline').replaceChildren();$('gaps').replaceChildren();$('destinations').replaceChildren();
    const groups=new Map();
    for(const observation of state.journey.observations){
      for(const match of observation.matches){const key=match.category+'/'+observation.destination;const item=groups.get(key)||{category:match.category,destination:observation.destination,kinds:new Set()};item.kinds.add(observation.kind);groups.set(key,item);}
      const matches=observation.matches.map(m=>(categories.get(m.category)||m.category)+' · '+m.encoding).join(', ');
      $('timeline').append(node('li',observation.kind.replaceAll('-',' ')+' · '+observation.context+' · '+(names.get(observation.destination)||observation.destination)+(matches?' — '+matches:''),'observation'));
    }
    $('overview').textContent=groups.size ? groups.size+' information/destination combinations observed. Expand the timeline for supporting observations.' : 'No supported test-input matches have been observed. This does not show that no information was shared.';
    for(const item of groups.values()){const card=node('article',undefined,'card');card.append(node('h3',categories.get(item.category)||item.category),node('p',names.get(item.destination)||item.destination),node('small',Array.from(item.kinds).map(x=>x.replaceAll('-',' ')).join(', ')));$('cards').append(card);}
    if(state.journey.gaps.length){const box=node('div',undefined,'gap');box.append(node('h3','What remains unknown'));const list=node('ul');for(const gap of state.journey.gaps)list.append(node('li',gaps[gap]||'An unsupported visibility boundary was recorded.'));box.append(list);$('gaps').append(box);}
    for(const destination of state.destinations||[]){const label=node('label');const checkbox=node('input');checkbox.type='checkbox';checkbox.value=destination.origin;label.append(checkbox,document.createTextNode(' '+destination.origin));$('destinations').append(label);}
    $('identity').textContent='Journey schema '+state.journey.schema_version+' · '+state.journey.observations.length+' ordered observations · '+state.journey.completeness+' visibility';
  }
  async function act(action,body){if(busy)return;busy=true;$('message').hidden=true;document.querySelectorAll('button').forEach(b=>b.disabled=true);$('status').textContent=action==='start'?'Opening a fresh recording browser…':'Working…';try{render(await(await request(action,body)).json());if(action==='stop')$('review-title').scrollIntoView({behavior:'smooth'});}catch(error){showError(error);}finally{busy=false;document.querySelectorAll('button').forEach(b=>b.disabled=false);}}
  $('start-form').addEventListener('submit',event=>{event.preventDefault();act('start',{url:$('site').value});});
  $('stop').addEventListener('click',()=>act('stop',{}));$('cancel').addEventListener('click',()=>act('cancel',{}));
  $('retry').addEventListener('click',()=>act('start',{url:$('site').value,location:$('location').value,block_origins:Array.from($('destinations').querySelectorAll('input:checked')).map(input=>input.value)}));
  $('export').addEventListener('click',async()=>{try{const response=await request('export');const url=URL.createObjectURL(await response.blob());const link=node('a');link.href=url;link.download='ariadne-evidence.json';link.click();setTimeout(()=>URL.revokeObjectURL(url),1000);}catch(error){showError(error);}});
  async function refresh(){if(!busy){try{render(await(await request('state')).json());}catch(error){showError(error);return;}}setTimeout(refresh,1500);}
  refresh();
})();
