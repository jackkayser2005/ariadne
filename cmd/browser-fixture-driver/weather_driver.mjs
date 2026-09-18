import { spawn } from 'node:child_process';
import { createHash } from 'node:crypto';
import { createReadStream } from 'node:fs';
import { mkdtemp } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { isAbsolute, join } from 'node:path';
import { pathToFileURL } from 'node:url';
import { DevTools, delay, freePort, waitForPage, stopBrowser, removeProfile, browserFailure } from './browser_fixture_driver.mjs';

export const procedureID = 'browser-weather-location-v1';
export const origin = 'https://beta.weather.gov';
export const coordinates = Object.freeze({precise: [38.889484, -77.035278], coarse: [38.9072, -77.0369]});
const hash = value => createHash('sha256').update(value).digest('hex');
const maxBody = 64 * 1024;

export function validateRequest(r) {
  const keys = ['schema_version','procedure_id','candidate','challenge','duration_ms'];
  if (!r || Object.keys(r).length !== keys.length || keys.some(k => !Object.hasOwn(r,k)) || r.schema_version !== 1 || r.procedure_id !== procedureID || !['precise','coarse','denied'].includes(r.candidate) || !/^[a-f0-9]{64}$/.test(r.challenge) || !Number.isInteger(r.duration_ms) || r.duration_ms < 100 || r.duration_ms > 30000) throw new Error('weather request invalid');
}

function containsLocationPair(text) {
  const tokens = [...text.matchAll(/(?<![\w.])[+-]?\d+\.\d+(?![\w.])/g)].map(match => Number(match[0]));
  const pairs = Object.values(coordinates).flatMap(pair => {
    const rounded = pair.map(n => Math.round(n*1000)/1000);
    return [pair, rounded, [pair[1], pair[0]], [rounded[1], rounded[0]]];
  });
  for (let index = 0; index + 1 < tokens.length; index += 1) {
    if (pairs.some(pair => pair[0] === tokens[index] && pair[1] === tokens[index + 1])) return true;
  }
  return false;
}

// Match a coordinate PAIR, including the site's reviewed three-decimal rounding.
// Keys and lone numbers are insufficient; unsupported encodings remain unknown.
function inspectLocation(value) {
  if (typeof value !== 'string' || Buffer.byteLength(value) > maxBody) return {matched:false,unsupported:false};
  let text;
  try { text = decodeURIComponent(value.replaceAll('+',' ')); } catch { return {matched:false,unsupported:true}; }
  // A second percent-decoding pass is deliberately outside the reviewed scope.
  // Keep the gap even when another part of this value matched.
  const unsupported = /%[0-9a-f]{2}/i.test(text);
  return {matched:containsLocationPair(text),unsupported};
}

export function matchesLocation(value) { return inspectLocation(value).matched; }

export function allowedRequest(value) {
  try { const u = new URL(value); return u.origin === origin && !u.username && !u.password && u.protocol === 'https:'; } catch { return false; }
}

