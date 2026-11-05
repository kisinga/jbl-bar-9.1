import {byId} from './dom.js';

// Local audio-sync draft and setup rendering; the shell schedules and sends.
export function createAdvanced(root, setupRoot=root) {
  const $ = id => byId(root,id);
  const setupById = id => byId(setupRoot,id);
  const maxSync=250;
  let draft=null;
  return {
    target:()=>draft,
    clear(){draft=null;},
    adjust(action,state){
      const base=draft ?? state?.sound?.av_sync_ms;
      if(!Number.isInteger(base)) return false;
      draft=Math.max(0,Math.min(maxSync,base+(action==='av_sync_up'?10:-10)));
      return true;
    },
    render({state:s,ready,awake,active}){
      const sound=s.sound || {};
      const sync=sound.av_sync_ms, tvInput=s.source_state===0 || s.source_state===3;
      const shown=draft ?? sync;
      $('av-sync-value').textContent=sync==null ? 'Unavailable' : shown+' ms'+(draft!=null && draft!==sync ? ' (now '+sync+' ms)' : '')+(tvInput?'':' · select TV or HDMI to adjust');
      $('av-sync-down').disabled=!ready || !awake || !!active || sync==null || !tvInput || shown<=0;
      $('av-sync-up').disabled=!ready || !awake || !!active || sync==null || !tvInput || shown>=maxSync;
      const setup=s.setup, calibration=setup?.calibration || '';
      const measuring=/starting|Starting in|Measuring/.test(calibration);
      if(setupRoot!==root) setupRoot.hidden=!setup;
      for(const row of setupRoot.querySelectorAll('.setup-row')) row.hidden=!setup;
      setupById('calibration-value').textContent=calibration || (sound.rear_docked===false?'Ready':'Undock both rear speakers first');
      setupById('calibrate').disabled=!ready || !awake || !!active || !setup || measuring || setup.sub_pairing || sound.rear_docked!==false;
      setupById('sub-value').textContent=setup?.sub_pairing?'Pairing… hold the subwoofer button':setup?.subwoofer || '—';
      setupById('sub-pair').disabled=!ready || !awake || !!active || !setup || measuring || setup.sub_pairing;
    }
  };
}
