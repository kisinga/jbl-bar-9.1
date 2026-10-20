'use strict';
let csrf = '', busy = false, current;
const $ = id => document.getElementById(id);
const buttons = [...document.querySelectorAll('[data-action]')];
function render(s) {
  current = s; csrf = s.csrf || csrf;
  $('connection').textContent = s.available ? 'Connected' : 'Reconnecting';
  $('connection').classList.toggle('online', !!s.available);
  $('summary').textContent = s.available ? `${s.input} · ${s.mode} mode` : (s.error || 'Waiting for your soundbar…');
  $('volume').textContent = s.volume ?? '—';
  $('volume-caption').textContent = s.muted ? 'SOUND MUTED' : 'LISTENING LEVEL';
  $('power-state').textContent = s.power;
  $('power').classList.toggle('on', s.power === 'On');
  $('power').setAttribute('aria-label', s.power === 'On' ? 'Put soundbar in standby' : 'Turn soundbar on');
  $('mute').classList.toggle('active', !!s.muted);
  $('mute').setAttribute('aria-pressed', String(!!s.muted));
  $('mute').querySelector('span').textContent = s.muted ? 'Unmute sound' : 'Mute sound';
  $('input-name').textContent = s.input;
  document.querySelectorAll('[data-input]').forEach(b => {b.classList.toggle('selected', b.dataset.input === s.input);b.setAttribute('aria-pressed', String(b.dataset.input === s.input));});
  $('mode').textContent = s.mode === 'Standard' ? 'Standard selected' : s.mode === 'Smart' ? 'Smart Mode on' : 'Waiting for sound mode';
  $('mode-description').textContent = s.mode === 'Standard' ? 'Smart Mode is off.' : 'Select Standard for the original mix.';
  buttons.forEach(b => { b.disabled = busy || !s.available || !['On','Standby'].includes(s.power) || (b.dataset.action !== 'power' && s.power !== 'On') || (b.dataset.action === 'standard' && s.mode === 'Standard'); });
  $('standard').textContent = s.mode === 'Standard' ? 'Selected ✓' : 'Use Standard ↗';
}
async function refresh() {
  if (busy) return;
  try { const response = await fetch('/api/state', {cache: 'no-store'}); if (!response.ok) throw new Error('Connection unavailable'); render(await response.json()); }
  catch { $('connection').textContent = 'Offline'; $('connection').classList.remove('online'); buttons.forEach(b => b.disabled = true); }
}
buttons.forEach(button => button.addEventListener('click', async () => {
  if (busy || !csrf) return;
  busy = true; buttons.forEach(b => b.disabled = true);
  $('message').className = 'message';
  $('message').textContent = button.dataset.action === 'standard' ? 'Selecting Standard… audio may briefly mute.' : 'Sending…';
  try {
    const response = await fetch('/api/action', {method:'POST', headers:{'Content-Type':'application/json','X-CSRF-Token':csrf}, body:JSON.stringify({action:button.dataset.action})});
    const data = await response.json();
    if (!response.ok) throw new Error(data.error || 'Control unavailable');
    busy = false; render(data); $('message').textContent = 'Done.';
  } catch (error) { busy = false; if (current) render(current); $('message').className = 'message error'; $('message').textContent = error.message; }
  setTimeout(refresh, 600);
}));
refresh(); setInterval(refresh, 3000);
