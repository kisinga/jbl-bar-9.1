'use strict';
const $ = id => document.getElementById(id);
const buttons = [...document.querySelectorAll('[data-action]')];
const isVolume = action => action === 'volume_up' || action === 'volume_down';
const delta = action => action === 'volume_up' ? 1 : action === 'volume_down' ? -1 : 0;
let current, csrf = '', online = false, active = null, queue = [], epoch = 0;
let refreshing = false, stream = null, latestState = null, lastSync = 0, holdTimer, repeatTimer, heldButton;
// A successful sender request changes the next button action, not reported playback.
let mediaIntent = null, mediaPeer = '', mediaAudio = '';
let eqDraft = null, eqCommitted = '';
const eqSliders = Array.from({length: 5}, (_, index) => $('eq-slider-' + index));
const eqPresets = Object.freeze({
  original: Object.freeze([0,0,0,0,0]),
  less_bass: Object.freeze([-4,-2,0,0,0]),
  clear_dialogue: Object.freeze([-3,-1,0,0,-1]),
  softer_treble: Object.freeze([0,0,0,-2,-3]),
  warm: Object.freeze([0,0,-1,-2,-3]),
  bright: Object.freeze([-3,-2,-1,0,0]),
  vocal_focus: Object.freeze([-4,-2,0,-1,-2]),
});
function mediaIdentity(s) {
  if (!s?.available || s.power !== 'On') return '';
  const bt = s?.bluetooth;
  if (s?.source_state === 7 && s?.streaming?.service === 'AirPlay' && s.streaming.active) return 'airplay';
  return s?.input === 'Bluetooth' && bt?.connected ? JSON.stringify([bt.peer_name || '', bt.name || '']) : '';
}
function syncMediaIntent(s) {
  const peer = mediaIdentity(s), audio = peer === 'airplay' ? 'Session active' : s?.bluetooth?.audio || '';
  if (!peer || peer !== mediaPeer || audio !== mediaAudio) mediaIntent = null;
  mediaPeer = peer; mediaAudio = audio;
}
function feedback(text, kind = '') {
  $('message-text').textContent = text;
  $('message').dataset.kind = kind;
}
function sourceName(s) { return /^Input \d+$/.test(s.input) ? 'Other source' : s.input || 'Unknown source'; }
function settingsChanged(s) { return current && (['volume','muted','power','input','mode'].some(key=>current[key]!==s[key]) || JSON.stringify(current.sound)!==JSON.stringify(s.sound)); }
function pendingVolume() { return (active ? delta(active) : 0) + queue.reduce((n, action) => n + delta(action), 0); }
function render() {
  const s = current;
  if (!s) return;
  const ready = online && s.available;
  const awake = s.power === 'On';
  const pending = active !== null || queue.length > 0;
  $('connection').dataset.state = ready ? 'online' : online ? 'connecting' : 'offline';
  $('connection-label').textContent = ready ? 'Connected' : online ? 'Starting' : 'Offline';
  $('summary').textContent = !online ? 'Connection lost. Showing the last known settings.' : !s.available ? (s.error || 'Waiting for your soundbar…') : awake ? sourceName(s) + ' · ' + s.mode + ' mode' : s.power === 'Standby' ? 'In standby. Ready when you are.' : 'Your soundbar is starting…';
  const offset = pendingVolume();
  $('volume').textContent = s.volume == null ? '—' : Math.max(0, s.volume + offset);
  $('volume-caption').textContent = !awake ? 'IN STANDBY' : s.muted ? 'SOUND MUTED' : '';
  $('volume-pending').textContent = pending && isVolume(active) ? 'Adjusting…' : '\u00a0';
  $('volume-panel').classList.toggle('pending', pending && isVolume(active));
  $('power').classList.toggle('on', awake);
  $('power').setAttribute('aria-label', awake ? 'Put soundbar in standby' : 'Turn soundbar on');
  $('power-label').textContent = active === 'power' ? 'Working…' : awake ? 'Standby' : 'Turn on';
  $('mute').classList.toggle('active', !!s.muted);
  $('mute').setAttribute('aria-pressed', String(!!s.muted));
  $('mute-label').textContent = active === 'mute' ? 'Changing…' : s.muted ? 'Unmute sound' : 'Mute sound';
  $('mute-icon').setAttribute('href', s.muted ? '#i-muted' : '#i-volume');
  $('input-name').textContent = sourceName(s);
  document.querySelectorAll('[data-input]').forEach(b => {
    const selected = b.dataset.input === s.input;
    b.classList.toggle('selected', selected);
    b.setAttribute('aria-pressed', String(selected));
  });
  $('mode').textContent = s.mode === 'Standard' ? 'Standard' : s.mode === 'Smart' ? 'Smart Mode' : 'Checking mode';
  $('mode-description').textContent = !awake ? 'Turn on to adjust your sound.' : s.mode === 'Standard' ? 'Smart Mode is off.' : 'Use Standard to turn Smart Mode off.';
  $('standard-label').textContent = active === 'standard' ? 'Selecting…' : s.mode === 'Standard' ? 'Selected' : 'Use Standard';
  $('standard').classList.toggle('confirmed', s.mode === 'Standard');
  $('volume-hint').textContent = !awake ? 'Turn the bar on to adjust volume.' : s.muted ? 'Sound is muted. Tap above to unmute.' : 'Tap to adjust. Hold to keep going.';
  buttons.forEach(b => {
    const action = b.dataset.action;
    b.disabled = !ready || !['On', 'Standby'].includes(s.power) || (action !== 'power' && !awake) || (pending && !(isVolume(action) && isVolume(active))) || (action === 'standard' && s.mode === 'Standard');
    b.classList.toggle('pending-control', action === active && !isVolume(action));
    b.setAttribute('aria-busy', String(action === active));
  });
  $('refresh').disabled = pending || refreshing;
  $('cancel-volume').hidden = queue.length === 0;
  updateSync();
  renderBluetooth(s, ready, awake);
  renderExtras(s, ready, awake);
  renderSystem(s, online);
  renderEQ(s, ready, awake);
}
function updateSync() {
  if (stream?.readyState === EventSource.OPEN && online) { $('last-seen').textContent = 'Live updates'; return; }
  if (!lastSync) return;
  const seconds = Math.floor((Date.now() - lastSync) / 1000);
  $('last-seen').textContent = seconds < 5 ? 'Synced just now' : 'Synced ' + seconds + 's ago';
}
function versioned(s) { return Number.isSafeInteger(s?.revision) && s.revision >= 0; }
function adopt(s) {
  if (versioned(current) && versioned(s) && s.revision < current.revision && (!s.csrf || s.csrf === csrf)) return false;
  current = s;
  csrf = s.csrf || csrf;
  online = true;
  lastSync = Date.now();
  return true;
}
function receiveState(s) {
  if (active) { if (!latestState || !versioned(s) || !versioned(latestState) || s.revision >= latestState.revision || s.csrf !== latestState.csrf) latestState = s; return; }
  const reconnecting = !online || !current?.available;
  const changed = settingsChanged(s);
  if (!adopt(s)) return;
  if (!s.available) feedback(s.error || 'Waiting for your soundbar…', 'error');
  else if (reconnecting) feedback('Ready. Your settings update live.');
  else if (changed && $('message').dataset.kind === 'success') feedback('Settings updated from the soundbar.');
  render();
}
function connectStream() {
  if (stream || document.hidden) return;
  const source = new EventSource('/api/events');
  stream = source;
  source.onmessage = event => {
    if (stream !== source) return;
    try { receiveState(JSON.parse(event.data)); } catch { feedback('Unable to read live state. Refresh to reconnect.', 'error'); }
  };
  source.onerror = () => {
    if (stream !== source) return;
    online = false; queue = []; latestState = null; mediaIntent = null; stopHold();
    feedback('Live connection lost. Reconnecting; pending taps were cleared.', 'error');
    render();
    if (!current) { $('connection-label').textContent='Reconnecting';$('refresh').disabled=false; }
  };
}
async function refresh() {
  if (refreshing || active || document.hidden) return;
  const version = epoch;
  refreshing = true;
  $('refresh').disabled = true;
  try {
    const response = await fetch('/api/state', {cache: 'no-store', signal: AbortSignal.timeout(8000)});
    if (!response.ok) throw new Error('Unable to reach the soundbar.');
    const s = await response.json();
    if (version !== epoch || active) return;
    const reconnecting = !online || !current?.available;
    const changed = settingsChanged(s);
    adopt(s);
    if (changed && $('message').dataset.kind === 'success') feedback('Settings updated from the soundbar.');
    if (!s.available) feedback(s.error || 'Waiting for your soundbar…', 'error');
    else if (reconnecting) feedback('Ready. Your settings update live.');
  } catch {
    if (version !== epoch || active) return;
    online = false;
    mediaIntent = null;
    queue = [];
    stopHold();
    feedback('Connection lost. Reconnecting automatically; no controls will be replayed.', 'error');
  } finally {
    refreshing = false;
    render();
    if (!current) {
      $('connection').dataset.state = 'offline';
      $('connection-label').textContent = 'Offline';
      $('summary').textContent = 'Waiting for your soundbar. Check the local connection.';
      $('refresh').disabled = false;
    }

  }
}
function success(action, s) {
  if (action === 'eq_reset' || (action === 'eq_apply' && s.eq?.enabled === false)) return 'Original tuning restored.';
  if (action === 'eq_apply') return 'Front EQ applied.';
	if (action === 'night_toggle') return 'Night mode ' + (s.sound?.night ? 'on.' : 'off.');
	if (action === 'media_play') return 'Play requested.';
	if (action === 'media_pause') return 'Pause requested.';
	if (action === 'media_next') return 'Next track requested.';
	if (action === 'media_previous') return 'Previous track requested.';
	if (action === 'media_stop') return 'Stop requested.';
  if (isVolume(action)) return 'Volume ' + s.volume + (s.muted ? ' · sound muted' : '');
  if (action === 'mute') return s.muted ? 'Sound muted.' : 'Sound unmuted.';
  if (action === 'power') return s.power === 'On' ? 'Your soundbar is on.' : 'Soundbar is in standby.';
  if (action === 'bass_next') return 'Bass level ' + (s.sound.bass+1) + ' selected.';
  if (action === 'atmos_next') return 'Atmos height: ' + ['Low','Mid','High'][s.sound.atmos] + '.';
  if (action === 'rear_next') return 'Rear speakers: ' + ['Low','Mid','High','Muted'][s.sound.rear] + '.';
  if (action === 'standard') return 'Standard selected. Smart Mode is off.';
  if (action === 'input_bluetooth') return s.bluetooth?.connected ? 'Bluetooth selected. A device is connected.' : 'Bluetooth selected. Choose ' + (s.bluetooth?.name || 'JBL Bar 9.1') + ' on your phone.';
  if (action === 'bluetooth_pair') return 'Pairing is open. Choose ' + (s.bluetooth?.name || 'JBL Bar 9.1') + ' in your phone’s Bluetooth settings.';
  return sourceName(s) + ' selected.';
}
function pendingMessage() {
  if (active === 'eq_apply') return 'Applying front EQ…';
  if (active === 'eq_reset') return 'Restoring original tuning…';
	if (active === 'night_toggle') return 'Changing night mode… audio may briefly mute.';
	if (active?.startsWith('media_')) return 'Sending playback control…';
  if (isVolume(active)) return 'Adjusting volume' + (queue.length ? ' · ' + queue.length + ' queued' : '') + '…';
  if (active === 'standard') return 'Selecting Standard… audio may briefly mute.';
  if (active === 'input_bluetooth') return 'Switching to Bluetooth…';
  if (active === 'bluetooth_pair') return 'Starting Bluetooth pairing…';
  return active === 'power' ? 'Changing power state…' : 'Applying your change…';
}
function enqueue(action) {
  const button = buttons.find(b => b.dataset.action === action);
  if (!button || button.disabled || !csrf) return;
  if (action === 'eq_apply' && !validEQGains(eqDraft)) return;
  if (isVolume(action) && queue.length && delta(queue[queue.length - 1]) === -delta(action)) queue.pop();
  else if (queue.length < 8) queue.push(action);
  else { feedback('Let the pending volume changes finish first.', 'pending'); return; }
  if (active) { render(); feedback(pendingMessage(), 'pending'); }
  else void pump();
}
async function pump() {
  if (active || !queue.length || !online) return;
  active = queue.shift();
  const gains = active === 'eq_apply' ? eqDraft?.slice() : null;
  epoch++;
  render();
  feedback(pendingMessage(), 'pending');
  const action = active;
  const requestPeer = mediaIdentity(current);
  try {
    const response = await fetch('/api/action', {method: 'POST', headers: {'Content-Type': 'application/json', 'X-CSRF-Token': csrf}, body: JSON.stringify(action === 'eq_apply' ? {action, gains_db: gains} : {action}), signal: AbortSignal.timeout(20000)});
    const text = await response.text();
    let data;
    try { data = JSON.parse(text); } catch { throw new Error(text.trim() || 'Control unavailable. Refresh and try again.'); }
    if (!response.ok) {
      if (data.state && 'available' in data.state) adopt(data.state);
      throw new Error(data.error || 'Control unavailable.');
    }
    adopt(data);
    if (!data.available) throw new Error(data.error || 'Soundbar unavailable.');
    syncMediaIntent(data);
    if (requestPeer && requestPeer === mediaIdentity(data) && ['media_play','media_pause','media_stop'].includes(action)) mediaIntent = action;
    feedback(success(action, data), 'success');
  } catch (error) {
    queue = [];
    stopHold();
    if (error.name === 'TypeError' || error.name === 'TimeoutError') {
      online = false;
      mediaIntent = null;
      feedback('Connection interrupted. The last change may have applied; pending taps were cleared.', 'error');
    } else feedback(error.message, 'error');
  } finally {
    active = null;
    render();
    if (latestState) {const next=latestState;latestState=null;receiveState(next);}
    if (queue.length && online) void pump();
  }
}
function stopHold() {
  clearTimeout(holdTimer);
  clearInterval(repeatTimer);
  if (heldButton) heldButton.classList.remove('pressed');
  heldButton = null;
}
buttons.forEach(button => {
  button.addEventListener('click', event => {
    // Pointer input is handled on down; keyboard and assistive clicks have detail 0.
    if (!isVolume(button.dataset.action) || event.detail === 0) enqueue(button.dataset.action);
  });
  if (!isVolume(button.dataset.action)) return;
  button.addEventListener('pointerdown', event => {
    if (event.button !== 0 || button.disabled) return;
    event.preventDefault();
    stopHold();
    button.focus({preventScroll: true});
    button.setPointerCapture(event.pointerId);
    heldButton = button;
    button.classList.add('pressed');
    enqueue(button.dataset.action);
    holdTimer = setTimeout(() => {
      repeatTimer = setInterval(() => {
        if (queue.length < 1 && !button.disabled) enqueue(button.dataset.action);
      }, 180);
    }, 400);
  });
  for (const event of ['pointerup', 'pointercancel', 'lostpointercapture']) button.addEventListener(event, stopHold);
});
document.addEventListener('keydown', event => {
  if (event.altKey || event.ctrlKey || event.metaKey || event.target.closest('input,textarea,select,[contenteditable="true"]')) return;
  const action = {'ArrowUp': 'volume_up', 'ArrowDown': 'volume_down', m: 'mute', M: 'mute'}[event.key];
  if (!action || (event.repeat && (!isVolume(action) || queue.length >= 1))) return;
  event.preventDefault();
  enqueue(action);
});
window.addEventListener('blur', stopHold);
document.addEventListener('visibilitychange', () => {
  stopHold();
  if (document.hidden) {stream?.close();stream=null;} else connectStream();
});
$('refresh').addEventListener('click', () => { connectStream(); void refresh(); });
$('cancel-volume').addEventListener('click', () => {
  queue = [];
  stopHold();
  render();
  feedback(active ? 'Queued taps cleared. Finishing the current step…' : 'Queued taps cleared.', active ? 'pending' : '');
});
function renderBluetooth(s, ready, awake) {
  const selected = s.input === 'Bluetooth';
  const bt = s.bluetooth;
  $('bluetooth-panel').hidden = !selected && !bt?.connected && active !== 'input_bluetooth';
  const name = bt?.name || 'JBL Bar 9.1';
  const pairing = active === 'bluetooth_pair';
  const discoverable = !!bt?.discoverable && !!bt?.connectable;
  $('bluetooth-panel').classList.toggle('discoverable', discoverable && ready && awake);
  $('bt-status').textContent = active === 'input_bluetooth' ? 'Switching to Bluetooth…' : pairing ? 'Starting pairing…' : !ready ? 'Connection unavailable' : !awake ? 'Soundbar in standby' : discoverable ? 'Ready to pair' : bt?.connected ? (selected && bt.audio === 'Streaming' ? 'Streaming audio' : selected && bt.audio === 'Paused' ? 'Stream suspended' : 'Device connected') : 'Bluetooth source selected';
  const peer = bt?.peer_name || 'A Bluetooth device';
  $('bt-help').textContent = !awake ? 'Turn on the bar to connect a device.' : !selected ? peer + ' is connected. Select Bluetooth above to listen.' : discoverable ? 'Choose ' + name + ' in your phone’s Bluetooth settings. The bar is discoverable now.' : bt?.connected ? peer + ' is connected. To use a different device, start pairing below.' : 'Open Bluetooth settings on your phone and choose ' + name + '. If it’s missing, start pairing below.';
  $('bt-pair-label').textContent = pairing ? 'Starting…' : discoverable ? 'Pairing is open' : bt?.connected ? 'Pair another device' : 'Pair a device';
  $('bt-pair').disabled = !ready || !awake || !selected || !!active || discoverable || !bt;
  $('bt-note').textContent = !selected ? 'Select Bluetooth above before starting pairing.' : !bt ? 'Bluetooth status is unavailable on this firmware.' : discoverable ? 'Waiting for a device. This status updates automatically.' : 'Pairing may disconnect the current Bluetooth device.';
}
connectStream();

