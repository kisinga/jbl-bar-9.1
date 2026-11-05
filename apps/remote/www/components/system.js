import {byId} from './dom.js';
export function createSystem(root) {
  const $ = id => byId(root,id);
  // History belongs to this visible page, and advances only for real server samples.
  const monitorHistory = [];
  let monitorSample = null, monitorReceived = 0;
  const finitePositive = value => Number.isFinite(value) && value > 0;
  const finiteNonnegative = value => Number.isFinite(value) && value >= 0;
  function memoryReading(value) {
    if (!value || !finitePositive(value.total_bytes) || !finiteNonnegative(value.available_bytes) || !finiteNonnegative(value.used_bytes) || value.available_bytes > value.total_bytes || value.used_bytes > value.total_bytes) return null;
    return value;
  }
  function monitorBytes(value) { return (value / 1048576).toLocaleString(undefined, {maximumFractionDigits: 1}) + ' MiB'; }
  function monitorRate(value) {
    if (!finiteNonnegative(value)) return '—';
    const unit = value >= 1048576 ? 1048576 : value >= 1024 ? 1024 : 1;
    return (value / unit).toLocaleString(undefined, {maximumFractionDigits: unit === 1 ? 0 : 1}) + (unit === 1048576 ? ' MiB/s' : unit === 1024 ? ' KiB/s' : ' B/s');
  }
  function monitorUptime(value) {
    if (!finiteNonnegative(value)) return '—';
    if (value < 60) return Math.floor(value) + 's';
    return [value >= 86400 ? Math.floor(value / 86400) + 'd' : '', value >= 3600 ? Math.floor(value / 3600) % 24 + 'h' : '', Math.floor(value / 60) % 60 + 'm'].filter(Boolean).join(' ');
  }
  function drawMonitorChart(id, key, label) {
    let line = '', area = '', segment = [];
    const finish = () => {
      if (!segment.length) return;
      line += 'M' + segment.map(p => p.join(',')).join('L');
      if (segment.length > 1) area += 'M' + segment[0][0] + ',72L' + segment.map(p => p.join(',')).join('L') + 'L' + segment.at(-1)[0] + ',72Z';
      segment = [];
    };
    for (const point of monitorHistory) {
      if (!Number.isFinite(point[key])) { finish(); continue; }
      const x = Math.max(0, 300 - (monitorReceived - point.time) / 120000 * 300);
      segment.push([Number(x.toFixed(2)), Number((70 - point[key] * .68).toFixed(2))]);
    }
    finish();
    $(id + '-line').setAttribute('d', line);
    $(id + '-area').setAttribute('d', area);
    const last = monitorHistory.at(-1);
    $(id + '-point').toggleAttribute('hidden', !Number.isFinite(last?.[key]));
    if (Number.isFinite(last?.[key])) { $(id + '-point').setAttribute('cx', 300); $(id + '-point').setAttribute('cy', 70 - last[key] * .68); }
    const valid = monitorHistory.filter(point => Number.isFinite(point[key]));
    $(id + '-chart').dataset.samples = valid.length;
    $(id + '-chart').setAttribute('aria-label', label + ' history: ' + (valid.length ? valid.length + ' real samples, latest ' + Math.round(valid.at(-1)[key]) + '%' : 'no samples available'));
  }
  function renderSystem(s, connected) {
    const reported = s.system;
    const valid = connected && Number.isSafeInteger(reported?.sample) && reported.sample >= 0;
    const now = Date.now();
    if (!valid) { monitorHistory.length = 0; monitorSample = null; monitorReceived = 0; }
    const data = valid ? reported : {};
    const cpu = finiteNonnegative(data.cpu_percent) && data.cpu_percent <= 100 ? data.cpu_percent : null;
    const memory = memoryReading(data.memory), storage = memoryReading(data.storage);
    const ram = memory ? memory.used_bytes / memory.total_bytes * 100 : null;
    if (valid && data.sample !== monitorSample) {
      if (monitorSample !== null && (data.sample < monitorSample || now - monitorReceived > 6500)) monitorHistory.length = 0;
      monitorSample = data.sample; monitorReceived = now;
      monitorHistory.push({time: now, cpu, ram});
      while (monitorHistory.length > 60 || now - monitorHistory[0].time > 120000) monitorHistory.shift();
    }
    const stale = valid && now - monitorReceived > 6500;
    $('system-panel').dataset.state = !connected ? 'offline' : stale ? 'stale' : valid ? 'live' : 'waiting';
    $('system-status').textContent = !connected ? 'Offline' : stale ? 'Waiting for new data' : valid ? 'Live · every 2s' : 'Not available';
    $('cpu-value').textContent = cpu === null ? '—' : Math.round(cpu) + '%';
    $('cpu-detail').textContent = Number.isSafeInteger(data.cpu_cores) && data.cpu_cores > 0 ? data.cpu_cores + (data.cpu_cores === 1 ? ' core' : ' cores') : 'Cores not reported';
    $('memory-value').textContent = ram === null ? '—' : Math.round(ram) + '%';
    $('memory-detail').textContent = memory ? monitorBytes(memory.used_bytes) + ' / ' + monitorBytes(memory.total_bytes) : 'Usage not reported';
    $('memory-available').textContent = memory ? monitorBytes(memory.available_bytes) + ' available' + (memory.estimated_available ? ' · estimated' : '') : 'Available memory not reported';
    $('history-count').textContent = monitorHistory.length ? monitorHistory.length === 1 ? 'Collecting history' : monitorHistory.length + ' samples · 2 min window' : 'No samples yet';
    drawMonitorChart('cpu', 'cpu', 'CPU'); drawMonitorChart('memory', 'ram', 'RAM');
    $('storage-progress').hidden = !storage;
    $('storage-progress').value = storage ? storage.used_bytes / storage.total_bytes * 100 : 0;
    $('storage-detail').textContent = storage ? Math.round(storage.used_bytes / storage.total_bytes * 100) + '% used' : '—';
    $('storage-value').textContent = storage ? monitorBytes(storage.available_bytes) + ' free of ' + monitorBytes(storage.total_bytes) : 'Usage not reported';
    $('network-receive').textContent = monitorRate(data.network?.receive_bytes_per_second);
    $('network-transmit').textContent = monitorRate(data.network?.transmit_bytes_per_second);
    $('system-uptime').textContent = monitorUptime(data.uptime_seconds);
    $('system-load').textContent = Array.isArray(data.load_average) && data.load_average.length === 3 && data.load_average.every(finiteNonnegative) ? data.load_average.map(value => value.toFixed(2)).join(' / ') : '—';
    $('system-processes-row').hidden = !Number.isSafeInteger(data.processes) || data.processes < 0;
    $('system-processes').textContent = data.processes ?? '—';
    $('system-temperature-row').hidden = !Number.isFinite(data.temperature_c);
    $('system-temperature').textContent = Number.isFinite(data.temperature_c) ? data.temperature_c.toFixed(1) + ' °C' : '—';
    $('system-note').textContent = !connected ? 'Connection lost. Vitals will resume with fresh samples.' : stale ? 'These readings have stopped updating. Waiting for a fresh sample.' : valid ? 'History builds while this page is open.' : 'System vitals are not available from this version of the remote.';
    const wifi=s.wifi || {};
    $('wifi-name').textContent=wifi.available ? wifi.connected ? wifi.ssid || 'Connected network' : 'Not connected' : 'Status unavailable';
    const signal = wifi.signal_dbm;
    const quality = signal == null ? '' : signal >= -55 ? 'Strong signal' : signal >= -67 ? 'Good signal' : signal >= -75 ? 'Fair signal' : 'Weak signal';
    $('wifi-status').textContent=wifi.connected ? ['Connected', quality].filter(Boolean).join(' · ') : wifi.hotspot_ip ? 'Setup hotspot is active' : wifi.available ? 'Waiting for a Wi-Fi connection' : 'The Wi-Fi service has not reported its state.';
    $('wifi-ip').textContent=wifi.ip || 'No network address';
    $('local-address').hidden = s.hostname !== 'jbl.local';
    const band=wifi.frequency_mhz ? (wifi.frequency_mhz>=5925?'6 GHz':wifi.frequency_mhz>=4900?'5 GHz':'2.4 GHz') : '';
    $('wifi-signal').textContent=[wifi.signal_dbm==null?'':wifi.signal_dbm+' dBm',band].filter(Boolean).join(' · ');
    $('hotspot-status').hidden=!wifi.hotspot_ip;
    $('hotspot-status').textContent=wifi.hotspot_ip ? 'Local hotspot control: http://' + wifi.hotspot_ip : '';

  }

  // A stalled stream can stay open: age the label without fetching or adding samples.




  return {
   render(context){renderSystem(context.state,context.online);},
   tick(context){if(!document.hidden && monitorReceived && Date.now()-monitorReceived>6500)renderSystem(context.state,context.online);}
  };
}
