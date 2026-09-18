import test from 'node:test';
import assert from 'node:assert/strict';
import {matchesLocation,allowedRequest,weatherCollector,validateRequest,procedureID} from './weather_driver.mjs';

test('matches paired synthetic coordinates in paths, encoded queries, and JSON, not keys',()=>{
 for(const value of ['/forecast/point/38.889/-77.035','lat=38.889484&lon=-77.035278','%33%38.9072%2C-77.0369','{"lat":38.907,"lon":-77.037}'])assert.equal(matchesLocation(value),true,value);
 for(const value of ['latitude=other&longitude=other','latitude=38.889','138.889/-77.035','38.889/-177.035','38.88/-77.03','%invalid','x'.repeat(70000),null])assert.equal(matchesLocation(value),false);
});
test('exact origin guard rejects credentials, redirects, private destinations and websockets',()=>{
 assert.equal(allowedRequest('https://beta.weather.gov/forecast/point/38.889/-77.035'),true);
 for(const u of ['https://beta.weather.gov.attacker.test/','https://x@beta.weather.gov/','http://beta.weather.gov/','https://127.0.0.1/','wss://beta.weather.gov/','not a url'])assert.equal(allowedRequest(u),false);
});
test('only matching requests with actual responses become response-backed',()=>{
 const c=weatherCollector();c.request({requestId:'1',request:{url:'https://beta.weather.gov/forecast/point/38.889/-77.035'}});
 assert.equal(c.result().observations[0].stage,'attempted');
 c.response({requestId:'1',response:{url:'https://beta.weather.gov/forecast/point/38.889/-77.035',status:200}});
 assert.equal(c.result().observations.length,2);
 const d=weatherCollector();d.request({requestId:'1',request:{url:'https://other.test/?lat=38.889&lon=-77.035'}});d.response({requestId:'1',response:{url:'https://other.test/',status:200}});assert.deepEqual(d.result().observations,[{stage:'attempted',destination:'undeclared',category:'location'}]);
});
test('redirect responses preserve origin and cache boundaries',()=>{
 const c=weatherCollector();
 c.request({requestId:'1',request:{url:'https://beta.weather.gov/forecast/point/38.889/-77.035'}});
 c.request({requestId:'1',redirectResponse:{url:'https://beta.weather.gov/forecast/point/38.889/-77.035',status:302},request:{url:'https://beta.weather.gov/'}});
 assert(c.result().observations.some(o=>o.stage==='response-backed' && o.destination==='weather-service'));
 const d=weatherCollector();
 d.request({requestId:'1',request:{url:'https://beta.weather.gov/forecast/point/38.889/-77.035'}});
 d.request({requestId:'1',redirectResponse:{url:'https://outside.test/redirect',status:302},request:{url:'https://beta.weather.gov/'}});
 let result=d.result();
 assert(!result.observations.some(o=>o.stage==='response-backed'));
 assert(result.gaps.includes('blocked-origin'));
 const e=weatherCollector();
 e.request({requestId:'1',request:{url:'https://beta.weather.gov/forecast/point/38.889/-77.035'}});
 e.request({requestId:'1',redirectResponse:{url:'https://beta.weather.gov/forecast/point/38.889/-77.035',status:302,fromDiskCache:true},request:{url:'https://beta.weather.gov/'}});
 result=e.result();
 assert(!result.observations.some(o=>o.stage==='response-backed'));
 assert(result.gaps.includes('capture-incomplete'));
});test('missing request identifiers cannot become response-backed',()=>{
 const c=weatherCollector();
 c.request({request:{url:'https://beta.weather.gov/forecast/point/38.889/-77.035'}});
 c.response({response:{url:'https://beta.weather.gov/forecast/point/38.889/-77.035',status:200}});
 const result=c.result();
 assert(result.gaps.includes('capture-incomplete'));
 assert(!result.observations.some(o=>o.stage==='response-backed'));
});
test('unsupported percent encodings remain an explicit visibility gap',()=>{
 const c=weatherCollector();
 c.request({requestId:'nested',request:{url:'https://beta.weather.gov/',hasPostData:true,headers:{'Content-Type':'application/json'},postData:'{\"where\":\"%2533%2538.889%252C-77.035278\"}'}});
 c.request({requestId:'malformed',request:{url:'https://beta.weather.gov/',hasPostData:true,headers:{'Content-Type':'text/plain'},postData:'location=%invalid'}});
 const result=c.result();
 assert(result.gaps.includes('unsupported-encoding'));
 assert(!result.observations.some(o=>o.stage==='attempted'));
});
test('body coverage, overflow, failures, cached responses and rate limits remain explicit',()=>{
 const c=weatherCollector(3);
 c.request({requestId:'a',request:{url:'https://beta.weather.gov/',hasPostData:true,headers:{'Content-Type':'application/json'},postData:'{"lat":38.889,"lon":-77.035}'}});
 c.response({requestId:'a',response:{url:'https://beta.weather.gov/',status:429,fromDiskCache:true}});
 c.request({requestId:'b',request:{url:'https://beta.weather.gov/',hasPostData:true,headers:{'Content-Type':'application/octet-stream'},postData:'binary'}});
 c.request({requestId:'c',request:{url:'https://beta.weather.gov/',hasPostData:true}});c.failed({requestId:'c'});c.request({});
 assert.equal(c.result().rateLimited,true);assert(c.result().gaps.includes('unsupported-body'));assert(c.result().gaps.includes('capture-incomplete'));assert(!c.result().observations.some(o=>o.stage==='response-backed'));
});
test('strict runner request has no arbitrary targets, scripts, or real coordinates',()=>{
 const r={schema_version:1,procedure_id:procedureID,candidate:'precise',challenge:'a'.repeat(64),duration_ms:30000};validateRequest(r);
 for(const v of [{...r,url:'https://other.test/'},{...r,candidate:'custom'},{...r,challenge:'bad'},{...r,duration_ms:30001},null])assert.throws(()=>validateRequest(v));
});