function mediaTime(value) {
  if (!Number.isFinite(value) || value < 0) return '';
  const seconds = Math.floor(value);
  return (seconds >= 3600 ? Math.floor(seconds / 3600) + ':' : '') + String(Math.floor(seconds / 60) % 60).padStart(seconds >= 3600 ? 2 : 1, '0') + ':' + String(seconds % 60).padStart(2, '0');
}
function watcherName(value) {
  if (!value) return 'Not reported';
  if (value === 'PAUSING_LEGACY') return 'Starting automation';
  if (value === 'WAITING_FOR_READY') return 'Waiting for sound controls';
  if (['STANDARD_THIS_WAKE', 'ALREADY_STANDARD_THIS_WAKE'].includes(value)) return 'Standard selected for this wake';
  if (value.startsWith('WAITING_POWER_') || value === 'WAITING_FOR_POWER_ON') return 'Waiting for power-on';
  if (value === 'WAITING_FOR_PROCESS') return 'Waiting for the soundbar';
  if (value === 'APPLYING_STANDARD' || value.startsWith('AWAKE_STABILIZING_')) return 'Selecting Standard…';
  if (value.startsWith('PROCESS_')) return 'Starting the watcher';
  return 'Check automation logs';
}
function renderPlayback(s, ready, awake) {
  const bt = s.bluetooth;
  const bluetooth = s.input === 'Bluetooth';
  const playback = bluetooth ? {
    service: 'Bluetooth',
    status: bt?.connected == null ? 'Status unavailable' : !bt.connected ? 'Waiting for a device' : bt.audio === 'Streaming' ? 'Streaming audio' : bt.audio === 'Paused' ? 'Stream suspended' : 'Connected',
    active: bt?.connected && bt.audio === 'Streaming',
  } : s.streaming;
  const cast = playback?.cast;
  $('playback').hidden = !awake || !playback;
  $('playback-icon').setAttribute('href', bluetooth ? '#i-bluetooth' : '#i-cast');
  $('playback-service').textContent = playback?.service || '';
  $('playback-status').textContent = playback?.status || '';
  const title = bluetooth ? (bt?.connected ? bt.peer_name || 'Connected device' : '') : cast?.title || '';
  $('playback-title').hidden = !title;
  $('playback-title').textContent = title;
  const metadata = bluetooth ? '' : [cast?.artist, cast?.album].filter(Boolean).join(' · ');
  $('playback-meta').hidden = !metadata;
  $('playback-meta').textContent = metadata;
  $('playback-app').hidden = !cast?.application;
  $('playback-app').textContent = cast?.application || '';
  $('playback-detail').hidden = !playback?.detail;
  $('playback-detail').textContent = playback?.detail || '';
  const timed = !cast?.mirroring && Number.isFinite(cast?.duration) && cast.duration > 0 && Number.isFinite(cast?.current_time);
  $('playback-timeline').hidden = !timed;
  $('playback-progress').max = timed ? cast.duration : 1;
  $('playback-progress').value = timed ? Math.min(cast.current_time, cast.duration) : 0;
  $('playback-time').textContent = timed ? mediaTime(cast.current_time) + ' / ' + mediaTime(cast.duration) : '';
  $('playback').classList.toggle('active', ready && awake && !!playback?.active);
  $('playback').classList.toggle('mirroring', !!cast?.mirroring);
  syncMediaIntent(s);
  const controls = s.media_controls || [];
  const airplay = playback?.service === 'AirPlay';
  const wantsPause = bluetooth ? (mediaIntent ? mediaIntent === 'media_play' : bt?.audio === 'Streaming') : airplay ? (mediaIntent ? mediaIntent === 'media_play' : !!playback.active) : cast?.player_state === 'PLAYING';
  let toggle = wantsPause ? 'media_pause' : 'media_play';
  if (!controls.includes(toggle) && controls.includes(wantsPause ? 'media_play' : 'media_pause')) toggle = wantsPause ? 'media_play' : 'media_pause';
  const label = toggle === 'media_pause' ? 'Pause' : 'Play';
  $('media-toggle').dataset.action = toggle;
  $('media-toggle').setAttribute('aria-label', label);
  $('media-toggle').title = label;
  $('media-toggle-label').textContent = label;
  $('media-toggle-icon').setAttribute('href', toggle === 'media_pause' ? '#i-pause' : '#i-play');
  for (const id of ['media-previous','media-toggle','media-next','media-stop']) {
    $(id).disabled = !ready || !awake || !!active || !controls.includes($(id).dataset.action);
  }
  $('media-help').textContent = controls.length ? (bluetooth ? 'Your app must support remote controls. Streaming may continue while an app is paused.' : airplay ? 'Sent to the AirPlay sender. Its play/pause state isn’t reported.' : 'Controls request changes from the receiver.') : cast?.mirroring ? 'Use playback controls in the sending app.' : bluetooth && bt?.connected ? 'Playback controls are unavailable for this connection.' : 'Playback controls aren’t available on this source.';
}
function renderExtras(s, ready, awake) {
  const sound=s.sound || {};
  $('night-value').textContent = sound.night == null ? 'Unavailable' : sound.night ? 'On' : 'Off';
  $('night-label').textContent = active === 'night_toggle' ? 'Changing…' : sound.night ? 'Turn off' : 'Turn on';
  $('night').setAttribute('aria-pressed', String(sound.night === true));
  $('night').disabled = !ready || !awake || !!active || sound.night == null;
  $('bass-value').textContent=sound.bass == null ? 'Unavailable' : 'Level ' + (sound.bass+1) + ' of 5';
  $('atmos-value').textContent=['Low','Mid','High'][sound.atmos] || 'Unavailable';
  const rear=['Low','Mid','High','Muted'][sound.rear] || 'Unavailable';
  $('rear-value').textContent=sound.rear_docked ? rear + ' · speakers docked' : rear;
  for (const [id,value] of [['bass-next',sound.bass],['atmos-next',sound.atmos],['rear-next',sound.rear]]) {
    $(id).disabled = !ready || !awake || !!active || value == null || (id==='rear-next' && sound.rear_docked !== false);
  }
  const wifi=s.wifi || {};
  $('wifi-name').textContent=wifi.available ? wifi.connected ? wifi.ssid || 'Connected network' : 'Not connected' : 'Status unavailable';
  const signal = wifi.signal_dbm;
  const quality = signal == null ? '' : signal >= -55 ? 'Strong signal' : signal >= -67 ? 'Good signal' : signal >= -75 ? 'Fair signal' : 'Weak signal';
  $('wifi-status').textContent=wifi.connected ? ['Connected', quality].filter(Boolean).join(' · ') : wifi.hotspot_ip ? 'Setup hotspot is active' : wifi.available ? 'Waiting for a Wi-Fi connection' : 'The Wi-Fi service has not reported its state.';
  $('wifi-ip').textContent=wifi.ip || 'No network address';
  $('local-address').hidden = s.hostname !== 'jbl.local';
  const band=wifi.frequency_mhz ? (wifi.frequency_mhz>=5925?'6 GHz':wifi.frequency_mhz>=4900?'5 GHz':'2.4 GHz') : '';
  $('wifi-signal').textContent=[wifi.signal_dbm==null?'':wifi.signal_dbm+' dBm',band].filter(Boolean).join(' · ');
  $('hotspot-status').hidden=!wifi.hotspot_ip;
  $('hotspot-status').textContent=wifi.hotspot_ip ? 'Local hotspot control: http://' + wifi.hotspot_ip : '';
  renderPlayback(s, ready, awake);
  $('device-name').textContent = s.bluetooth?.name || 'Not reported';
  $('device-uptime').textContent = Number.isFinite(s.uptime) && s.uptime > 0 ? Math.floor(s.uptime / 86400) + 'd ' + Math.floor(s.uptime / 3600) % 24 + 'h ' + Math.floor(s.uptime / 60) % 60 + 'm' : 'Not reported';
  $('device-automation').textContent = s.automation?.enabled ? (s.automation.error || 'Go · ' + watcherName(s.automation.status)) : watcherName(s.watcher);
  const cast = s.streaming?.cast;
  const gains = [cast?.low_shelf_gain, cast?.high_shelf_gain];
  const hasGains = gains.every(Number.isFinite);
  $('cast-eq-row').hidden = !hasGains;
  $('cast-eq').textContent = hasGains ? 'Low ' + gains[0] + ' dB · High ' + gains[1] + ' dB (read only)' : '';
}


