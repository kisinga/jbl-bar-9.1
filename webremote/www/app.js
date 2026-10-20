'use strict';
const $ = id => document.getElementById(id);
const buttons = [...document.querySelectorAll('[data-action]')];
const isVolume = action => action === 'volume_up' || action === 'volume_down';
const delta = action => action === 'volume_up' ? 1 : action === 'volume_down' ? -1 : 0;
let current, csrf = '', online = false, active = null, queue = [], epoch = 0;
let refreshing = false, stream = null, latestState = null, lastSync = 0, holdTimer, repeatTimer, heldButton;
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
  $('volume-caption').textContent = !awake ? 'IN STANDBY' : s.muted ? 'SOUND MUTED' : 'LISTENING LEVEL';
  $('volume-pending').textContent = pending && isVolume(active) ? 'Adjusting…' : '\u00a0';
  $('volume-panel').classList.toggle('pending', pending && isVolume(active));
  $('power-state').textContent = ready ? s.power : 'Last known';
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
  $('mode-description').textContent = !awake ? 'Turn on to adjust your sound.' : s.mode === 'Standard' ? 'Smart Mode is off. Enjoy the original mix.' : 'Use Standard to turn Smart Mode off.';
  $('standard-label').textContent = active === 'standard' ? 'Selecting…' : s.mode === 'Standard' ? 'Selected' : 'Use Standard';
  $('standard').classList.toggle('confirmed', s.mode === 'Standard');
  $('volume-hint').textContent = !awake ? 'Turn the bar on to adjust volume.' : s.muted ? 'Sound is muted. Tap above to unmute.' : 'Tap for one step. Hold to keep going.';
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
  else if (reconnecting) feedback('Ready to listen. Settings update live.');
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
    online = false; queue = []; latestState = null; stopHold();
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
    else if (reconnecting) feedback('Ready to listen. All settings are live.');
  } catch {
    if (version !== epoch || active) return;
    online = false;
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
  if (isVolume(active)) return 'Adjusting volume' + (queue.length ? ' · ' + queue.length + ' queued' : '') + '…';
  if (active === 'standard') return 'Selecting Standard… audio may briefly mute.';
  if (active === 'input_bluetooth') return 'Switching to Bluetooth…';
  if (active === 'bluetooth_pair') return 'Starting Bluetooth pairing…';
  return active === 'power' ? 'Changing power state…' : 'Applying your change…';
}
function enqueue(action) {
  const button = buttons.find(b => b.dataset.action === action);
  if (!button || button.disabled || !csrf) return;
  if (isVolume(action) && queue.length && delta(queue[queue.length - 1]) === -delta(action)) queue.pop();
  else if (queue.length < 8) queue.push(action);
  else { feedback('Let the pending volume changes finish first.', 'pending'); return; }
  if (active) { render(); feedback(pendingMessage(), 'pending'); }
  else void pump();
}
async function pump() {
  if (active || !queue.length || !online) return;
  active = queue.shift();
  epoch++;
  render();
  feedback(pendingMessage(), 'pending');
  const action = active;
  try {
    const response = await fetch('/api/action', {method: 'POST', headers: {'Content-Type': 'application/json', 'X-CSRF-Token': csrf}, body: JSON.stringify({action}), signal: AbortSignal.timeout(20000)});
    const text = await response.text();
    let data;
    try { data = JSON.parse(text); } catch { throw new Error(text.trim() || 'Control unavailable. Refresh and try again.'); }
    if (!response.ok) {
      if (data.state && 'available' in data.state) adopt(data.state);
      throw new Error(data.error || 'Control unavailable.');
    }
    adopt(data);
    if (!data.available) throw new Error(data.error || 'Soundbar unavailable.');
    feedback(success(action, data), 'success');
  } catch (error) {
    queue = [];
    stopHold();
    if (error.name === 'TypeError' || error.name === 'TimeoutError') {
      online = false;
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
  $('bt-status').textContent = active === 'input_bluetooth' ? 'Switching to Bluetooth…' : pairing ? 'Starting pairing…' : !ready ? 'Connection unavailable' : !awake ? 'Soundbar in standby' : discoverable ? 'Ready to pair' : bt?.connected ? (bt.audio === 'Streaming' && selected ? 'Streaming audio' : 'Device connected') : 'Bluetooth source selected';
  const peer = bt?.peer_name || 'A Bluetooth device';
  $('bt-help').textContent = !awake ? 'Turn on the bar to connect a device.' : !selected ? peer + ' is connected. Select Bluetooth above to listen.' : discoverable ? 'Choose ' + name + ' in your phone’s Bluetooth settings. The bar is discoverable now.' : bt?.connected ? peer + ' is connected. To use a different device, start pairing below.' : 'Open Bluetooth settings on your phone and choose ' + name + '. If it’s missing, start pairing below.';
  $('bt-pair-label').textContent = pairing ? 'Starting…' : discoverable ? 'Pairing is open' : bt?.connected ? 'Pair another device' : 'Pair a device';
  $('bt-pair').disabled = !ready || !awake || !selected || !!active || discoverable || !bt;
  $('bt-note').textContent = !selected ? 'Select Bluetooth above before starting pairing.' : !bt ? 'Bluetooth status is unavailable on this firmware.' : discoverable ? 'Waiting for a device. This status updates automatically.' : 'Pairing may disconnect the current Bluetooth device.';
}
connectStream();

function renderExtras(s, ready, awake) {
  const sound=s.sound || {};
  $('bass-value').textContent=sound.bass == null ? 'Unavailable' : 'Level ' + (sound.bass+1) + ' of 5';
  $('atmos-value').textContent=['Low','Mid','High'][sound.atmos] || 'Unavailable';
  const rear=['Low','Mid','High','Muted'][sound.rear] || 'Unavailable';
  $('rear-value').textContent=sound.rear_docked ? rear + ' · speakers docked' : rear;
  for (const [id,value] of [['bass-next',sound.bass],['atmos-next',sound.atmos],['rear-next',sound.rear]]) {
    $(id).disabled = !ready || !awake || !!active || value == null || (id==='rear-next' && sound.rear_docked !== false);
  }
  const wifi=s.wifi || {};
  $('wifi-name').textContent=wifi.available ? wifi.connected ? wifi.ssid || 'Connected network' : 'Not connected' : 'Status unavailable';
  $('wifi-status').textContent=wifi.connected ? 'Connected to Wi-Fi' : wifi.hotspot_ip ? 'Setup hotspot is active' : wifi.available ? 'Waiting for a Wi-Fi connection' : 'The Wi-Fi service has not reported its state.';
  $('wifi-ip').textContent=wifi.ip || 'No network address';
  const band=wifi.frequency_mhz ? (wifi.frequency_mhz>=5925?'6 GHz':wifi.frequency_mhz>=4900?'5 GHz':'2.4 GHz') : '';
  $('wifi-signal').textContent=[wifi.signal_dbm==null?'':wifi.signal_dbm+' dBm',band].filter(Boolean).join(' · ');
  $('hotspot-status').hidden=!wifi.hotspot_ip;
  $('hotspot-status').textContent=wifi.hotspot_ip ? 'Local hotspot control: http://' + wifi.hotspot_ip : '';
  const playback=s.streaming;
  const bt=s.bluetooth;
  const bluetoothPlaying=bt?.selected && bt?.audio==='Streaming';
  $('playback').hidden=!awake || (!playback&&!bluetoothPlaying);
  $('playback-service').textContent=playback?.service || 'Bluetooth';
  $('playback-status').textContent=playback?.status || bt?.peer_name || 'Playing audio';
  $('playback').classList.toggle('active',ready && awake && (!!playback?.active || bluetoothPlaying));
}
