import {byId} from './dom.js';

// Local audio-sync draft and setup rendering; the shell schedules and sends.
export function createAdvanced(root, setupRoot=root) {
  const $ = id => byId(root,id);
  const setupById = id => byId(setupRoot,id);
  const maxSync=250;
  let draft=null;
  return {
    target:()=>draft,
    // Also discards text typed but not yet applied (disconnect, error, other control).
    clear(){draft=null;const input=$('av-sync-input');if(input)input.dataset.stale='true';},
    // Typed value, read on Enter or blur: rounded to 10 ms and limited to the
    // native range. Returns true when it differs from the current delay.
    type(state){
      const input=$('av-sync-input'), sync=state?.sound?.av_sync_ms, typed=Number(input.value);
      if(input.disabled || !Number.isInteger(sync)) return false;
      if(input.value.trim()==='' || !Number.isFinite(typed)){input.value=draft ?? sync;return false;}
      const value=Math.max(0,Math.min(maxSync,Math.round(typed/10)*10));
      input.value=value;
      draft=value===sync ? null : value;
      return draft!==null;
    },
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
      const input=$('av-sync-input');
      input.disabled=!ready || !awake || !!active || sync==null || !tvInput;
      if(input.dataset.stale || root.ownerDocument.activeElement!==input){input.value=shown ?? '';delete input.dataset.stale;}
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
