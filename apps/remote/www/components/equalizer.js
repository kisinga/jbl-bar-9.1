import {byId} from './dom.js';
export function createEqualizer(root) {
  const $ = id => byId(root,id);
  let current, online=false, active=null, queued=0, csrf='';
  let eqDraft=null, eqCommitted='';
  const eqSliders=Array.from({length:5},(_,i)=>$('eq-slider-'+i));
  const eqPresets = Object.freeze({
    original: Object.freeze([0,0,0,0,0]),
    less_bass: Object.freeze([-4,-2,0,0,0]),
    clear_dialogue: Object.freeze([-3,-1,0,0,-1]),
    softer_treble: Object.freeze([0,0,0,-2,-3]),
    warm: Object.freeze([0,0,-1,-2,-3]),
    bright: Object.freeze([-3,-2,-1,0,0]),
    vocal_focus: Object.freeze([-4,-2,0,-1,-2]),
  });
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
    const pending = !!active || queued > 0;
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
    $('eq-reset').disabled = !ready || !awake || pending || !(eq.enabled === true || eq.status === 'reset_required' || eq.kept === true);
    $('eq-reset').textContent = active === 'eq_reset' ? 'Resetting…' : 'Reset';
    $('eq-panel').dataset.state = !ready ? 'offline' : eq.status === 'reset_required' ? 'reset-required' : !eq.available || !valid ? 'unavailable' : changed ? 'draft' : eq.enabled ? 'custom' : 'original';
    $('eq-state').textContent = !online ? 'Offline. Reconnect to adjust EQ.' : !ready ? 'Waiting for the soundbar.' : !awake ? (s.power === 'Standby' ? 'Turn on the bar to adjust EQ.' : 'Power state is unavailable.') : eq.status === 'reset_required' ? 'Reset required before making further changes.' : eq.error ? eq.error : !eq.available || !valid ? 'EQ is unavailable on this firmware or audio path.' : active === 'eq_apply' ? 'Applying your settings…' : active === 'eq_reset' ? 'Restoring original processing…' : changed ? 'Unapplied changes. Select Apply EQ to listen.' : eq.enabled ? (eq.kept ? 'Custom EQ active. It stays through sound, input and power changes until Reset.' : 'Custom EQ active. Reset restores original processing.') : 'Original tuning. Adjust a band, then apply.';
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

  return {
   render(context) { ({state:current,online,active,queued,csrf}=context); renderEQ(current,context.ready,context.awake); },
   gains() { return validEQGains(eqDraft) ? eqDraft.slice() : null; }
  };
}