// History belongs to this visible page, and advances only for real server samples.
const monitorHistory = [];
let monitorSample = null, monitorReceived = 0;
const finitePositive = value => Number.isFinite(value) && value > 0;
const finiteNonnegative = value => Number.isFinite(value) && value >= 0;
function memoryReading(value) {
  if (!value || !finitePositive(value.total_bytes) || !finiteNonnegative(value.available_bytes) || !finiteNonnegative(value.used_bytes) || value.available_bytes > value.total_bytes || value.used_bytes > value.total_bytes) return null;
  return value;
}
function monitorBytes(value) { return (value / 1048576).toLocaleString(undefined, {maximumFractionDigits: 1}) + ' MiB'; }
function monitorRate(value) {
  if (!finiteNonnegative(value)) return '—';
  const unit = value >= 1048576 ? 1048576 : value >= 1024 ? 1024 : 1;
  return (value / unit).toLocaleString(undefined, {maximumFractionDigits: unit === 1 ? 0 : 1}) + (unit === 1048576 ? ' MiB/s' : unit === 1024 ? ' KiB/s' : ' B/s');
}
function monitorUptime(value) {
  if (!finiteNonnegative(value)) return '—';
  if (value < 60) return Math.floor(value) + 's';
  return [value >= 86400 ? Math.floor(value / 86400) + 'd' : '', value >= 3600 ? Math.floor(value / 3600) % 24 + 'h' : '', Math.floor(value / 60) % 60 + 'm'].filter(Boolean).join(' ');
}
function drawMonitorChart(id, key, label) {
  let line = '', area = '', segment = [];
  const finish = () => {
    if (!segment.length) return;
    line += 'M' + segment.map(p => p.join(',')).join('L');
    if (segment.length > 1) area += 'M' + segment[0][0] + ',72L' + segment.map(p => p.join(',')).join('L') + 'L' + segment.at(-1)[0] + ',72Z';
    segment = [];
  };
  for (const point of monitorHistory) {
    if (!Number.isFinite(point[key])) { finish(); continue; }
    const x = Math.max(0, 300 - (monitorReceived - point.time) / 120000 * 300);
    segment.push([Number(x.toFixed(2)), Number((70 - point[key] * .68).toFixed(2))]);
  }
  finish();
  $(id + '-line').setAttribute('d', line);
  $(id + '-area').setAttribute('d', area);
  const last = monitorHistory.at(-1);
  $(id + '-point').toggleAttribute('hidden', !Number.isFinite(last?.[key]));
  if (Number.isFinite(last?.[key])) { $(id + '-point').setAttribute('cx', 300); $(id + '-point').setAttribute('cy', 70 - last[key] * .68); }
  const valid = monitorHistory.filter(point => Number.isFinite(point[key]));
  $(id + '-chart').dataset.samples = valid.length;
  $(id + '-chart').setAttribute('aria-label', label + ' history: ' + (valid.length ? valid.length + ' real samples, latest ' + Math.round(valid.at(-1)[key]) + '%' : 'no samples available'));
}
function renderSystem(s, connected) {
  const reported = s.system;
  const valid = connected && Number.isSafeInteger(reported?.sample) && reported.sample >= 0;
  const now = Date.now();
  if (!valid) { monitorHistory.length = 0; monitorSample = null; monitorReceived = 0; }
  const data = valid ? reported : {};
  const cpu = finiteNonnegative(data.cpu_percent) && data.cpu_percent <= 100 ? data.cpu_percent : null;
  const memory = memoryReading(data.memory), storage = memoryReading(data.storage);
  const ram = memory ? memory.used_bytes / memory.total_bytes * 100 : null;
  if (valid && data.sample !== monitorSample) {
    if (monitorSample !== null && (data.sample < monitorSample || now - monitorReceived > 6500)) monitorHistory.length = 0;
    monitorSample = data.sample; monitorReceived = now;
    monitorHistory.push({time: now, cpu, ram});
    while (monitorHistory.length > 60 || now - monitorHistory[0].time > 120000) monitorHistory.shift();
  }
  const stale = valid && now - monitorReceived > 6500;
  $('system-panel').dataset.state = !connected ? 'offline' : stale ? 'stale' : valid ? 'live' : 'waiting';
  $('system-status').textContent = !connected ? 'Offline' : stale ? 'Waiting for new data' : valid ? 'Live · every 2s' : 'Not available';
  $('cpu-value').textContent = cpu === null ? '—' : Math.round(cpu) + '%';
  $('cpu-detail').textContent = Number.isSafeInteger(data.cpu_cores) && data.cpu_cores > 0 ? data.cpu_cores + (data.cpu_cores === 1 ? ' core' : ' cores') : 'Cores not reported';
  $('memory-value').textContent = ram === null ? '—' : Math.round(ram) + '%';
  $('memory-detail').textContent = memory ? monitorBytes(memory.used_bytes) + ' / ' + monitorBytes(memory.total_bytes) : 'Usage not reported';
  $('memory-available').textContent = memory ? monitorBytes(memory.available_bytes) + ' available' + (memory.estimated_available ? ' · estimated' : '') : 'Available memory not reported';
  $('history-count').textContent = monitorHistory.length ? monitorHistory.length === 1 ? 'Collecting history' : monitorHistory.length + ' samples · 2 min window' : 'No samples yet';
  drawMonitorChart('cpu', 'cpu', 'CPU'); drawMonitorChart('memory', 'ram', 'RAM');
  $('storage-progress').hidden = !storage;
  $('storage-progress').value = storage ? storage.used_bytes / storage.total_bytes * 100 : 0;
  $('storage-detail').textContent = storage ? Math.round(storage.used_bytes / storage.total_bytes * 100) + '% used' : '—';
  $('storage-value').textContent = storage ? monitorBytes(storage.available_bytes) + ' free of ' + monitorBytes(storage.total_bytes) : 'Usage not reported';
  $('network-receive').textContent = monitorRate(data.network?.receive_bytes_per_second);
  $('network-transmit').textContent = monitorRate(data.network?.transmit_bytes_per_second);
  $('system-uptime').textContent = monitorUptime(data.uptime_seconds);
  $('system-load').textContent = Array.isArray(data.load_average) && data.load_average.length === 3 && data.load_average.every(finiteNonnegative) ? data.load_average.map(value => value.toFixed(2)).join(' / ') : '—';
  $('system-processes-row').hidden = !Number.isSafeInteger(data.processes) || data.processes < 0;
  $('system-processes').textContent = data.processes ?? '—';
  $('system-temperature-row').hidden = !Number.isFinite(data.temperature_c);
  $('system-temperature').textContent = Number.isFinite(data.temperature_c) ? data.temperature_c.toFixed(1) + ' °C' : '—';
  $('system-note').textContent = !connected ? 'Connection lost. Vitals will resume with fresh samples.' : stale ? 'These readings have stopped updating. Waiting for a fresh sample.' : valid ? 'Real device samples. History builds while this page is open.' : 'System vitals are not available from this version of the remote.';
}

