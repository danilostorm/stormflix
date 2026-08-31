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
    window.location.replace(server + '/?stormflix_tv=1&platform=webos&tv_shell=1');
  }

  var saved = normalizeServer(localStorage.getItem(STORAGE_KEY));
  input.value = saved || DEFAULT_SERVER;

  form.addEventListener('submit', function (event) {
    event.preventDefault();
    openStormFlix(input.value);
  });

  document.addEventListener('keydown', function (event) {
    // LG webOS Back. Once the hosted UI is loaded, StormFlix tv-remote.js owns
    // this same code and closes menus/player before the platform exits.
    if (event.keyCode === 461) {
      event.preventDefault();
      try {
        if (window.webOS && webOS.platformBack) webOS.platformBack();
      } catch (_) {}
    }
  });

  window.setTimeout(function () {
    if (normalizeServer(input.value)) openStormFlix(input.value);
  }, 350);
})();
