(function () {
  'use strict';

  var DEFAULT_SERVER = 'https://stormflix.cloud';
  var STORAGE_KEY = 'stormflix.tv.server';
  var input = document.getElementById('server');
  var form = document.getElementById('server-form');
  var status = document.getElementById('status');
  var connect = document.getElementById('connect');

  function normalizeServer(value) {
    value = String(value || '').trim();
    if (!value) return '';
    if (!/^https?:\/\//i.test(value)) value = 'https://' + value;
    try {
      var parsed = new URL(value);
      if (parsed.protocol !== 'https:' && parsed.protocol !== 'http:') return '';
      return parsed.origin.replace(/\/$/, '');
    } catch (_) {
      return '';
    }
  }

  function registerRemoteKeys() {
    try {
      if (!window.tizen || !tizen.tvinputdevice) return;
      [
        'MediaPlayPause', 'MediaPlay', 'MediaPause', 'MediaStop',
        'MediaRewind', 'MediaFastForward', 'MediaTrackPrevious', 'MediaTrackNext'
      ].forEach(function (key) {
        try { tizen.tvinputdevice.registerKey(key); } catch (_) {}
      });
    } catch (_) {}
  }

  function openStormFlix(value) {
    var server = normalizeServer(value);
    if (!server) {
      status.textContent = 'Digite um endereço válido.';
      input.focus();
      return;
    }
    localStorage.setItem(STORAGE_KEY, server);
    status.textContent = 'Abrindo StormFlix...';
    connect.disabled = true;
    window.location.replace(server + '/?stormflix_tv=1&platform=tizen&tv_shell=1');
  }

  var saved = normalizeServer(localStorage.getItem(STORAGE_KEY));
  input.value = saved || DEFAULT_SERVER;
  registerRemoteKeys();

  form.addEventListener('submit', function (event) {
    event.preventDefault();
    openStormFlix(input.value);
  });

  document.addEventListener('keydown', function (event) {
    if (event.keyCode === 10009) {
      event.preventDefault();
      try { tizen.application.getCurrentApplication().exit(); } catch (_) {}
    }
  });

  // This deployment has a canonical public endpoint, so normal launches go
  // straight into the hosted StormFlix UI. The form remains available when the
  // saved server was cleared or the user returns to the packaged launcher.
  window.setTimeout(function () {
    if (normalizeServer(input.value)) openStormFlix(input.value);
  }, 350);
})();
