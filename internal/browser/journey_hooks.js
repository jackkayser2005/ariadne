(() => {
  'use strict';
  if (globalThis.__ariadneInstalled) return;
  Object.defineProperty(globalThis, '__ariadneInstalled', {value: true});
  const binding = globalThis.__ariadneObserve;
  const stringify = JSON.stringify;
  const limit = 262144;
  const emit = (kind, payload = '', url = '', selector = '', checkpoint = false) => {
    try {
      if (typeof payload !== 'string' || payload.length > limit) {
        binding(stringify({gap: 'unsupported-payload'})); return;
      }
      binding(stringify({kind, payload, url: String(url || location.href).slice(0, 4096), selector, checkpoint}));
    } catch (_) { /* An unavailable binding is detected by the collector handshake. */ }
  };
  const gap = () => { try { binding(stringify({gap: 'unsupported-payload'})); } catch (_) {} };
  const text = value => {
    if (value === undefined || value === null) return '';
    if (typeof value === 'string') return value;
    if (typeof URLSearchParams !== 'undefined' && value instanceof URLSearchParams) return value.toString();
    if (typeof FormData !== 'undefined' && value instanceof FormData) {
      let result = '';
      for (const [key, part] of value.entries()) {
        if (typeof part !== 'string' || result.length + key.length + part.length > limit) { gap(); return ''; }
        result += key + '=' + part + '\n';
      }
      return result;
    }
    gap(); return '';
  };
  const wrap = (object, name, before) => {
    try {
      const original = object[name];
      if (typeof original !== 'function') return;
      object[name] = function (...args) { try { before.apply(this,args); } catch (_) { gap(); } return Reflect.apply(original,this,args); };
    } catch (_) { gap(); }
  };
  const selector = element => {
    const parts=[];
    for (let current=element;current && current.nodeType===1 && parts.length<12;current=current.parentElement) {
      let index=1;
      for (let sibling=current.previousElementSibling;sibling;sibling=sibling.previousElementSibling) if (sibling.localName===current.localName) index++;
      parts.unshift(current.localName+':nth-of-type('+index+')');
    }
    return parts.join(' > ').slice(0,512);
  };
  if (typeof document !== 'undefined') {
    document.addEventListener('input', event => {
      const element=event.target;
      if (element instanceof HTMLInputElement || element instanceof HTMLTextAreaElement) emit('input',element.value,'',selector(element));
      else gap();
    },true);
    document.addEventListener('click', event => {
      const element=event.target;
      if (element instanceof Element) emit('click','','',selector(element),true);
    },true);
    document.addEventListener('submit', () => emit('checkpoint'),true);
    wrap(Storage.prototype,'setItem',function(key,value){emit('storage-write',text(key)+'='+text(value));});
    try {
      const get=Storage.prototype.getItem;
      Storage.prototype.getItem=function(key){ const value=Reflect.apply(get,this,[key]);emit('storage-read',text(value));return value; };
      const cookie=Object.getOwnPropertyDescriptor(Document.prototype,'cookie');
      if (cookie && cookie.get && cookie.set) Object.defineProperty(Document.prototype,'cookie',{
        configurable:cookie.configurable, enumerable:cookie.enumerable,
        get(){const value=Reflect.apply(cookie.get,this,[]);emit('cookie-read',value);return value;},
        set(value){emit('cookie-write',text(value));return Reflect.apply(cookie.set,this,[value]);}
      }); else gap();
    } catch (_) { gap(); }
    if (navigator.sendBeacon) wrap(Object.getPrototypeOf(navigator),'sendBeacon',(url,body)=>emit('beacon',text(body),new URL(url,location.href).href));
  }
  if (typeof fetch==='function') wrap(globalThis,'fetch',(input,options)=>{
    const url=typeof input==='string' || input instanceof URL ? String(input) : input.url;
    if (typeof Request!=='undefined' && input instanceof Request && input.body) gap();
    emit('fetch',text(options && options.body),new URL(url,location.href).href);
  });
  if (typeof XMLHttpRequest!=='undefined') {
    const urls=new WeakMap();
    wrap(XMLHttpRequest.prototype,'open',function(method,url){urls.set(this,new URL(url,location.href).href);});
    wrap(XMLHttpRequest.prototype,'send',function(body){emit('xhr',text(body),urls.get(this));});
  }
  if (typeof Worker!=='undefined') {
    wrap(Worker.prototype,'postMessage',function(value){emit('worker-send',text(value));});
    try {
      globalThis.Worker=new Proxy(Worker,{construct(target,args,newTarget){
        const worker=Reflect.construct(target,args,newTarget);
        worker.addEventListener('message',event=>emit('worker-receive',text(event.data)));
        return worker;
      }});
    } catch (_) { gap(); }
  }
  if (typeof MessagePort!=='undefined') wrap(MessagePort.prototype,'postMessage',function(value){emit('worker-send',text(value));});
  if (typeof document==='undefined' && typeof postMessage==='function') wrap(globalThis,'postMessage',value=>emit('worker-send',text(value)));
  globalThis.addEventListener('message',event=>emit(typeof document==='undefined'?'worker-receive':'message-receive',text(event.data)));
  emit('ready');
})()