export function weatherCollector(limit = 1024) {
  const observations = new Map();
  const gaps = new Set(['server-side-unobservable']);
  const pending = new Map();
  let count = 0;
  let rateLimited = false;
  const add = (stage,destination) => observations.set(stage+destination,{stage,destination,category:'location'});
  return {
    request(p) {
      p ||= {};
      if (++count > limit) { gaps.add('capture-incomplete'); return; }
      const requestID = typeof p.requestId === 'string' && p.requestId.length > 0 ? p.requestId : null;
      if (!requestID) gaps.add('capture-incomplete');
      const r = p.request || {};
      if (p.redirectResponse) {
        if (p.redirectResponse.status === 429) rateLimited = true;
        if (p.redirectResponse.fromServiceWorker || p.redirectResponse.fromDiskCache) gaps.add('capture-incomplete');
        if (!allowedRequest(p.redirectResponse.url)) gaps.add('blocked-origin');
        if (requestID && pending.get(requestID) && allowedRequest(p.redirectResponse.url) && !p.redirectResponse.fromServiceWorker && !p.redirectResponse.fromDiskCache) add('response-backed','weather-service');
        if (requestID) pending.delete(requestID);
      }
      const urlMatch = inspectLocation(r.url);
      let matched = urlMatch.matched;
      if (urlMatch.unsupported) gaps.add('unsupported-encoding');
      if (r.hasPostData) {
        const type = Object.entries(r.headers || {}).find(([k]) => k.toLowerCase()==='content-type')?.[1] || '';
        if (typeof r.postData !== 'string' || Buffer.byteLength(r.postData)>maxBody || !/^(application\/(json|x-www-form-urlencoded)|text\/plain)(;|$)/i.test(type)) gaps.add('unsupported-body');
        else {
          const bodyMatch = inspectLocation(r.postData);
          matched ||= bodyMatch.matched;
          if (bodyMatch.unsupported) gaps.add('unsupported-encoding');
        }
      }
      const allowed = allowedRequest(r.url);
      if (!allowed) gaps.add('blocked-origin');
      if (matched) { add('attempted',allowed ? 'weather-service' : 'undeclared'); if (allowed && requestID) pending.set(requestID,true); }
    },
    response(p) {
      p ||= {};
      const requestID = typeof p.requestId === 'string' && p.requestId.length > 0 ? p.requestId : null;
      if (!requestID) gaps.add('capture-incomplete');
      if (p.response?.status === 429) rateLimited = true;
      if (p.response?.fromServiceWorker || p.response?.fromDiskCache) gaps.add('capture-incomplete');
      if (requestID && pending.get(requestID)) {
        if (allowedRequest(p.response?.url) && !p.response?.fromServiceWorker && !p.response?.fromDiskCache) add('response-backed','weather-service');
        else if (p.response?.url && !allowedRequest(p.response.url)) gaps.add('blocked-origin');
        else gaps.add('capture-incomplete');
        pending.delete(requestID);
      }
    },
    failed(p) {
      const requestID = typeof p?.requestId === 'string' && p.requestId.length > 0 ? p.requestId : null;
      if (!requestID) gaps.add('capture-incomplete');
      else pending.delete(requestID);
      gaps.add('capture-incomplete');
    },
    gap(g) { gaps.add(g); },
    rateLimited() { return rateLimited; },
    result() { if (pending.size) gaps.add('capture-incomplete'); return {gaps:[...gaps].sort(),observations:[...observations.values()].sort((a,b)=>(a.stage+a.destination).localeCompare(b.stage+b.destination)),rateLimited}; }
  };
}

async function executableHash(path) { const h=createHash('sha256');for await(const chunk of createReadStream(path))h.update(chunk);return h.digest('hex'); }

