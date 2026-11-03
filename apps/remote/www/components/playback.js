import {byId} from './dom.js';
export function createPlayback(root) {
  const $ = id => byId(root,id);
  let active=null, mediaIntent=null,mediaPeer='',mediaAudio='';
  function mediaIdentity(s) {
    if (!s?.available || s.power !== 'On') return '';
    const bt = s?.bluetooth;
    if (s?.source_state === 7 && s?.streaming?.service === 'AirPlay' && s.streaming.active) return 'airplay';
    return s?.input === 'Bluetooth' && bt?.connected ? JSON.stringify([bt.peer_name || '', bt.name || '']) : '';
  }
  function syncMediaIntent(s) {
    const peer = mediaIdentity(s), audio = peer === 'airplay' ? 'Session active' : s?.bluetooth?.audio || '';
    if (!peer || peer !== mediaPeer || audio !== mediaAudio) mediaIntent = null;
    mediaPeer = peer; mediaAudio = audio;
  }
  function mediaTime(value) {
    if (!Number.isFinite(value) || value < 0) return '';
    const seconds = Math.floor(value);
    return (seconds >= 3600 ? Math.floor(seconds / 3600) + ':' : '') + String(Math.floor(seconds / 60) % 60).padStart(seconds >= 3600 ? 2 : 1, '0') + ':' + String(seconds % 60).padStart(2, '0');
  }
  function renderPlayback(s, ready, awake) {
    const bt = s.bluetooth;
    const bluetooth = s.input === 'Bluetooth';
    const playback = bluetooth ? {
      service: 'Bluetooth',
      status: bt?.connected == null ? 'Status unavailable' : !bt.connected ? 'Waiting for a device' : bt.audio === 'Streaming' ? 'Streaming audio' : bt.audio === 'Paused' ? 'Stream suspended' : 'Connected',
      active: bt?.connected && bt.audio === 'Streaming',
    } : s.streaming;
    const cast = playback?.cast;
    $('playback').hidden = !awake || !playback;
    $('playback-icon').setAttribute('href', bluetooth ? '#i-bluetooth' : '#i-cast');
    $('playback-service').textContent = playback?.service || '';
    $('playback-status').textContent = playback?.status || '';
    const title = bluetooth ? (bt?.connected ? bt.peer_name || 'Connected device' : '') : cast?.title || '';
    $('playback-title').hidden = !title;
    $('playback-title').textContent = title;
    const metadata = bluetooth ? '' : [cast?.artist, cast?.album].filter(Boolean).join(' · ');
    $('playback-meta').hidden = !metadata;
    $('playback-meta').textContent = metadata;
    $('playback-app').hidden = !cast?.application;
    $('playback-app').textContent = cast?.application || '';
    $('playback-detail').hidden = !playback?.detail;
    $('playback-detail').textContent = playback?.detail || '';
    const timed = !cast?.mirroring && Number.isFinite(cast?.duration) && cast.duration > 0 && Number.isFinite(cast?.current_time);
    $('playback-timeline').hidden = !timed;
    $('playback-progress').max = timed ? cast.duration : 1;
    $('playback-progress').value = timed ? Math.min(cast.current_time, cast.duration) : 0;
    $('playback-time').textContent = timed ? mediaTime(cast.current_time) + ' / ' + mediaTime(cast.duration) : '';
    $('playback').classList.toggle('active', ready && awake && !!playback?.active);
    $('playback').classList.toggle('mirroring', !!cast?.mirroring);
    syncMediaIntent(s);
    const controls = s.media_controls || [];
    const airplay = playback?.service === 'AirPlay';
    const wantsPause = bluetooth ? (mediaIntent ? mediaIntent === 'media_play' : bt?.audio === 'Streaming') : airplay ? (mediaIntent ? mediaIntent === 'media_play' : !!playback.active) : cast?.player_state === 'PLAYING';
    let toggle = wantsPause ? 'media_pause' : 'media_play';
    if (!controls.includes(toggle) && controls.includes(wantsPause ? 'media_play' : 'media_pause')) toggle = wantsPause ? 'media_play' : 'media_pause';
    const label = toggle === 'media_pause' ? 'Pause' : 'Play';
    $('media-toggle').dataset.action = toggle;
    $('media-toggle').setAttribute('aria-label', label);
    $('media-toggle').title = label;
    $('media-toggle-label').textContent = label;
    $('media-toggle-icon').setAttribute('href', toggle === 'media_pause' ? '#i-pause' : '#i-play');
    for (const id of ['media-previous','media-toggle','media-next','media-stop']) {
      $(id).disabled = !ready || !awake || !!active || !controls.includes($(id).dataset.action);
    }
    $('media-help').textContent = controls.length ? (bluetooth ? 'Your app must support remote controls. Streaming may continue while an app is paused.' : airplay ? 'Sent to the AirPlay sender. Its play/pause state isn’t reported.' : 'Controls request changes from the receiver.') : cast?.mirroring ? 'Use playback controls in the sending app.' : bluetooth && bt?.connected ? 'Playback controls are unavailable for this connection.' : 'Playback controls aren’t available on this source.';
  }

  return {
   render(context) {active=context.active;renderPlayback(context.state,context.ready,context.awake);},
   identity:mediaIdentity,
   resetIntent(){ mediaIntent=null; },
   accepted(action,peer,state){syncMediaIntent(state);if(peer && peer===mediaIdentity(state) && ['media_play','media_pause','media_stop'].includes(action))mediaIntent=action;}
  };
}
