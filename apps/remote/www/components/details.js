import {byId} from './dom.js';
export function createDetails(root) {
  const $ = id => byId(root,id);
  function watcherName(value) {
    if (!value) return 'Not reported';
    if (value === 'PAUSING_LEGACY') return 'Starting automation';
    if (value === 'WAITING_FOR_READY') return 'Waiting for sound controls';
    if (['STANDARD_THIS_WAKE', 'ALREADY_STANDARD_THIS_WAKE'].includes(value)) return 'Standard selected for this wake';
    if (value.startsWith('WAITING_POWER_') || value === 'WAITING_FOR_POWER_ON') return 'Waiting for power-on';
    if (value === 'WAITING_FOR_PROCESS') return 'Waiting for the soundbar';
    if (value === 'SMART_KEPT_THIS_WAKE') return 'Smart Mode kept, as before standby';
    if (value === 'APPLYING_STANDARD') return 'Selecting Standard…';
    if (value.startsWith('AWAKE_STABILIZING_')) return 'Checking the mode after wake…';
    if (value.startsWith('PROCESS_')) return 'Starting the watcher';
    return 'Check automation logs';
  }
  return {render({state:s}){
    $('device-name').textContent = s.bluetooth?.name || 'Not reported';
    $('device-uptime').textContent = Number.isFinite(s.uptime) && s.uptime > 0 ? Math.floor(s.uptime / 86400) + 'd ' + Math.floor(s.uptime / 3600) % 24 + 'h ' + Math.floor(s.uptime / 60) % 60 + 'm' : 'Not reported';
    // With smart_mode_memory, also say which mode the next wake restores.
    const remembered = ['Standard','Smart'].includes(s.automation?.remembered_mode) ? ' · after standby: ' + s.automation.remembered_mode : '';
    $('device-automation').textContent = s.automation?.enabled ? (s.automation.error || 'Go · ' + watcherName(s.automation.status) + remembered) : watcherName(s.watcher);
    const cast = s.streaming?.cast;
    const gains = [cast?.low_shelf_gain, cast?.high_shelf_gain];
    const hasGains = gains.every(Number.isFinite);
    $('cast-eq-row').hidden = !hasGains;
    $('cast-eq').textContent = hasGains ? 'Low ' + gains[0] + ' dB · High ' + gains[1] + ' dB (read only)' : '';
  }};
}
