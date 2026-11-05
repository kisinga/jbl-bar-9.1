import {byId} from './dom.js';

// Pure presentation of reported state. No connections, timers, or controls.
export function createSpeakers(root) {
  const $ = id => byId(root,id);
  return {render({state:s,ready,awake}) {
    const sound=s.sound || {};
    const docked=ready && typeof sound.rear_docked==='boolean' ? sound.rear_docked : null;
    root.dataset.state=docked===null?'unknown':docked?'docked':'detached';
    root.querySelector('.rear-caption').textContent=docked===null?'Docking state unavailable':docked?'Rear speakers docked':'Rear speakers detached';
    root.classList.toggle('inactive',!ready || !awake);
    for (const [side,label] of [['left','Left'],['right','Right']]) {
      const value=sound['rear_battery_'+side];
      const known=ready && Number.isInteger(value) && value>=0 && value<=100;
      $('rear-battery-'+side).textContent=label+': '+(known?value+'%':'Unavailable');
      const meter=$('rear-battery-'+side+'-meter');
      meter.hidden=!known;
      meter.value=known?value:0;
      meter.dataset.level=!known?'unknown':value<20?'critical':value<40?'low':'normal';
    }
    // This field is only the current display message, not persistent telemetry.
    const message=s.setup?.subwoofer || '';
    const pairing=s.setup?.sub_pairing===true;
    $('sub-render-status').textContent=!ready?'Subwoofer · status unavailable':pairing?'Subwoofer · pairing':message?'Subwoofer · '+message.toLowerCase():'Subwoofer';
    const bass=sound.bass;
    const knownBass=ready && Number.isInteger(bass) && bass>=0 && bass<5;
    const showBass=knownBass && awake && s.muted===false && !pairing && !['Not connected','Reconnecting'].includes(message);
    root.querySelector('.bass-rings').hidden=!showBass;
    root.querySelector('.bass-caption').hidden=!knownBass;
    $('bass-render-level').textContent=knownBass?(bass+1)+' / 5':'—';
    root.style.setProperty('--bass-opacity',showBass?String(.04+(bass+1)*.025):'0');
    root.querySelector('.bar-display').textContent=!ready || !awake?'':({'TV / ARC':'TV',HDMI:'HDMI',Bluetooth:'BT',Chromecast:'CAST',AirPlay:'AIR'}[s.input] || '');
  }};
}
