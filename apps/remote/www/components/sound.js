import {byId} from './dom.js';
import {createCenter} from './center.js';
  export function createSound(root) {
  const $ = id => byId(root,id);
  const center=createCenter(root);
  return {render(context){const {state:s,ready,awake,active}=context;const sound=s.sound||{};
    $('mode').textContent = s.mode === 'Standard' ? 'Standard' : s.mode === 'Smart' ? 'Smart Mode' : 'Checking mode';
    $('mode-description').textContent = s.power === 'Standby' ? 'Turn on to adjust your sound.' : !awake ? 'Power state is unavailable.' : s.mode === 'Standard' ? 'Smart Mode is off. Use Smart to turn it back on.' : s.mode === 'Smart' ? 'Use Standard to turn Smart Mode off.' : 'Sound mode is not reported.';
    $('standard').dataset.action = s.mode === 'Standard' ? 'smart' : 'standard';
    $('standard-label').textContent = active === 'standard' || active === 'smart' ? 'Selecting…' : s.mode === 'Standard' ? 'Use Smart' : 'Use Standard';
    $('standard').classList.toggle('confirmed', false);
    $('audio-format-row').hidden = !awake || !sound.audio_format;
    $('audio-format').textContent = sound.audio_format || '—';
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
  center.render(context);
  }};
}