export async function captureWeather(request, executable) {
  validateRequest(request);
  if (!isAbsolute(executable)) throw new Error('absolute browser required');
  const browserSHA256 = await executableHash(executable);
  const deadline = Date.now()+request.duration_ms;
  const profile = await mkdtemp(join(tmpdir(),'ariadne-weather-'));
  let browser, cdp, timer;
  const result={schema_version:1,procedure_id:procedureID,candidate:request.candidate,challenge_sha256:hash(request.challenge),browser_sha256:browserSHA256,functionality:'unknown',geolocation:'unknown',status:'workflow-failed',gaps:[],observations:[]};
  const collector=weatherCollector();
  try {
    const port=await freePort();
    browser=spawn(executable,['--headless=new','--disable-gpu','--disable-extensions','--disable-background-networking','--disable-component-update','--disable-sync','--disable-crash-reporter','--disable-breakpad','--no-proxy-server','--no-first-run','--no-default-browser-check','--host-resolver-rules=MAP * ~NOTFOUND, EXCLUDE beta.weather.gov',`--user-data-dir=${profile}`,'--remote-debugging-address=127.0.0.1',`--remote-debugging-port=${port}`,'about:blank'],{stdio:'ignore',windowsHide:true,detached:process.platform!=='win32'});
    const work=async()=>{
      const socket=await Promise.race([waitForPage(port,deadline),browserFailure(browser)]);
      cdp=new DevTools(socket);await cdp.connect();
      cdp.on('Network.requestWillBeSent',p=>collector.request(p));
      cdp.on('Network.responseReceived',p=>collector.response(p));
      cdp.on('Network.loadingFailed',p=>collector.failed(p));
      cdp.on('Network.webSocketCreated',()=>collector.gap('unsupported-channel'));
      cdp.on('Target.targetCreated',p=>{if(['worker','service_worker','shared_worker'].includes(p.targetInfo?.type))collector.gap('unsupported-channel');});
      cdp.on('Fetch.requestPaused',p=>{const allowed=allowedRequest(p.request?.url);if(!allowed)collector.gap('blocked-origin');void cdp.command(allowed?'Fetch.continueRequest':'Fetch.failRequest',allowed?{requestId:p.requestId}:{requestId:p.requestId,errorReason:'BlockedByClient'}).catch(()=>collector.gap('capture-incomplete'));});
      await cdp.command('Page.enable');await cdp.command('Network.enable',{maxPostDataSize:maxBody});
      await cdp.command('Network.setCacheDisabled',{cacheDisabled:true});
      await cdp.command('Network.setBypassServiceWorker',{bypass:true});
      await cdp.command('Network.setBlockedURLs',{urls:['ws://*','wss://*']});
      await cdp.command('Target.setDiscoverTargets',{discover:true});
      await cdp.command('Fetch.enable',{patterns:[{urlPattern:'*',requestStage:'Request'}]});
      await cdp.command('Browser.setPermission',{permission:{name:'geolocation'},setting:request.candidate==='denied'?'denied':'granted',origin});
      result.geolocation=request.candidate==='denied'?'denied':'granted';
      if(request.candidate!=='denied'){const [latitude,longitude]=coordinates[request.candidate];await cdp.command('Emulation.setGeolocationOverride',{latitude,longitude,accuracy:request.candidate==='precise'?5:5000});}
      await cdp.command('Page.navigate',{url:origin+'/'});
      let activated=false;
      while(Date.now()<deadline){
        if(collector.rateLimited()){result.status='rate-limited';return;}
        const state=await cdp.command('Runtime.evaluate',{returnByValue:true,expression:`(() => { const buttons=[...document.querySelectorAll('button.weathergov-use-browser-location')].filter(b=>b.getClientRects().length && !b.disabled); const ready=document.readyState==='complete' && !!document.querySelector('form[data-location-search]'); const available=document.querySelector('h1')?.textContent.trim()==='Washington, DC' && !!document.querySelector('wx-daily-forecast .wx-quick-forecast-item'); return {ready,buttons:buttons.length,available}; })()`});
        const v=state.result?.value;
        if(!v || state.exceptionDetails)throw new Error('workflow inspection failed');
        if(v.available){result.functionality='available';result.status='complete';await delay(500);return;}
        if(v.ready && request.candidate==='denied' && v.buttons===0){result.functionality='unavailable';result.status='complete';await delay(500);return;}
        if(v.ready && v.buttons>0 && !activated){
          // Only the fixed, visibly available control observed on the reviewed page.
          await cdp.command('Runtime.evaluate',{expression:`[...document.querySelectorAll('button.weathergov-use-browser-location')].find(b=>b.getClientRects().length && !b.disabled)?.click()`});activated=true;
        }
        await delay(200);
      }
    };
    await Promise.race([work(),new Promise((_,reject)=>{timer=setTimeout(()=>reject(new Error('weather timeout')),request.duration_ms);})]);
  } catch { collector.gap('capture-incomplete');result.functionality='unknown';result.status='workflow-failed'; }
  finally {
    clearTimeout(timer);cdp?.close();if(browser)await stopBrowser(browser);await removeProfile(profile);
  }
  if(await executableHash(executable)!==browserSHA256)throw new Error('browser changed');
  const observed=collector.result();result.gaps=observed.gaps;result.observations=observed.observations;
  if(observed.rateLimited){result.status='rate-limited';result.functionality='unknown';}
  return result;
}

if(process.argv[1] && import.meta.url===pathToFileURL(process.argv[1]).href){
 try {
  const args=process.argv.slice(2);if(args.length!==2 || args[0]!=='--browser')throw new Error('arguments invalid');
  const chunks=[];let size=0;for await(const chunk of process.stdin){size+=chunk.length;if(size>16384)throw new Error('input limit');chunks.push(chunk);}
  const request=JSON.parse(Buffer.concat(chunks).toString('utf8'));
  process.stdout.write(JSON.stringify(await captureWeather(request,args[1])));
 }catch{process.stderr.write('weather driver failed\n');process.exitCode=1;}
}
