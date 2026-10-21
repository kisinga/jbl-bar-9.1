// Run with Node and Playwright installed; PLAYWRIGHT_MODULE can point to its index.mjs.
import assert from 'node:assert/strict';
import {createServer} from 'node:http';
import {readFile, mkdir} from 'node:fs/promises';
import {fileURLToPath} from 'node:url';
const {chromium} = await import(process.env.PLAYWRIGHT_MODULE || 'playwright');
const root = new URL('../', import.meta.url);
const pause = ms => new Promise(resolve => setTimeout(resolve, ms));
const systemFixture = () => ({sample:1,cpu_percent:18.2,cpu_cores:2,load_average:[0.21,0.14,0.09],memory:{total_bytes:268435456,available_bytes:146800640,used_bytes:121634816},storage:{total_bytes:134217728,available_bytes:100663296,used_bytes:33554432},network:{receive_bytes_per_second:16384,transmit_bytes_per_second:1536},uptime_seconds:90300,processes:64,temperature_c:46.5});
const eqFixture = () => ({available:true,enabled:false,gains_db:[0,0,0,0,0],frequencies_hz:[125,500,2000,4000,8000],status:'original'});
const initial = () => ({eq:eqFixture(),system:systemFixture(),available:true, power:'On', mode:'Standard', volume:12, muted:false, input:'TV / ARC', source:6, source_state:0, hostname:'jbl.local', csrf:'fixture', uptime:90300, watcher:'STANDARD_THIS_WAKE', sound:{bass:0,atmos:1,rear:2,rear_docked:true,night:false}, wifi:{available:true,connected:true,ssid:'Example Wi-Fi',ip:'192.168.50.1',signal_dbm:-48,frequency_mhz:5180}, bluetooth:{selected:false,connectable:false,discoverable:false,connected:false,media_ready:true,audio:'Disconnected',name:'My speaker'}});
let state = initial(), calls = [], lastPayload = null, delay = 300, failure = '', offline = false, getDelay = 0, advertisedControls = null;
const streams=new Set(); let stateGets=0, streamGets=0, revision=0, lastSnapshot='';
function snapshot() {
 state.media_controls=[];
 if(state.power==='On' && state.input==='Bluetooth' && state.bluetooth?.connected && state.bluetooth.media_ready)state.media_controls=['media_previous','media_play','media_pause','media_next','media_stop'];
 if(state.power==='On' && state.source===14 && state.source_state===7 && state.streaming?.service==='AirPlay' && state.streaming.active)state.media_controls=['media_previous','media_play','media_pause','media_next','media_stop'];
 const c=state.streaming?.cast;
 if(state.power==='On' && state.input==='Chromecast' && c?.has_media && !c.mirroring){
  if(c.supported_commands&64)state.media_controls.push('media_next');
  if(c.supported_commands&128)state.media_controls.push('media_previous');
  if(c.player_state==='PAUSED')state.media_controls.push('media_play');
  if(c.player_state==='PLAYING' && c.supported_commands&1)state.media_controls.push('media_pause');
  state.media_controls.push('media_stop');
 }

 if(advertisedControls !== null)state.media_controls=[...advertisedControls];
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
      const payload = JSON.parse(body); lastPayload = payload; const {action} = payload; calls.push(action);
      await pause(delay);
      if (failure) { res.writeHead(409, {'Content-Type':'application/json'}); res.end(JSON.stringify({error:failure,state})); return; }
      if (action === 'eq_apply') {state.eq={...state.eq,enabled:payload.gains_db.some(value=>value!==0),gains_db:[...payload.gains_db],status:payload.gains_db.some(value=>value!==0)?'custom':'original'};}
      if (action === 'eq_reset') state.eq={...eqFixture()};
      if (action === 'night_toggle') state.sound.night=!state.sound.night;
      // A2DP can remain Streaming while the app handles AVRCP Play/Pause.
      // Only a real receiver media session has an independently reported player state.
      if (state.input==='Chromecast' && state.streaming?.cast) {
        if(action==='media_play')state.streaming.cast.player_state='PLAYING';
        if(action==='media_pause')state.streaming.cast.player_state='PAUSED';
      }
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
  check(await page.locator('h1').count()===1,'one clear page heading');
  state.automation={enabled:true,engine:'go',status:'WAITING_FOR_POWER_ON'};
  await page.waitForFunction(()=>document.querySelector('#device-automation').textContent==='Go · Waiting for power-on');
  check(true,'Go automation engine and state appear in device details');
  state.automation={enabled:true,engine:'go',status:'AUTOMATION_FAILED',error:'Automatic retry stopped'};
  await page.waitForFunction(()=>document.querySelector('#device-automation').textContent==='Automatic retry stopped');
  check(true,'automation failure shows its reason instead of claiming Standard selected');
  delete state.automation;
  await page.waitForFunction(()=>document.querySelector('#device-automation').textContent==='Standard selected for this wake');

  check(await page.locator('#eq-panel h2').textContent()==='Front EQ','front-channel equalizer is explicit');
  check((await page.locator('#eq-panel').textContent()).includes('Experimental') && (await page.locator('#eq-processing').textContent()).includes('custom DSP mode'),'EQ experimental processing mode is disclosed');
  check(await page.locator('#eq-panel input[type=range]').count()===5 && await page.locator('#eq-panel input').evaluateAll(inputs=>inputs.every(input=>input.min==='-6'&&input.max==='0'&&input.step==='0.5')),'five real cuts-only half-decibel sliders');
  check(await page.locator('#eq-apply').isDisabled() && await page.locator('#eq-reset').isDisabled(),'unchanged original tuning offers no unnecessary write');
  check(await page.locator('#eq-preset').inputValue()==='original','committed factory tuning selects Original preset');
  await page.locator('#eq-preset').selectOption('less_bass');
  check(calls.length===0 && state.eq.gains_db.every(value=>value===0) && await page.locator('#eq-slider-0').inputValue()==='-4' && await page.locator('#eq-slider-1').inputValue()==='-2','preset selection changes draft and graph only, without a POST');
  check(await page.locator('#eq-apply').isEnabled() && (await page.locator('#eq-graph svg').getAttribute('aria-label')).includes('Unapplied'),'preset remains explicitly unapplied');
  await page.locator('#eq-apply').click();await page.waitForFunction(()=>document.querySelector('#message-text').textContent==='Front EQ applied.');
  check(JSON.stringify(lastPayload.gains_db)==='[-4,-2,0,0,0]' && await page.locator('#eq-preset').inputValue()==='less_bass','preset Apply commits the exact named curve and keeps selection');
  await page.locator('#eq-slider-0').evaluate(input=>{input.value='-3.5';input.dispatchEvent(new Event('input',{bubbles:true}));});
  check(await page.locator('#eq-preset').inputValue()==='custom','manual tuning of a preset switches to Custom');
  await page.locator('#eq-preset').selectOption('custom');
  check(await page.locator('#eq-slider-0').inputValue()==='-3.5','selecting Custom retains the current draft');
  await page.locator('#eq-preset').selectOption('clear_dialogue');
  check(await page.locator('#eq-slider-0').inputValue()==='-3' && await page.locator('#eq-slider-1').inputValue()==='-1' && await page.locator('#eq-slider-4').inputValue()==='-1','Clear dialogue uses a cuts-only draft');
  await page.locator('#eq-preset').selectOption('softer_treble');
  check(await page.locator('#eq-slider-3').inputValue()==='-2' && await page.locator('#eq-slider-4').inputValue()==='-3','Softer treble uses the specified upper-band cuts');
  const callsBeforeMorePresets=calls.length;
  for(const [name,values] of [['warm',[0,0,-1,-2,-3]],['bright',[-3,-2,-1,0,0]],['vocal_focus',[-4,-2,0,-1,-2]]]) {
    await page.locator('#eq-preset').selectOption(name);
    check(JSON.stringify(await page.locator('#eq-panel input[type=range]').evaluateAll(inputs=>inputs.map(input=>Number(input.value))))===JSON.stringify(values) && calls.length===callsBeforeMorePresets,'additional '+name+' preset sets the exact bounded draft without sending audio commands');
  }
  await page.locator('#eq-preset').selectOption('original');
  check(await page.locator('#eq-apply').isEnabled() && state.eq.enabled,'Original remains a draft until Apply when custom EQ is active');
  await page.locator('#eq-apply').click();await page.waitForFunction(()=>document.querySelector('#message-text').textContent==='Original tuning restored.');
  check(!state.eq.enabled && lastPayload.gains_db.every(value=>value===0) && await page.locator('#eq-preset').inputValue()==='original','applying Original restores native processing');
  calls=[];lastPayload=null;
  await page.locator('#eq-slider-0').focus();await page.keyboard.press('ArrowLeft');
  check(await page.locator('#eq-gain-0').textContent()==='-0.5 dB' && calls.length===0 && state.eq.gains_db[0]===0,'keyboard EQ edits a local draft without changing sound or volume');
  check(await page.locator('#eq-apply').isEnabled() && (await page.locator('#eq-state').textContent()).includes('Unapplied'),'draft requires explicit Apply');
  state.system.sample=2;await pause(100);
  check(await page.locator('#eq-slider-0').inputValue()==='-0.5','ordinary live updates preserve the EQ draft');
  await page.locator('#eq-apply').click();
  check(await page.locator('#eq-slider-0').isDisabled() && await page.locator('#eq-preset').isDisabled() && await page.locator('[data-action=volume_up]').isDisabled(),'EQ presets and sliders share one global pending action guard');
  await page.waitForFunction(()=>document.querySelector('#message-text').textContent==='Front EQ applied.');
  check(lastPayload.action==='eq_apply' && JSON.stringify(lastPayload.gains_db)==='[-0.5,0,0,0,0]' && state.eq.enabled,'Apply sends only the captured five-band draft');
  check(await page.locator('#eq-apply').isDisabled() && await page.locator('#eq-reset').isEnabled(),'applied EQ enables original-processing reset');
  await page.locator('#eq-reset').click();await page.waitForFunction(()=>document.querySelector('#message-text').textContent==='Original tuning restored.');
  check(lastPayload.action==='eq_reset' && !('gains_db' in lastPayload) && !state.eq.enabled,'Reset requests native original processing, not a flat custom preset');
  await page.locator('#eq-slider-1').evaluate(input=>{input.value='-6';input.dispatchEvent(new Event('input',{bubbles:true}));});
  await page.locator('#eq-apply').click();await page.waitForFunction(()=>document.querySelector('#message-text').textContent==='Front EQ applied.');
  await page.locator('#eq-slider-1').evaluate(input=>{input.value='0';input.dispatchEvent(new Event('input',{bubbles:true}));});
  await page.locator('#eq-apply').click();await page.waitForFunction(()=>document.querySelector('#message-text').textContent==='Original tuning restored.');
  check(!state.eq.enabled && state.eq.gains_db.every(value=>value===0),'applying all zero gains restores original processing');
  await page.locator('#eq-slider-2').evaluate(input=>{input.value='-1';input.dispatchEvent(new Event('input',{bubbles:true}));});
  failure='EQ application refused';await page.locator('#eq-apply').click();await page.waitForFunction(()=>document.querySelector('#message').dataset.kind==='error');
  check(!state.eq.enabled && await page.locator('#eq-slider-2').inputValue()==='-1' && await page.locator('#eq-apply').isEnabled(),'refused EQ preserves the unapplied draft without claiming success');
  failure='';state.eq={...eqFixture(),enabled:true,gains_db:[-2,0,0,0,0],status:'custom'};await page.waitForFunction(()=>document.querySelector('#eq-slider-0').value==='-2');
  check(await page.locator('#eq-slider-2').inputValue()==='0','new committed soundbar EQ replaces an obsolete local draft');
  state.eq={available:false,enabled:true,gains_db:[-2,0,0,0,0],status:'reset_required'};await page.waitForFunction(()=>document.querySelector('#eq-slider-0').disabled);
  check(await page.locator('#eq-reset').isEnabled() && await page.locator('#eq-apply').isDisabled() && (await page.locator('#eq-state').textContent()).includes('Reset required'),'reset-required state permits recovery but blocks further tuning');
  state.eq={...eqFixture(),gains_db:[1,0,0,0,0]};await page.waitForFunction(()=>document.querySelector('#eq-gain-0').textContent==='—');
  check(await page.locator('#eq-slider-0').isDisabled() && await page.locator('#eq-preset').isDisabled() && await page.locator('#eq-graph').isHidden(),'invalid boost readback disables presets rather than showing a false setting');
  delete state.eq;await pause(100);
  check(await page.locator('#eq-apply').isDisabled() && await page.locator('#eq-reset').isDisabled() && (await page.locator('#eq-state').textContent()).includes('unavailable'),'older backend has no pretend active EQ controls');
  state.eq=eqFixture();state.system=systemFixture();calls=[];lastPayload=null;await pause(100);

  check(await page.locator('.panel').first().evaluate(e=>getComputedStyle(e).animationName)==='none','reduced-motion preference removes entry animation');
  check(await page.locator('#volume').evaluate(e=>Number.parseFloat(getComputedStyle(e).fontSize))>await page.locator('#sound-heading').evaluate(e=>Number.parseFloat(getComputedStyle(e).fontSize)),'volume remains the primary visual control');

  check(await page.locator('#system-heading').textContent()==='System monitor','system monitor groups device vitals');
  check(await page.locator('#system-panel #wifi-panel').count()===1 && await page.locator('#wifi-panel.panel').count()===0,'Wi-Fi lives inside system monitor');
  check(await page.locator('#cpu-value').textContent()==='18%' && await page.locator('#cpu-detail').textContent()==='2 cores','real aggregate CPU and core count shown');
  check(await page.locator('#memory-value').textContent()==='45%' && (await page.locator('#memory-detail').textContent()).includes('256 MiB'),'memory used and total shown');
  check(await page.locator('#storage-value').textContent()==='96 MiB free of 128 MiB' && await page.locator('#storage-progress').getAttribute('value')==='25','persistent storage usage shown');
  check(await page.locator('#network-receive').textContent()==='16 KiB/s' && await page.locator('#network-transmit').textContent()==='1.5 KiB/s','real Wi-Fi byte rates shown');
  check(await page.locator('#system-load').textContent()==='0.21 / 0.14 / 0.09' && await page.locator('#system-uptime').textContent()==='1d 1h 5m','load intervals and uptime readable');
  check(await page.locator('#system-temperature').textContent()==='46.5 °C' && await page.locator('#system-processes').textContent()==='64','optional temperature and process count shown');
  check(await page.locator('#cpu-chart').getAttribute('data-samples')==='1','first server sample starts real history');
  await page.evaluate(()=>{render();render();});
  check(await page.locator('#cpu-chart').getAttribute('data-samples')==='1','renders and duplicate action snapshots never fabricate history');
  for(let n=2;n<=4;n++){state.system={...state.system,sample:n,cpu_percent:n*10};await page.waitForFunction(count=>document.querySelector('#cpu-chart').dataset.samples===String(count),n);}
  check(await page.locator('#cpu-line').getAttribute('d')!=='' && (await page.locator('#cpu-chart').getAttribute('aria-label')).includes('4 real samples'),'bounded SVG graph has accessible real sample history');
  state.system={sample:5,cpu_percent:null,memory:{total_bytes:100,available_bytes:200,used_bytes:-1},storage:{total_bytes:0,available_bytes:0,used_bytes:0},network:{receive_bytes_per_second:null,transmit_bytes_per_second:-1},load_average:[0,null,0]};
  await page.waitForFunction(()=>document.querySelector('#cpu-value').textContent==='—');
  check(await page.locator('#memory-value').textContent()==='—' && await page.locator('#network-receive').textContent()==='—' && await page.locator('#network-transmit').textContent()==='—','missing and invalid vitals never become invented zeros');
  check(await page.locator('#storage-progress').isHidden() && await page.locator('#system-temperature-row').isHidden() && await page.locator('#system-processes-row').isHidden(),'unsupported readings and invalid storage stay unavailable');
  check(await page.locator('#cpu-chart').getAttribute('data-samples')==='4','unavailable metric creates a history gap, not a zero');
  state.system={...systemFixture(),sample:6,cpu_percent:0,memory:{...systemFixture().memory,estimated_available:true}};
  await page.waitForFunction(()=>document.querySelector('#cpu-value').textContent==='0%');
  check((await page.locator('#memory-available').textContent()).includes('estimated'),'fallback available-memory estimate is labelled');
  check((await page.locator('#cpu-line').getAttribute('d')).split('M').length===3,'missing CPU sample breaks graph into separate segments');
  await page.evaluate(()=>{const real=Date.now;try{Date.now=()=>real()+7000;render();}finally{Date.now=real;}});
  check(await page.locator('#system-panel').getAttribute('data-state')==='stale' && (await page.locator('#system-note').textContent()).includes('stopped updating'),'stalled samples are marked stale');
  await page.evaluate(()=>renderSystem({...current,available:false,system:{...current.system,sample:7}},true));
  check(await page.locator('#system-panel').getAttribute('data-state')==='live' && await page.locator('#cpu-value').textContent()==='0%','system monitor remains usable while native controls are starting');
  await page.evaluate(()=>{const real=Date.now;try{Date.now=()=>real()+7000;renderSystem({...current,system:{...current.system,sample:8}},true);}finally{Date.now=real;}});
  check(await page.locator('#cpu-chart').getAttribute('data-samples')==='1','fresh sample after a long gap starts honest history');
  delete state.system;await page.waitForFunction(()=>document.querySelector('#system-panel').dataset.state==='waiting');
  check(await page.locator('#cpu-chart').getAttribute('data-samples')==='0' && await page.locator('#cpu-value').textContent()==='—','missing backend clears values and historical chart');
  state.system=systemFixture();await page.waitForFunction(()=>document.querySelector('#cpu-chart').dataset.samples==='1');
  // Feed distinct backend samples without waiting for a two-minute real-device capture.
  await page.evaluate(()=>{for(let sample=2;sample<=70;sample++)renderSystem({...current,system:{...current.system,sample,cpu_percent:sample%100}},true);});
  check(await page.locator('#cpu-chart').getAttribute('data-samples')==='60','history retains at most 60 unique samples');
  await page.evaluate(()=>renderSystem({...current,system:{...current.system,sample:1}},true));
  check(await page.locator('#cpu-chart').getAttribute('data-samples')==='1','backend sample reset starts a new history');

  const stale={...snapshot(),revision:revision-1,volume:200};
  for(const res of streams)res.write('data: '+JSON.stringify(stale)+'\n\n');
  await pause(100);
  check((await page.locator('#volume').textContent())==='12','stale pushed states cannot rewind the UI');
  check((await page.locator('#wifi-name').textContent())==='Example Wi-Fi','Wi-Fi network shown');
  check(await page.locator('#local-address').isVisible(),'confirmed local hostname shown');
  check((await page.locator('#local-address').getAttribute('href'))==='http://jbl.local','local address uses advertised alias');
  check((await page.locator('#wifi-status').textContent()).includes('Strong signal'),'Wi-Fi quality label');
  await page.locator('.device-details summary').click();
  check((await page.locator('#device-uptime').textContent())==='1d 1h 5m','uptime shown in readable units');
  check((await page.locator('#device-automation').textContent())==='Standard selected for this wake','automation status explained');
  await page.locator('.device-details summary').click();
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
  check(await page.locator('#eq-slider-0').isDisabled() && await page.locator('#eq-preset').isDisabled() && await page.locator('#eq-apply').isDisabled() && await page.locator('#eq-reset').isDisabled(),'offline disables EQ presets, drafts and mutations');
  check(await page.locator('#cpu-chart').getAttribute('data-samples')==='0' && await page.locator('#system-panel').getAttribute('data-state')==='offline' && await page.locator('#memory-value').textContent()==='—','offline clears graphs and readings rather than displaying false zeros');
  const atLoss=calls.length; offline=false;
  await page.locator('#refresh').click();
  await page.waitForFunction(()=>document.querySelector('#connection').dataset.state==='online');
  await pause(600);
  check(calls.length===atLoss,'reconnect never replays controls');
  state.power='Standby'; await page.locator('#refresh').click();
  await page.waitForFunction(()=>document.querySelector('#power').getAttribute('aria-label')==='Turn soundbar on');
  check(await plus.isDisabled() && await page.locator('#power').isEnabled(),'standby only permits power');
  check(await page.locator('#eq-slider-0').isDisabled() && (await page.locator('#eq-state').textContent()).includes('Turn on'),'standby blocks EQ and explains why');
  check((await page.locator('#summary').textContent()).includes('standby'),'standby copy');
  state.available=false;state.power='Starting';state.error='Soundbar is starting';
  await page.locator('#refresh').click();
  await page.waitForFunction(()=>document.querySelector('#connection-label').textContent==='Starting');
  check(await page.locator('#power').isDisabled(),'starting firmware disables power');
  state=initial(); await page.locator('#refresh').click();
  await page.waitForFunction(()=>document.querySelector('#power').getAttribute('aria-label')==='Put soundbar in standby');
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
  check((await page.locator('#playback-status').textContent())==='Streaming audio','Bluetooth status describes the audio stream');
  check((await page.locator('#playback-title').textContent())==='Example phone','Bluetooth playback identifies peer');
  check(await page.locator('#media-toggle').getAttribute('data-action')==='media_pause','streaming Bluetooth initially offers Pause');
  check(await page.locator('[data-action=media_play],[data-action=media_pause]').count()===1,'one shared Play/Pause button');
  await page.locator('#media-toggle').click();
  await page.waitForFunction(()=>document.querySelector('#message-text').textContent==='Pause requested.');
  check(calls.at(-1)==='media_pause' && state.bluetooth.audio==='Streaming','Pause is requested without inventing an app or stream state');
  check((await page.locator('#playback-status').textContent())==='Streaming audio','A2DP can stay active after Pause');
  check((await page.locator('#media-help').textContent()).includes('while an app is paused'),'stream activity limitation is explained');
  check(await page.locator('#media-toggle').getAttribute('aria-label')==='Play','successful Pause changes the next requested action to Play');
  const callsBeforePlaybackReconnect=calls.length;
  offline=true;
  await page.waitForFunction(()=>document.querySelector('#connection').dataset.state==='offline');
  check(await page.locator('#media-toggle').isDisabled(),'connection loss disables playback controls');
  offline=false;
  await page.waitForFunction(()=>document.querySelector('#connection').dataset.state==='online' && !document.querySelector('#media-toggle').disabled);
  check(await page.locator('#media-toggle').getAttribute('aria-label')==='Pause' && calls.length===callsBeforePlaybackReconnect,'reconnection clears stale playback intent without replaying commands');
  await page.locator('#media-toggle').click();
  await page.waitForFunction(()=>document.querySelector('#media-toggle').dataset.action==='media_play' && !document.querySelector('#media-toggle').disabled);
  await page.locator('#media-toggle').click();
  await page.waitForFunction(()=>document.querySelector('#message-text').textContent==='Play requested.');
  check(calls.at(-1)==='media_play' && state.bluetooth.audio==='Streaming','Play reports a request without claiming app confirmation');
  check(await page.locator('#media-toggle').getAttribute('aria-label')==='Pause','successful Play restores Pause');
  failure='playback request failed';
  await page.locator('#media-toggle').click();
  await page.waitForFunction(()=>document.querySelector('#message').dataset.kind==='error');
  check(await page.locator('#media-toggle').getAttribute('aria-label')==='Pause','failed Pause never flips intent');
  failure='';
  state.bluetooth.audio='Paused';
  await page.waitForFunction(()=>document.querySelector('#playback-status').textContent==='Stream suspended');
  check(await page.locator('#playback').isVisible() && (await page.locator('#bt-status').textContent())==='Stream suspended','suspended Bluetooth stream stays visible');
  check(!await page.locator('#playback').evaluate(e=>e.classList.contains('active')),'suspended stream stops the activity indicator');
  check(await page.locator('#media-toggle').getAttribute('aria-label')==='Play','actual stream transition resets pending playback intent');
  advertisedControls=['media_pause'];
  await page.waitForFunction(()=>document.querySelector('#media-toggle').dataset.action==='media_pause');
  check(await page.locator('#media-toggle').isEnabled(),'single transport follows server capabilities');
  advertisedControls=null;
  await page.waitForFunction(()=>document.querySelector('#media-toggle').dataset.action==='media_play');
  await page.locator('#media-toggle').click();
  await page.waitForFunction(()=>document.querySelector('#message-text').textContent==='Play requested.');
  check(await page.locator('#media-toggle').getAttribute('aria-label')==='Pause','request intent works while stream remains suspended');
  state.bluetooth.peer_name='Another example phone';
  await page.waitForFunction(()=>document.querySelector('#playback-title').textContent==='Another example phone');
  check(await page.locator('#media-toggle').getAttribute('aria-label')==='Play','changing peers clears previous playback intent');
  await page.locator('#media-toggle').click();
  await page.waitForFunction(()=>document.querySelector('#message-text').textContent==='Play requested.');
  state.input='TV / ARC';
  await page.waitForFunction(()=>document.querySelector('#playback').hidden);
  state.input='Bluetooth';
  await page.waitForFunction(()=>!document.querySelector('#playback').hidden);
  check(await page.locator('#media-toggle').getAttribute('aria-label')==='Play','leaving and returning to Bluetooth clears previous playback intent');

  await page.locator('#media-next').click();
  await page.waitForFunction(()=>document.querySelector('#message-text').textContent==='Next track requested.');
  check(calls.at(-1)==='media_next','next reports requested rather than confirmed track');
  await page.locator('#media-previous').click();
  await page.waitForFunction(()=>document.querySelector('#message-text').textContent==='Previous track requested.');
  check(calls.at(-1)==='media_previous','previous uses explicit command');
  state.bluetooth.audio='Streaming';
  await page.waitForFunction(()=>document.querySelector('#media-toggle').dataset.action==='media_pause');
  await page.locator('#media-stop').click();
  await page.waitForFunction(()=>document.querySelector('#message-text').textContent==='Stop requested.');
  check(calls.at(-1)==='media_stop','stop request acknowledged');
  check(state.bluetooth.audio==='Streaming' && await page.locator('#media-toggle').getAttribute('aria-label')==='Play','successful Stop offers Play without inventing a stream transition');
  state.bluetooth.media_ready=false;
  await page.waitForFunction(()=>document.querySelector('#media-toggle').disabled);
  check((await page.locator('.transport button:disabled').count())===4,'missing AVRCP readiness disables every transport control');
  const beforeUnavailable=calls.length;
  await page.locator('#media-toggle').evaluate(button=>button.click());
  await pause(100);
  check(calls.length===beforeUnavailable,'unavailable AVRCP cannot enqueue a command');
  state.bluetooth.media_ready=true;
  state.bluetooth.audio='Connected';
  await page.waitForFunction(()=>document.querySelector('#playback-status').textContent==='Connected');
  check(await page.locator('#playback').isVisible(),'connected state does not imply playing');
  state.bluetooth.connected=false;state.bluetooth.audio='Disconnected';
  await page.waitForFunction(()=>document.querySelector('#playback-status').textContent==='Waiting for a device');
  check(await page.locator('#playback-title').isHidden(),'disconnected peer name clears');
  check((await page.locator('.transport button:disabled').count())===4,'disconnected device cannot receive any controls');
  state.bluetooth.connected=null;
  await page.waitForFunction(()=>document.querySelector('#playback-status').textContent==='Status unavailable');
  check(await page.locator('#playback').isVisible(),'unknown Bluetooth state is explicit');
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
  await page.locator('#night').click();
  await page.waitForFunction(()=>document.querySelector('#night-value').textContent==='On');
  check(await page.locator('#night').getAttribute('aria-pressed')==='true','night mode shows live on state');
  await page.locator('#night').click();
  await page.waitForFunction(()=>document.querySelector('#night-value').textContent==='Off');
  check(!state.sound.night,'night mode can return off');
  delete state.sound.night;
  await page.waitForFunction(()=>document.querySelector('#night').disabled);
  check((await page.locator('#night-value').textContent())==='Unavailable','missing night readback stays unavailable');
  state.input='AirPlay';state.source=14;state.source_state=7;state.streaming={service:'AirPlay',status:'Session active',active:true};
  await page.waitForFunction(()=>document.querySelector('#playback-service').textContent==='AirPlay');
  check((await page.locator('#playback-status').textContent())==='Session active','AirPlay updates via push without refresh');
  check(await page.locator('#media-toggle').getAttribute('aria-label')==='Pause' && await page.locator('#media-next').isEnabled(),'active AirPlay exposes direct sender controls');
  await page.locator('#media-toggle').click();
  await page.waitForFunction(()=>document.querySelector('#media-toggle').dataset.action==='media_play' && !document.querySelector('#media-toggle').disabled);
  check(calls.at(-1)==='media_pause' && await page.locator('#playback-status').textContent()==='Session active','AirPlay pause intent never invents sender playback feedback');
  check((await page.locator('#media-help').textContent()).includes('isn’t reported'),'AirPlay sender feedback limit is explicit');
  await page.locator('#media-toggle').click();
  await page.waitForFunction(()=>document.querySelector('#media-toggle').dataset.action==='media_pause' && !document.querySelector('#media-toggle').disabled);
  check(calls.at(-1)==='media_play','AirPlay single transport alternates requested Play and Pause');
  state.streaming.active=false;
  await page.waitForFunction(()=>document.querySelector('#media-toggle').disabled);
  check(await page.locator('#media-next').isDisabled(),'ended AirPlay sessions disable sender controls');
  state.streaming.active=true;
  await page.waitForFunction(()=>!document.querySelector('#media-toggle').disabled);
  check(await page.locator('#media-toggle').getAttribute('aria-label')==='Pause','new AirPlay session clears earlier playback intent');
  await page.locator('#media-toggle').click();
  await page.waitForFunction(()=>document.querySelector('#media-toggle').dataset.action==='media_play' && !document.querySelector('#media-toggle').disabled);
  state.power='Standby';
  await page.waitForFunction(()=>document.querySelector('#playback').hidden);
  state.power='On';
  await page.waitForFunction(()=>!document.querySelector('#playback').hidden && !document.querySelector('#media-toggle').disabled);
  check(await page.locator('#media-toggle').getAttribute('aria-label')==='Pause','standby clears AirPlay playback intent even if session status stays active');
  state.input='Chromecast';state.streaming={service:'Chromecast',status:'Paused',active:false};
  await page.waitForFunction(()=>document.querySelector('#playback-service').textContent==='Chromecast');
  check((await page.locator('#playback-status').textContent())==='Paused','Chromecast pause state shown');
  state.streaming={service:'Chromecast',status:'Audio mirroring',active:true,detail:'The sending app’s play/pause state isn’t reported by mirrored audio.',cast:{available:true,application:'Chrome Audio Mirroring',mirroring:true,player_state:'PLAYING',stream_type:'LIVE'}};
  await page.waitForFunction(()=>document.querySelector('#playback-status').textContent==='Audio mirroring');
  check((await page.locator('#playback-detail').textContent()).includes('isn’t reported'),'mirroring limitation explained');
  check((await page.locator('#playback-app').textContent())==='Chrome Audio Mirroring','receiver application shown');
  check(await page.locator('#playback-timeline').isHidden(),'mirroring never invents a track timeline');
  check((await page.locator('.transport button:disabled').count())===4,'mirrored sender controls stay disabled');
  state.streaming={service:'Chromecast',status:'Paused',active:false,cast:{available:true,application:'Music app',mirroring:false,has_media:true,supported_commands:129,player_state:'PAUSED',title:'<img src=x onerror=alert(1)> '+ 'Long title '.repeat(30),artist:'Example artist',album:'Example album',current_time:75,duration:240,low_shelf_gain:0,high_shelf_gain:-2}};
  await page.waitForFunction(()=>document.querySelector('#playback-time').textContent==='1:15 / 4:00');
  check((await page.locator('#playback-meta').textContent())==='Example artist · Example album','Cast artist and album shown');
  check(await page.locator('#media-toggle').isEnabled() && await page.locator('#media-toggle').getAttribute('aria-label')==='Play' && await page.locator('#media-previous').isEnabled() && await page.locator('#media-next').isDisabled(),'paused Cast enables only server-advertised Play and Previous');
  state.streaming.cast.player_state='PLAYING';state.streaming.cast.supported_commands=65;
  await page.waitForFunction(()=>document.querySelector('#media-toggle').dataset.action==='media_pause');
  check(await page.locator('#media-toggle').isEnabled() && await page.locator('#media-previous').isDisabled() && await page.locator('#media-next').isEnabled(),'playing Cast enables only server-advertised Pause and Next');
  await page.locator('#media-toggle').click();
  await page.waitForFunction(()=>document.querySelector('#message-text').textContent==='Pause requested.');
  check(calls.at(-1)==='media_pause' && await page.locator('#media-toggle').isEnabled() && await page.locator('#media-toggle').getAttribute('aria-label')==='Play','Cast pause request follows refreshed receiver capabilities');
  state.streaming.cast.player_state='PLAYING';state.streaming.cast.supported_commands=0;
  await page.waitForFunction(()=>document.querySelector('#media-toggle').disabled && document.querySelector('#media-next').disabled);
  check(await page.locator('#media-toggle').isDisabled() && await page.locator('#media-next').isDisabled(),'Cast without pause or skip capabilities keeps those commands disabled');
  check((await page.locator('#playback-title img').count())===0,'remote metadata is rendered as text');
  check(await page.locator('#playback-progress').getAttribute('value')==='75','timeline uses receiver position');
  await page.locator('.device-details summary').click();
  check((await page.locator('#cast-eq').textContent())==='Low 0 dB · High -2 dB (read only)','Cast tone readback shown without unsafe controls');
  for (const width of [320,390,1024]) {
    await page.setViewportSize({width,height:900});
    check(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),'long media metadata fits at '+width);
    check(await page.locator('.transport button').evaluateAll(buttons=>buttons.every(button=>{const r=button.getBoundingClientRect();return r.width>=44 && r.height>=44 && r.left>=0 && r.right<=innerWidth;})),'four 44px transport targets fit at '+width);
  }
  await page.locator('.device-details summary').click();
  const beforeIdle=stateGets;await pause(1200);
  check(stateGets===beforeIdle,'idle browser never polls the state endpoint');
  state=initial();await page.locator('#refresh').click();await pause(200);
  for (const width of [320,390,1024]) {
    await page.setViewportSize({width,height:900});
    check(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),'no horizontal overflow at '+width);
    check(await page.locator('button,summary').evaluateAll(elements=>elements.filter(e=>e.getBoundingClientRect().height>0).every(e=>e.getBoundingClientRect().height>=44)),'44px touch height for all visible controls at '+width);
    if(width>=1000) check(await page.locator('.primary').evaluate(e=>e.getBoundingClientRect().right)<=await page.locator('.secondary').evaluate(e=>e.getBoundingClientRect().left),'listening and sound controls form separate desktop columns');

  }
  await page.setViewportSize({width:320,height:920});
  state=initial();state.input='Bluetooth';state.bluetooth={...state.bluetooth,selected:true,connected:true,audio:'Streaming',peer_name:'Example phone'}; await page.locator('#refresh').click();
  await pause(200);
  await mkdir(new URL('build/',root),{recursive:true});
  check(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),'Bluetooth transport fits at 320px');
  // Public screenshots contain explicit sample fixtures, never live device captures.
  await page.evaluate(()=>{
    const realNow=Date.now, end=realNow();
    renderSystem({...current,system:null},true);
    try {
      for(let sample=1;sample<=60;sample++) {
        Date.now=()=>end-(60-sample)*2000;
        renderSystem({...current,system:{...current.system,sample,cpu_percent:18+Math.sin(sample*.34)*8+(sample%13===0?19:0),memory:{...current.system.memory,used_bytes:118000000+Math.sin(sample*.12)*4000000,available_bytes:150435456-Math.sin(sample*.12)*4000000}}},true);
      }
    } finally {Date.now=realNow;}
  });
  await page.screenshot({path:fileURLToPath(new URL('build/webremote-mobile-320.png',root)),fullPage:true});
  await page.setViewportSize({width:390,height:920});
  await page.screenshot({path:fileURLToPath(new URL('build/webremote-mobile-new.png',root)),fullPage:true});
  await page.setViewportSize({width:1200,height:900});
  await page.screenshot({path:fileURLToPath(new URL('build/webremote-desktop-new.png',root)),fullPage:true});
  await page.emulateMedia({reducedMotion:'no-preference'});
  check(await page.locator('.panel').first().evaluate(e=>getComputedStyle(e).animationName)==='arrive','motion enabled only when preference permits');
  check(errors.length===0,'no browser exceptions: '+errors.join(', '));
  console.log(checks+' browser UX checks passed');
} finally {await browser.close();clearInterval(pushTimer);for(const res of streams)res.destroy();server.close();}
