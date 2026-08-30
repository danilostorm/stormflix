// StormFlix bridge for the official Jellyfin Android WebView.
//
// Jellyfin Android loads the configured server root, intercepts the first
// main.*.bundle.js request and injects its NativeShell. The injected client then
// waits for a Sessions/Capabilities/Full request and reads `jellyfin_credentials`
// from localStorage. StormFlix keeps rendering its own Web UI; this deferred
// bundle only mirrors the already-authenticated StormFlix session into the
// credential shape expected by the official Android wrapper.
(() => {
  window.StormFlixJellyfinWebViewBridge = true;

  const nativeWrapper = Boolean(
    window.NativeInterface ||
    window.NativePlayer ||
    window.ExternalPlayer ||
    window.MediaSegments
  );
  if (!nativeWrapper) return;

  const credentialsKey = 'jellyfin_credentials';
  const bridgeURL = '/api/v1/compat/jellyfin-mobile-bridge';
  const capabilitiesURL = '/Sessions/Capabilities/Full';
  const origin = window.location.origin.replace(/\/$/, '');
  let syncing = false;
  let lastToken = '';

  function parseCredentials() {
    try {
      const raw = window.localStorage.getItem(credentialsKey);
      const parsed = raw ? JSON.parse(raw) : null;
      return parsed && Array.isArray(parsed.Servers) ? parsed : { Servers: [] };
    } catch (_) {
      return { Servers: [] };
    }
  }

  function belongsToThisServer(server) {
    if (!server || typeof server !== 'object') return false;
    const addresses = [server.Address, server.ManualAddress]
      .filter(Boolean)
      .map(value => String(value).replace(/\/$/, ''));
    return addresses.includes(origin);
  }

  function clearStormFlixCredentials() {
    const credentials = parseCredentials();
    const servers = credentials.Servers.filter(server => !belongsToThisServer(server));
    if (servers.length === credentials.Servers.length) return;
    if (servers.length === 0) {
      window.localStorage.removeItem(credentialsKey);
    } else {
      window.localStorage.setItem(credentialsKey, JSON.stringify({ ...credentials, Servers: servers }));
    }
    lastToken = '';
  }

  function storeCredentials(bridge) {
    const credentials = parseCredentials();
    const now = new Date().toISOString();
    const server = {
      Id: String(bridge.server_id || ''),
      Name: String(bridge.server_name || 'StormFlix'),
      Address: String(bridge.base_url || origin).replace(/\/$/, ''),
      ManualAddress: String(bridge.base_url || origin).replace(/\/$/, ''),
      UserId: String(bridge.user_id || ''),
      AccessToken: String(bridge.access_token || ''),
      DateLastAccessed: now,
      Version: String(bridge.version || '')
    };
    const others = credentials.Servers.filter(item => !belongsToThisServer(item));
    window.localStorage.setItem(credentialsKey, JSON.stringify({ ...credentials, Servers: [server, ...others] }));
  }

  async function notifyNativeShell(token) {
    await fetch(capabilitiesURL, {
      method: 'POST',
      credentials: 'same-origin',
      cache: 'no-store',
      headers: {
        'Accept': 'application/json',
        'Content-Type': 'application/json',
        'X-Emby-Token': token
      },
      body: '{}'
    });
  }

  async function syncAndroidWrapper() {
    if (syncing) return;
    syncing = true;
    try {
      const response = await fetch(bridgeURL, {
        credentials: 'same-origin',
        cache: 'no-store',
        headers: { 'Accept': 'application/json' }
      });
      if (response.status === 401) {
        clearStormFlixCredentials();
        return;
      }
      if (!response.ok) return;

      const bridge = await response.json();
      const token = String(bridge.access_token || '');
      const userId = String(bridge.user_id || '');
      if (!token || !userId) return;

      storeCredentials(bridge);
      if (token !== lastToken) {
        lastToken = token;
        try {
          await notifyNativeShell(token);
        } catch (_) {
          // The Android wrapper intercepts this request before the network in
          // normal operation. A transient network failure must not disturb the
          // StormFlix Web UI; the next poll retries after lastToken is cleared.
          lastToken = '';
        }
      }
    } catch (_) {
      // Connection/login transitions are expected while this bridge polls.
    } finally {
      syncing = false;
    }
  }

  syncAndroidWrapper();
  window.setInterval(syncAndroidWrapper, 1500);
  document.addEventListener('visibilitychange', () => {
    if (!document.hidden) syncAndroidWrapper();
  });
})();
