// Run with Node and Playwright installed; PLAYWRIGHT_MODULE can point to its index.mjs.
import assert from 'node:assert/strict';
import {createServer} from 'node:http';
import {readFile, mkdir} from 'node:fs/promises';
import {fileURLToPath} from 'node:url';
const {chromium} = await import(process.env.PLAYWRIGHT_MODULE || 'playwright');
const root = new URL('../', import.meta.url);
const pause = ms => new Promise(resolve => setTimeout(resolve, ms));
const initial = () => ({available:true, power:'On', mode:'Standard', volume:12, muted:false, input:'TV / ARC', source:6, csrf:'fixture', watcher:'', sound:{bass:0,atmos:1,rear:2,rear_docked:true}, wifi:{available:true,connected:true,ssid:'Example Wi-Fi',ip:'192.168.50.1',signal_dbm:-48,frequency_mhz:5180}, bluetooth:{selected:false,connectable:false,discoverable:false,connected:false,audio:'Disconnected',name:'My speaker'}});
let state = initial(), calls = [], delay = 300, failure = '', offline = false, getDelay = 0;
const streams=new Set(); let stateGets=0, streamGets=0, revision=0, lastSnapshot='';
function snapshot() {
 const comparable={...state};delete comparable.revision;
 const text=JSON.stringify(comparable);
 if(text!==lastSnapshot){revision++;lastSnapshot=text;}
 state.revision=revision;return {...state};
}
const pushTimer=setInterval(()=>{
 for(const res of streams){if(offline){res.destroy();streams.delete(res);continue;}const text=JSON.stringify(snapshot());if(text!==res.lastSnapshot){res.lastSnapshot=text;res.write('data: '+text+'\n\n');}}
},50);
const server = createServer(async (req, res) => {
  if(req.url==='/api/events') {
    streamGets++;
    if(offline){req.socket.destroy();return;}
    res.writeHead(200,{'Content-Type':'text/event-stream','Cache-Control':'no-store'});
    res.lastSnapshot=JSON.stringify(snapshot());res.write('data: '+res.lastSnapshot+'\n\n');
    streams.add(res);res.on('close',()=>streams.delete(res));return;
  }
  if (req.url.startsWith('/api/')) {
    if (offline) { req.socket.destroy(); return; }
    if (req.method === 'POST') {
      let body = ''; for await (const chunk of req) body += chunk;
      const {action} = JSON.parse(body); calls.push(action);
      await pause(delay);
      if (failure) { res.writeHead(409, {'Content-Type':'application/json'}); res.end(JSON.stringify({error:failure,state})); return; }
      if (action === 'volume_up') state.volume++;
      if (action === 'volume_down') state.volume--;
      if (action === 'bluetooth_pair') {state.bluetooth.discoverable=true;state.bluetooth.connectable=true;}
      if (action === 'bass_next') state.sound.bass=(state.sound.bass+1)%5;
      if (action === 'atmos_next') state.sound.atmos=(state.sound.atmos+1)%3;
      if (action === 'rear_next') state.sound.rear=(state.sound.rear+1)%4;
      if (action === 'mute') state.muted = !state.muted;
      if (action === 'input_bluetooth') {state.input='Bluetooth';state.source=10;state.bluetooth.selected=true;}
    }
    const result = snapshot();
    if (req.method === 'GET') {stateGets++;await pause(getDelay);}
    res.writeHead(200, {'Content-Type':'application/json'}); res.end(JSON.stringify(result)); return;
  }
  const name = {'/':'index.html','/style.css':'style.css','/app.js':'app.js'}[req.url];
  if (!name) {res.writeHead(404);res.end();return;}
  res.setHeader('Content-Type', name.endsWith('.js')?'text/javascript':name.endsWith('.css')?'text/css':'text/html');
  res.end(await readFile(new URL('webremote/www/'+name,root)));
});
await new Promise(resolve => server.listen(0,'127.0.0.1',resolve));
const browser = await chromium.launch({headless:true});
let checks=0;
function check(value,message) {assert.ok(value,message);checks++;}
try {
  const page = await browser.newPage({viewport:{width:390,height:844}, reducedMotion:'reduce'});
  const errors=[]; page.on('pageerror',error=>errors.push(error.message));
  const url='http://127.0.0.1:'+server.address().port;
  await page.goto(url);
  await page.waitForFunction(()=>document.querySelector('#connection').dataset.state==='online');
  check(stateGets===0 && streamGets===1,'initial state uses one push connection');
  const stale={...snapshot(),revision:revision-1,volume:200};
  for(const res of streams)res.write('data: '+JSON.stringify(stale)+'\n\n');
  await pause(100);
  check((await page.locator('#volume').textContent())==='12','stale pushed states cannot rewind the UI');
  check((await page.locator('#wifi-name').textContent())==='Example Wi-Fi','Wi-Fi network shown');
  check(await page.locator('#rear-next').isDisabled(),'docked rear control disabled');
  await page.locator('[data-action=volume_down]').click();
  check(await page.locator('#volume').textContent()==='11','immediate optimistic volume');
  check(await page.locator('[data-action=volume_up]').isEnabled(),'volume remains interactive');
  for(let n=0;n<3;n++) await page.locator('[data-action=volume_down]').click();
  await page.waitForFunction(()=>document.querySelector('#message').dataset.kind==='success');
  check(state.volume===8 && calls.length===4,'rapid taps all confirmed');
  for(let n=0;n<4;n++) await page.locator('[data-action=volume_up]').click();
  await page.waitForFunction(()=>document.querySelector('#message').dataset.kind==='success');
  check(state.volume===12,'restore volume after repeated taps');
  delay=800;
  calls=[];
  await page.locator('[data-action=volume_up]').click();
  await page.locator('[data-action=volume_down]').click();
  await page.locator('[data-action=volume_up]').click();
  await page.waitForFunction(()=>document.querySelector('#message').dataset.kind==='success');
  check(calls.length===1 && state.volume===13,'opposite unsent intents cancel');
  failure='remote control is busy; release its buttons and retry';
  await page.locator('[data-action=volume_up]').click();
  await page.locator('[data-action=volume_up]').click();
  await page.waitForFunction(()=>document.querySelector('#message').dataset.kind==='error');
  check(await page.locator('#volume').textContent()==='13','error rolls optimistic volume back');
  const count=calls.length; await pause(1000);
  check(calls.length===count,'errors discard pending controls');
  failure=''; delay=120;
  await page.keyboard.press('ArrowDown');
  await page.waitForFunction(()=>document.querySelector('#message').dataset.kind==='success');
  check(state.volume===12,'keyboard volume');
  await page.keyboard.press('m');
  await page.waitForFunction(()=>document.querySelector('#message').dataset.kind==='success');
  check(state.muted && await page.locator('#mute').getAttribute('aria-pressed')==='true','keyboard mute');
  const plus=page.locator('[data-action=volume_up]'); const box=await plus.boundingBox();
  await page.mouse.move(box.x+box.width/2,box.y+box.height/2);
  await page.mouse.down(); await pause(1100); await page.mouse.up(); await pause(700);
  const released=calls.length; await pause(600);
  check(calls.length===released,'hold stops generating after release');
  // A stale GET must not rewind a control response.
  getDelay=700;
  await page.locator('#refresh').click(); await pause(50);
  const before=state.volume;
  await page.keyboard.press('ArrowDown');
  await pause(900);
  check(Number(await page.locator('#volume').textContent())===before-1,'stale poll cannot rewind action');
  getDelay=0;
  delay=400; offline=true;
  await page.keyboard.press('ArrowUp'); await page.keyboard.press('ArrowUp');
  await page.waitForFunction(()=>document.querySelector('#connection').dataset.state==='offline');
  check(await plus.isDisabled(),'offline disables controls');
  const atLoss=calls.length; offline=false;
  await page.locator('#refresh').click();
  await page.waitForFunction(()=>document.querySelector('#connection').dataset.state==='online');
  await pause(600);
  check(calls.length===atLoss,'reconnect never replays controls');
  state.power='Standby'; await page.locator('#refresh').click();
  await page.waitForFunction(()=>document.querySelector('#power-state').textContent==='Standby');
  check(await plus.isDisabled() && await page.locator('#power').isEnabled(),'standby only permits power');
  check((await page.locator('#summary').textContent()).includes('standby'),'standby copy');
  state.available=false;state.power='Starting';state.error='Soundbar is starting';
  await page.locator('#refresh').click();
  await page.waitForFunction(()=>document.querySelector('#connection-label').textContent==='Starting');
  check(await page.locator('#power').isDisabled(),'starting firmware disables power');
  state=initial(); await page.locator('#refresh').click();
  await page.waitForFunction(()=>document.querySelector('#power-state').textContent==='On');
  await page.locator('[data-action=input_bluetooth]').click();
  check((await page.locator('#message-text').textContent()).includes('Bluetooth'),'Bluetooth progress');
  await page.waitForFunction(()=>document.querySelector('#message').dataset.kind==='success');
  check(await page.locator('#bt-pair').isEnabled(),'pairing requires selected Bluetooth source');
  check((await page.locator('#bt-help').textContent()).includes('My speaker'),'personalized Bluetooth name');
  await page.locator('#bt-pair').click();
  check((await page.locator('#bt-status').textContent()).includes('Starting'),'pairing progress');
  await page.waitForFunction(()=>document.querySelector('#bt-status').textContent==='Ready to pair');
  check(await page.locator('#bt-pair').isDisabled(),'discoverability readback confirms pairing');
  state.bluetooth.discoverable=false;state.bluetooth.connected=true;state.bluetooth.audio='Streaming';state.bluetooth.peer_name='Example phone';
  await page.locator('#refresh').click();
  await page.waitForFunction(()=>document.querySelector('#bt-status').textContent==='Streaming audio');
  check((await page.locator('#bt-pair-label').textContent())==='Pair another device','connected device pairing action');
  check((await page.locator('#bt-help').textContent()).includes('Example phone'),'connected peer name shown separately from bar name');
  // Clearing queued taps leaves only the already-sent step.
  delay=1600;calls=[];
  const startingVolume=state.volume;
  for(let i=0;i<12;i++)await plus.click();
  check(Number(await page.locator('#volume').textContent())===startingVolume+9,'volume queue has a finite bound');
  await page.locator('#cancel-volume').click();
  await page.waitForFunction(()=>document.querySelector('#message').dataset.kind==='success');
  check(calls.length===1,'cancel clears unsent volume intents');
  state=initial();await page.locator('#refresh').click();await pause(200);
  delay=120;
  await page.locator('#bass-next').click();
  await page.waitForFunction(()=>document.querySelector('#bass-value').textContent==='Level 2 of 5');
  check(state.sound.bass===1,'bass cycle confirms readback');
  await page.locator('#atmos-next').click();
  await page.waitForFunction(()=>document.querySelector('#atmos-value').textContent==='High');
  check(state.sound.atmos===2,'Atmos height cycle confirms readback');
  state.input='AirPlay';state.source_state=7;state.streaming={service:'AirPlay',status:'Session active',active:true};
  await page.waitForFunction(()=>document.querySelector('#playback-service').textContent==='AirPlay');
  check((await page.locator('#playback-status').textContent())==='Session active','AirPlay updates via push without refresh');
  state.input='Chromecast';state.streaming={service:'Chromecast',status:'Paused',active:false};
  await page.waitForFunction(()=>document.querySelector('#playback-service').textContent==='Chromecast');
  check((await page.locator('#playback-status').textContent())==='Paused','Chromecast pause state shown');
  const beforeIdle=stateGets;await pause(1200);
  check(stateGets===beforeIdle,'idle browser never polls the state endpoint');
  state=initial();await page.locator('#refresh').click();await pause(200);
  for (const width of [320,390,1024]) {
    await page.setViewportSize({width,height:900});
    check(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),'no horizontal overflow at '+width);
  }
  await page.setViewportSize({width:390,height:920});
  state=initial(); await page.locator('#refresh').click();
  await pause(200);
  await mkdir(new URL('build/',root),{recursive:true});
  await page.screenshot({path:fileURLToPath(new URL('build/webremote-mobile-new.png',root)),fullPage:true});
  await page.setViewportSize({width:1200,height:900});
  await page.screenshot({path:fileURLToPath(new URL('build/webremote-desktop-new.png',root)),fullPage:true});
  check(errors.length===0,'no browser exceptions: '+errors.join(', '));
  console.log(checks+' browser UX checks passed');
} finally {await browser.close();clearInterval(pushTimer);for(const res of streams)res.destroy();server.close();}
