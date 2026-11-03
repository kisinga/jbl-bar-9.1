// Exercise the real shared server and a synthetic optional-feature composition.
// No soundbar, firmware, credentials or frontend build tool is required.
import assert from 'node:assert/strict';
import {spawn,spawnSync} from 'node:child_process';
import {mkdtemp,writeFile,rm} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {fileURLToPath} from 'node:url';
const {chromium}=await import(process.env.PLAYWRIGHT_MODULE || 'playwright');
const remote=fileURLToPath(new URL('../apps/remote/',import.meta.url));
const folder=await mkdtemp(join(tmpdir(),'jbl-components-'));
const binary=join(folder,'preview');
const build=spawnSync('go',['build','-o',binary,'./cmd/preview'],{cwd:remote,encoding:'utf8'});
assert.equal(build.status,0,build.stderr);
const browser=await chromium.launch({headless:true});
const children=[];
const errors=[];
let checks=0;
const check=(condition,message)=>{assert.ok(condition,message);checks++;};
async function start(fixture) {
  const args=['--listen','127.0.0.1:0'];
  if(fixture){const path=join(folder,'state.json');await writeFile(path,JSON.stringify(fixture));args.push('--fixture',path);}
  const child=spawn(binary,args,{cwd:remote,stdio:['ignore','pipe','pipe']});children.push(child);
  const url=await new Promise((resolve,reject)=>{
    let output='';const timer=setTimeout(()=>reject(Error('Preview startup timed out')),10000);
    child.stdout.on('data',data=>{output+=data;const match=output.match(/http:\/\/127\.0\.0\.1:\d+/);if(match){clearTimeout(timer);resolve(match[0]);}});
    child.once('error',reject);child.once('exit',code=>{clearTimeout(timer);reject(Error('Preview exited '+code));});
  });
  return url;
}
async function pageFor(url){const page=await browser.newPage();page.on('pageerror',error=>errors.push(error.message));await page.goto(url);await page.waitForFunction(()=>document.querySelector('#connection').dataset.state==='online');return page;}
try {
  const url=await start();const page=await pageFor(url);
  check(await page.title()==='Toolkit preview · Remote','composition supplies model branding');
  check(await page.locator('#eq-panel').isHidden(),'unregistered EQ component is hidden');
  check(await page.locator('[data-component=center-level]').isHidden(),'unregistered center component is hidden');
  check(await page.locator('[data-action=input_hdmi]').isDisabled(),'unsupported core controls are disabled');
  await page.locator('[data-action=volume_up]').click();
  await page.waitForFunction(()=>document.querySelector('#volume').textContent==='13');
  check(true,'shared preview supports safe simulated controls');
  await page.close();

  const fixture={available:true,power:'On',mode:'Unknown',input:'Fixture',volume:9,muted:false,source_state:0,sound:{av_sync_ms:20},extensions:{
    'fixture.eq':{available:true,enabled:false,gains_db:[0,0,0,0,0],frequencies_hz:[125,500,2000,4000,8000],status:'original'},
    'fixture.center':{level:2},
  }};
  const fixtureURL=await start(fixture);
  const fixtureProcess=children.at(-1);
  const snapshot=await (await fetch(fixtureURL+'/api/state')).json();
  const body={action:'volume_up'};
  const rejected=await fetch(fixtureURL+'/api/action',{method:'POST',headers:{'Content-Type':'application/json','X-CSRF-Token':snapshot.csrf},body:JSON.stringify(body)});
  check(rejected.status===409,'normalized fixture cannot mutate real backend state');
  const readonly=await pageFor(fixtureURL);
  check(await readonly.locator('[data-action=volume_up]').isDisabled(),'fixture replay advertises no actions');
  await readonly.close();

  const unknownURL=await start({available:true,power:'Unknown',mode:'Unknown',input:'Unknown',volume:19,muted:null});
  const unknown=await pageFor(unknownURL);
  check(await unknown.locator('#volume').textContent()==='19' && await unknown.locator('#volume-caption').textContent()==='POWER UNKNOWN','partial observations do not turn unknown power into standby');
  check((await unknown.locator('#summary').textContent()).includes('Power state is not reported'),'unknown power does not claim startup');
  check(await unknown.locator('#mute').getAttribute('aria-pressed')==='mixed' && (await unknown.locator('#mute-label').textContent()).includes('unavailable'),'unknown mute is not reported as unmuted');
  check(await unknown.locator('#power').isDisabled() && await unknown.locator('#mute').isDisabled(),'partial research state enables no guessed controls');
  await unknown.close();

  // Mock only the feature contract/action responses. The real loopback server
  // still supplies all embedded assets and the initial snapshot/SSE stream.
  const composition=await browser.newPage();composition.on('pageerror',error=>errors.push(error.message));
  const features={schema:1,model:'fixture-family',name:'Fixture family',actions:['av_sync_set'],extensions:[
    {id:'fixture.eq',component:'front-eq',included:true,actions:['fixture.eq.apply','fixture.eq.reset']},
    {id:'fixture.center',component:'center-level',included:true,actions:['fixture.center.next']},
    {id:'fixture.unknown',component:'https://invalid.example/plugin.js',included:true,actions:[]},
  ]};
  const requests=[];const external=[];
  composition.on('request',request=>{if(!request.url().startsWith(fixtureURL))external.push(request.url());});
  await composition.route('**/api/features',route=>route.fulfill({json:features}));
  await composition.route('**/api/action',async route=>{
    const command=route.request().postDataJSON();requests.push(command);
    if(command.action==='fixture.eq.apply'){snapshot.extensions['fixture.eq'].gains_db=command.parameters.gains_db;snapshot.extensions['fixture.eq'].enabled=true;}
    if(command.action==='fixture.center.next')snapshot.extensions['fixture.center'].level++;
    if(command.action==='av_sync_set')snapshot.sound.av_sync_ms=command.value;
    snapshot.revision++;
    await route.fulfill({json:snapshot});
  });
  await composition.goto(fixtureURL);await composition.waitForFunction(()=>document.querySelector('#connection').dataset.state==='online');
  check(await composition.locator('#eq-panel').isVisible(),'compatible EQ component works with a different namespace');
  check((await composition.locator('#center-value').textContent()).includes('Level 2'),'center component reads namespaced state without legacy sound fields');
  await composition.locator('#eq-slider-0').evaluate(input=>{input.value='-2';input.dispatchEvent(new Event('input',{bubbles:true}));});
  await composition.locator('#eq-apply').click();await composition.waitForFunction(()=>document.querySelector('#eq-state').textContent.includes('Custom EQ active'));
  check(requests[0].action==='fixture.eq.apply' && requests[0].parameters.gains_db[0]===-2 && !('gains_db' in requests[0]),'EQ emits the declared typed extension action');
  await composition.locator('#center-next').click();await composition.waitForFunction(()=>document.querySelector('#center-value').textContent.includes('Level 3'));
  check(requests[1].action==='fixture.center.next','center emits the declared namespaced action');
  await composition.locator('#advanced summary').click();
  check(!(await composition.locator('#av-sync-up').isDisabled()),'advertised audio-sync setter enables local stepper intents');
  await composition.locator('#av-sync-up').click();await composition.locator('#av-sync-up').click();await composition.locator('#av-sync-up').click();
  await composition.waitForFunction(()=>document.querySelector('#av-sync-value').textContent==='50 ms');
  check(requests.length===3 && requests[2].action==='av_sync_set' && requests[2].value===50,'contributed audio-sync draft stays one legacy-compatible request through discovery');
  await composition.locator('#av-sync-up').click();
  fixtureProcess.kill('SIGTERM');
  await composition.waitForFunction(()=>document.querySelector('#connection-label').textContent==='Offline');
  await new Promise(resolve=>setTimeout(resolve,1100));
  check(requests.length===3 && !(await composition.locator('#av-sync-value').textContent()).includes('now'),'disconnect discards unsent sync draft without replay');
  check(external.length===0,'unknown feature metadata never loads remote code');
  check(errors.length===0,'no browser exceptions: '+errors.join(', '));
  console.log(checks+' component and shared-server browser checks passed');
} finally {
  await browser.close();
  for(const child of children)if(child.exitCode===null)child.kill('SIGTERM');
  await Promise.all(children.map(child=>child.exitCode===null?new Promise(resolve=>child.once('exit',resolve)):Promise.resolve()));
  await rm(folder,{recursive:true,force:true});
}