// A stalled stream can stay open: age the label without fetching or adding samples.
setInterval(() => { if (!document.hidden && monitorReceived && Date.now() - monitorReceived > 6500) renderSystem(current, online); }, 2000);


function validEQGains(gains) {
  return Array.isArray(gains) && gains.length === 5 && gains.every(value => Number.isFinite(value) && value >= -6 && value <= 0 && Number.isInteger(value * 2));
}
function eqGain(value) { return (value === 0 ? '0' : value.toFixed(1).replace(/\.0$/, '')) + ' dB'; }
function renderEQ(s, ready, awake) {
  const eq = s.eq || {};
  const valid = validEQGains(eq.gains_db);
  const key = JSON.stringify([s.csrf || csrf, eq.enabled, valid ? eq.gains_db : null]);
  if (key !== eqCommitted) { eqCommitted = key; eqDraft = valid ? eq.gains_db.slice() : null; }
  const draftValid = validEQGains(eqDraft);
  const changed = valid && draftValid && eqDraft.some((value, index) => value !== eq.gains_db[index]);
  const pending = !!active || queue.length > 0;
  const canAdjust = ready && awake && eq.available === true && valid && !pending && eq.status !== 'reset_required';
  $('eq-preset').disabled = !canAdjust;
  $('eq-preset').value = draftValid ? Object.keys(eqPresets).find(name => eqPresets[name].every((value, index) => value === eqDraft[index])) || 'custom' : 'custom';
  for (const [index, input] of eqSliders.entries()) {
    input.disabled = !canAdjust;
    input.value = draftValid ? eqDraft[index] : 0;
    input.setAttribute('aria-valuetext', draftValid ? eqGain(eqDraft[index]) : 'Unavailable');
    $('eq-gain-' + index).textContent = draftValid ? eqGain(eqDraft[index]) : '—';
  }
  $('eq-apply').disabled = !canAdjust || !changed;
  $('eq-apply').textContent = active === 'eq_apply' ? 'Applying…' : 'Apply EQ';
  $('eq-reset').disabled = !ready || !awake || pending || !(eq.enabled === true || eq.status === 'reset_required');
  $('eq-reset').textContent = active === 'eq_reset' ? 'Resetting…' : 'Reset';
  $('eq-panel').dataset.state = !ready ? 'offline' : eq.status === 'reset_required' ? 'reset-required' : !eq.available || !valid ? 'unavailable' : changed ? 'draft' : eq.enabled ? 'custom' : 'original';
  $('eq-state').textContent = !online ? 'Offline. Reconnect to adjust EQ.' : !ready ? 'Waiting for the soundbar.' : !awake ? 'Turn on the bar to adjust EQ.' : eq.status === 'reset_required' ? 'Reset required before making further changes.' : eq.error ? eq.error : !eq.available || !valid ? 'EQ is unavailable on this firmware or audio path.' : active === 'eq_apply' ? 'Applying your settings…' : active === 'eq_reset' ? 'Restoring original processing…' : changed ? 'Unapplied changes. Select Apply EQ to listen.' : eq.enabled ? 'Custom EQ active. Reset restores original processing.' : 'Original tuning. Adjust a band, then apply.';
  $('eq-graph').hidden = !draftValid;
  if (draftValid) {
    const frequencies = Array.isArray(eq.frequencies_hz) && eq.frequencies_hz.length === 5 && eq.frequencies_hz.every(value => Number.isFinite(value) && value > 0) ? eq.frequencies_hz : [125,500,2000,4000,8000];
    const points = eqDraft.map((gain, index) => [10 + Math.log2(frequencies[index] / 125) / 6 * 280, 8 - gain * 8]);
    const line = 'M' + points.map(point => point.join(',')).join('L');
    $('eq-gain-line').setAttribute('d', line);
    $('eq-gain-area').setAttribute('d', line + 'L290,8L10,8Z');
    $('eq-graph').querySelector('svg').setAttribute('aria-label', (changed ? 'Unapplied' : eq.enabled ? 'Applied' : 'Original') + ' EQ gain settings: ' + eqDraft.map(eqGain).join(', '));
  }
}
for (const [index, input] of eqSliders.entries()) input.addEventListener('input', () => {
  if (input.disabled || !validEQGains(eqDraft)) return;
  const next = eqDraft.slice(); next[index] = Number(input.value);
  if (!validEQGains(next)) return;
  eqDraft = next;
  renderEQ(current, online && current.available, current.power === 'On');
});

$('eq-preset').addEventListener('change', () => {
  const preset = $('eq-preset').value;
  if ($('eq-preset').disabled || !Object.hasOwn(eqPresets, preset)) return;
  eqDraft = eqPresets[preset].slice();
  renderEQ(current, online && current.available, current.power === 'On');
});
