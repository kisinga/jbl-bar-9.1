import {byId} from './dom.js';
export function createCenter(root) {
  const $ = id => byId(root,id);
  return {render({state:s,ready,awake,active}) {const sound=s.sound||{};
    $('center-value').textContent=sound.center == null ? 'Unavailable' : 'Level ' + sound.center + ' of 5 · ' + (sound.center ? '+' + ['0','2.4','4.8','7.2','9.6','12'][sound.center] + ' dB' : 'original') + (sound.center_error ? ' · ' + sound.center_error : '');
  $('center-next').disabled=!ready||!awake||!!active||sound.center==null;
  }};
}
