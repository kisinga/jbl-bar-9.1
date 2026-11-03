import {byId} from './dom.js';
import {cycleSteps, renderSegments, settleTarget} from './segmented.js';
export function createCenter(root) {
  const $ = id => byId(root,id);
  const targets={};
  const group=()=>root.querySelector('[data-level="center"]');
  return {
    // Steps to reach the chosen center level. The bar reports 0 (original) to
    // 5; the page shows them as levels 1 to 6.
    plan(button,state) {
      const steps=cycleSteps(state?.sound?.center, Number(button.dataset.value), 6);
      if(steps) targets.center=Number(button.dataset.value);
      return steps;
    },
    render({state:s,ready,awake,active,pending}) {const sound=s.sound||{};
    $('center-value').textContent=sound.center == null ? 'Unavailable' : 'Level ' + (sound.center+1) + ' of 6 · ' + (sound.center ? '+' + ['0','2.4','4.8','7.2','9.6','12'][sound.center] + ' dB' : 'original') + (sound.center_error ? ' · ' + sound.center_error : '');
    const target=settleTarget(targets,'center',sound.center,'center_next',pending);
    renderSegments(group(), sound.center, target, !ready||!awake||!!active||sound.center==null);
  }};
}
