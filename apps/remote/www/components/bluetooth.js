import {byId} from './dom.js';
export function createBluetooth(root) {
  const $ = id => byId(root,id);
  let active=null,deviceName='your soundbar';
  function renderBluetooth(s, ready, awake) {
    const selected = s.input === 'Bluetooth';
    const bt = s.bluetooth;
    $('bluetooth-panel').hidden = !selected && !bt?.connected && active !== 'input_bluetooth';
    const name = bt?.name || deviceName;
    const pairing = active === 'bluetooth_pair';
    const discoverable = !!bt?.discoverable && !!bt?.connectable;
    $('bluetooth-panel').classList.toggle('discoverable', discoverable && ready && awake);
    $('bt-status').textContent = active === 'input_bluetooth' ? 'Switching to Bluetooth…' : pairing ? 'Starting pairing…' : !ready ? 'Connection unavailable' : !awake ? (s.power === 'Standby' ? 'Soundbar in standby' : 'Power state unavailable') : discoverable ? 'Ready to pair' : bt?.connected ? (selected && bt.audio === 'Streaming' ? 'Streaming audio' : selected && bt.audio === 'Paused' ? 'Stream suspended' : 'Device connected') : 'Bluetooth source selected';
    const peer = bt?.peer_name || 'A Bluetooth device';
    $('bt-help').textContent = !awake ? (s.power === 'Standby' ? 'Turn on the bar to connect a device.' : 'Power state is not reported; pairing is unavailable.') : !selected ? peer + ' is connected. Select Bluetooth above to listen.' : discoverable ? 'Choose ' + name + ' in your phone’s Bluetooth settings. The bar is discoverable now.' : bt?.connected ? peer + ' is connected. To use a different device, start pairing below.' : 'Open Bluetooth settings on your phone and choose ' + name + '. If it’s missing, start pairing below.';
    $('bt-pair-label').textContent = pairing ? 'Starting…' : discoverable ? 'Pairing is open' : bt?.connected ? 'Pair another device' : 'Pair a device';
    $('bt-pair').disabled = !ready || !awake || !selected || !!active || discoverable || !bt;
    $('bt-note').textContent = !selected ? 'Select Bluetooth above before starting pairing.' : !bt ? 'Bluetooth status is unavailable on this firmware.' : discoverable ? 'Waiting for a device. This status updates automatically.' : 'Pairing may disconnect the current Bluetooth device.';
  }
  return {render(context){active=context.active;deviceName=context.deviceName;renderBluetooth(context.state,context.ready,context.awake);}};
}
