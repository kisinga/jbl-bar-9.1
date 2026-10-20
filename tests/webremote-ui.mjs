// Run with Node and Playwright installed; PLAYWRIGHT_MODULE can point to its index.mjs.
import assert from 'node:assert/strict';
import {createServer} from 'node:http';
import {readFile, mkdir} from 'node:fs/promises';
import {fileURLToPath} from 'node:url';
const {chromium} = await import(process.env.PLAYWRIGHT_MODULE || 'playwright');
const root = new URL('../', import.meta.url);
const pause = ms => new Promise(resolve => setTimeout(resolve, ms));
const initial = () => ({available:true, power:'On', mode:'Standard', volume:12, muted:false, input:'TV / ARC', source:6, source_state:0, hostname:'jbl.local', csrf:'fixture', uptime:90300, watcher:'STANDARD_THIS_WAKE', sound:{bass:0,atmos:1,rear:2,rear_docked:true,night:false}, wifi:{available:true,connected:true,ssid:'Example Wi-Fi',ip:'192.168.50.1',signal_dbm:-48,frequency_mhz:5180}, bluetooth:{selected:false,connectable:false,discoverable:false,connected:false,media_ready:true,audio:'Disconnected',name:'My speaker'}});
let state = initial(), calls = [], delay = 300, failure = '', offline = false, getDelay = 0, advertisedControls = null;
const streams=new Set(); let stateGets=0, streamGets=0, revision=0, lastSnapshot='';
function snapshot() {
 state.media_controls=[];
 if(state.power==='On' && state.input==='Bluetooth' && state.bluetooth?.connected && state.bluetooth.media_ready)state.media_controls=['media_previous','media_play','media_pause','media_next','media_stop'];
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
      const {action} = JSON.parse(body); calls.push(action);
      await pause(delay);
      if (failure) { res.writeHead(409, {'Content-Type':'application/json'}); res.end(JSON.stringify({error:failure,state})); return; }
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
  check((await page.locator('#playback-status').textContent())==='Streaming audio','Bluetooth status describes the audio stream');
  check((await page.locator('#playback-title').textContent())==='Example phone','Bluetooth playback identifies peer');
  check(await page.locator('#media-toggle').getAttribute('data-action')==='media_play' && await page.locator('#media-pause').getAttribute('data-action')==='media_pause','Play and Pause are separate fixed commands');
  check(await page.locator('#media-toggle').isEnabled() && await page.locator('#media-pause').isEnabled(),'connected Bluetooth offers both Play and Pause');
  await page.locator('#media-pause').click();
  await page.waitForFunction(()=>document.querySelector('#message-text').textContent==='Pause requested.');
  check(calls.at(-1)==='media_pause' && state.bluetooth.audio==='Streaming','Pause is requested without inventing an app or stream state');
  check((await page.locator('#playback-status').textContent())==='Streaming audio','A2DP can stay active after Pause');
  check((await page.locator('#media-help').textContent()).includes('while an app is paused'),'stream activity limitation is explained');
  check(await page.locator('#media-toggle').isEnabled() && await page.locator('#media-pause').isEnabled(),'Play remains available even when A2DP stays Streaming');
  await page.locator('#media-toggle').click();
  await page.waitForFunction(()=>document.querySelector('#message-text').textContent==='Play requested.');
  check(calls.at(-1)==='media_play' && state.bluetooth.audio==='Streaming','Play reports a request without claiming app confirmation');
  state.bluetooth.audio='Paused';
  await page.waitForFunction(()=>document.querySelector('#playback-status').textContent==='Stream suspended');
  check(await page.locator('#playback').isVisible() && (await page.locator('#bt-status').textContent())==='Stream suspended','suspended Bluetooth stream stays visible');
  check(!await page.locator('#playback').evaluate(e=>e.classList.contains('active')),'suspended stream stops the activity indicator');
  check(await page.locator('#media-toggle').isEnabled() && await page.locator('#media-pause').isEnabled(),'suspended stream does not infer or hide transport commands');
  advertisedControls=['media_pause'];
  await page.waitForFunction(()=>document.querySelector('#media-toggle').disabled);
  check(await page.locator('#media-pause').isEnabled(),'availability follows server flags rather than Bluetooth stream state');
  advertisedControls=null;
  await page.waitForFunction(()=>!document.querySelector('#media-toggle').disabled);
  await page.locator('#media-next').click();
  await page.waitForFunction(()=>document.querySelector('#message-text').textContent==='Next track requested.');
  check(calls.at(-1)==='media_next','next reports requested rather than confirmed track');
  await page.locator('#media-previous').click();
  await page.waitForFunction(()=>document.querySelector('#message-text').textContent==='Previous track requested.');
  check(calls.at(-1)==='media_previous','previous uses explicit command');
  await page.locator('#media-stop').click();
  await page.waitForFunction(()=>document.querySelector('#message-text').textContent==='Stop requested.');
  check(calls.at(-1)==='media_stop','stop request acknowledged');
  state.bluetooth.media_ready=false;
  await page.waitForFunction(()=>document.querySelector('#media-toggle').disabled);
  check((await page.locator('.transport button:disabled').count())===5,'missing AVRCP readiness disables every transport control');
  const beforeUnavailable=calls.length;
  await page.locator('#media-pause').evaluate(button=>button.click());
  await pause(100);
  check(calls.length===beforeUnavailable,'unavailable AVRCP cannot enqueue a command');
  state.bluetooth.media_ready=true;
  state.bluetooth.audio='Connected';
  await page.waitForFunction(()=>document.querySelector('#playback-status').textContent==='Connected');
  check(await page.locator('#playback').isVisible(),'connected state does not imply playing');
  state.bluetooth.connected=false;state.bluetooth.audio='Disconnected';
  await page.waitForFunction(()=>document.querySelector('#playback-status').textContent==='Waiting for a device');
  check(await page.locator('#playback-title').isHidden(),'disconnected peer name clears');
  check((await page.locator('.transport button:disabled').count())===5,'disconnected device cannot receive any controls');
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
  state.input='AirPlay';state.source_state=7;state.streaming={service:'AirPlay',status:'Session active',active:true};
  await page.waitForFunction(()=>document.querySelector('#playback-service').textContent==='AirPlay');
  check((await page.locator('#playback-status').textContent())==='Session active','AirPlay updates via push without refresh');
  state.input='Chromecast';state.streaming={service:'Chromecast',status:'Paused',active:false};
  await page.waitForFunction(()=>document.querySelector('#playback-service').textContent==='Chromecast');
  check((await page.locator('#playback-status').textContent())==='Paused','Chromecast pause state shown');
  state.streaming={service:'Chromecast',status:'Audio mirroring',active:true,detail:'The sending app’s play/pause state isn’t reported by mirrored audio.',cast:{available:true,application:'Chrome Audio Mirroring',mirroring:true,player_state:'PLAYING',stream_type:'LIVE'}};
  await page.waitForFunction(()=>document.querySelector('#playback-status').textContent==='Audio mirroring');
  check((await page.locator('#playback-detail').textContent()).includes('isn’t reported'),'mirroring limitation explained');
  check((await page.locator('#playback-app').textContent())==='Chrome Audio Mirroring','receiver application shown');
  check(await page.locator('#playback-timeline').isHidden(),'mirroring never invents a track timeline');
  check((await page.locator('.transport button:disabled').count())===5,'mirrored sender controls stay disabled');
  state.streaming={service:'Chromecast',status:'Paused',active:false,cast:{available:true,application:'Music app',mirroring:false,has_media:true,supported_commands:129,player_state:'PAUSED',title:'<img src=x onerror=alert(1)> '+ 'Long title '.repeat(30),artist:'Example artist',album:'Example album',current_time:75,duration:240,low_shelf_gain:0,high_shelf_gain:-2}};
  await page.waitForFunction(()=>document.querySelector('#playback-time').textContent==='1:15 / 4:00');
  check((await page.locator('#playback-meta').textContent())==='Example artist · Example album','Cast artist and album shown');
  check(await page.locator('#media-toggle').isEnabled() && await page.locator('#media-pause').isDisabled() && await page.locator('#media-previous').isEnabled() && await page.locator('#media-next').isDisabled(),'paused Cast enables only server-advertised Play and Previous');
  state.streaming.cast.player_state='PLAYING';state.streaming.cast.supported_commands=65;
  await page.waitForFunction(()=>!document.querySelector('#media-pause').disabled);
  check(await page.locator('#media-toggle').isDisabled() && await page.locator('#media-previous').isDisabled() && await page.locator('#media-next').isEnabled(),'playing Cast enables only server-advertised Pause and Next');
  await page.locator('#media-pause').click();
  await page.waitForFunction(()=>document.querySelector('#message-text').textContent==='Pause requested.');
  check(calls.at(-1)==='media_pause' && await page.locator('#media-toggle').isEnabled() && await page.locator('#media-pause').isDisabled(),'Cast pause request follows refreshed receiver capabilities');
  state.streaming.cast.player_state='PLAYING';state.streaming.cast.supported_commands=0;
  await page.waitForFunction(()=>document.querySelector('#media-toggle').disabled && document.querySelector('#media-pause').disabled && document.querySelector('#media-next').disabled);
  check(await page.locator('#media-toggle').isDisabled() && await page.locator('#media-next').isDisabled(),'Cast without pause or skip capabilities keeps those commands disabled');
  check((await page.locator('#playback-title img').count())===0,'remote metadata is rendered as text');
  check(await page.locator('#playback-progress').getAttribute('value')==='75','timeline uses receiver position');
  await page.locator('.device-details summary').click();
  check((await page.locator('#cast-eq').textContent())==='Low 0 dB · High -2 dB (read only)','Cast tone readback shown without unsafe controls');
  for (const width of [320,390,1024]) {
    await page.setViewportSize({width,height:900});
    check(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),'long media metadata fits at '+width);
    check(await page.locator('.transport button').evaluateAll(buttons=>buttons.every(button=>{const r=button.getBoundingClientRect();return r.width>=44 && r.height>=44 && r.left>=0 && r.right<=innerWidth;})),'five 44px transport targets fit at '+width);
  }
  await page.locator('.device-details summary').click();
  const beforeIdle=stateGets;await pause(1200);
  check(stateGets===beforeIdle,'idle browser never polls the state endpoint');
  state=initial();await page.locator('#refresh').click();await pause(200);
  for (const width of [320,390,1024]) {
    await page.setViewportSize({width,height:900});
    check(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),'no horizontal overflow at '+width);
  }
  await page.setViewportSize({width:320,height:920});
  state=initial();state.input='Bluetooth';state.bluetooth={...state.bluetooth,selected:true,connected:true,audio:'Streaming',peer_name:'Example phone'}; await page.locator('#refresh').click();
  await pause(200);
  await mkdir(new URL('build/',root),{recursive:true});
  check(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),'Bluetooth transport fits at 320px');
  await page.screenshot({path:fileURLToPath(new URL('build/webremote-mobile-320.png',root)),fullPage:true});
  await page.setViewportSize({width:390,height:920});
  await page.screenshot({path:fileURLToPath(new URL('build/webremote-mobile-new.png',root)),fullPage:true});
  await page.setViewportSize({width:1200,height:900});
  await page.screenshot({path:fileURLToPath(new URL('build/webremote-desktop-new.png',root)),fullPage:true});
  check(errors.length===0,'no browser exceptions: '+errors.join(', '));
  console.log(checks+' browser UX checks passed');
} finally {await browser.close();clearInterval(pushTimer);for(const res of streams)res.destroy();server.close();}
