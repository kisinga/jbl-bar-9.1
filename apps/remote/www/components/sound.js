import {byId} from './dom.js';
import {createCenter} from './center.js';
import {cycleSteps, renderSegments, settleTarget} from './segmented.js';
const LEVELS={bass:{action:'bass_next',count:5},atmos:{action:'atmos_next',count:3},rear:{action:'rear_next',count:4}};
  export function createSound(root) {
  const $ = id => byId(root,id);
  const center=createCenter(root);
  const targets={};
  return {
    // Steps to reach a chosen segment; 0 when it is already selected.
    plan(button,state){
      const name=button.closest('[data-level]')?.dataset.level;
      if(name==='center') return center.plan(button,state);
      const level=LEVELS[name];
      if(level){const steps=cycleSteps(state?.sound?.[name], Number(button.dataset.value), level.count);if(steps)targets[name]=Number(button.dataset.value);return steps;}
      if(button.dataset.mode) return state?.mode===button.dataset.mode ? 0 : 1;
      return 1;
    },
    render(context){const {state:s,ready,awake,active,pending}=context;const sound=s.sound||{};
    $('mode').textContent = s.mode === 'Standard' ? 'Standard' : s.mode === 'Smart' ? 'Smart Mode' : 'Checking mode';
    $('mode-description').textContent = s.power === 'Standby' ? 'Turn on to adjust your sound.' : !awake ? 'Power state is unavailable.' : active === 'standard' || active === 'smart' ? 'Selecting… audio may briefly mute.' : s.mode === 'Standard' ? 'Smart Mode is off.' : s.mode === 'Smart' ? 'Smart Mode adapts the sound to the content.' : 'Sound mode is not reported.';
    // Standard | Smart: the current mode is selected, the other one is the control.
    const modeTarget = active === 'smart' ? 'Smart' : active === 'standard' ? 'Standard' : null;
    for (const b of root.querySelectorAll('#mode-select button')) {
      const selected = b.dataset.mode === s.mode;
      b.classList.toggle('selected', selected);
      b.classList.toggle('target', b.dataset.mode === modeTarget && !selected);
      b.setAttribute('aria-checked', String(selected));
    }
    $('audio-format-row').hidden = !awake || !sound.audio_format;
    $('audio-format').textContent = sound.audio_format || '—';
    $('night-value').textContent = sound.night == null ? 'Unavailable' : sound.night ? 'On' : 'Off';
    $('night-label').textContent = active === 'night_toggle' ? 'Changing…' : sound.night ? 'Turn off' : 'Turn on';
    $('night').setAttribute('aria-pressed', String(sound.night === true));
    $('night').disabled = !ready || !awake || !!active || sound.night == null;
    // A saved bass offset (set in Advanced) shifts every level: show it here too.
    const offset=s.bass_offset?.offset_db;
    const shifted=Number.isFinite(offset) && offset !== 0 ? ' · offset ' + (offset > 0 ? '+' : '') + offset.toFixed(1) + ' dB' : '';
    $('bass-value').textContent=sound.bass == null ? 'Unavailable' : 'Level ' + (sound.bass+1) + ' of 5' + shifted;
    $('atmos-value').textContent=['Low','Mid','High'][sound.atmos] || 'Unavailable';
    const rear=['Low','Mid','High','Muted'][sound.rear] || 'Unavailable';
    $('rear-value').textContent=sound.rear_docked ? rear + ' · speakers docked' : rear;
    for (const [name,level] of Object.entries(LEVELS)) {
      const value=sound[name], target=settleTarget(targets,name,value,level.action,pending);
      renderSegments(root.querySelector('[data-level="'+name+'"]'), value, target, !ready || !awake || !!active || value == null || (name==='rear' && sound.rear_docked !== false));
    }
  center.render(context);
  }};
}
