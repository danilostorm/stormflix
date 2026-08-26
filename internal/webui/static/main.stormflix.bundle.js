// StormFlix compatibility marker for the official Jellyfin Android WebView.
// The Jellyfin Android app watches for a main.*.bundle.js request to decide
// that the server web application has loaded successfully. The first request
// is intercepted by the app so it can inject NativeShell; the deferred request
// comes back here and intentionally remains a no-op for the StormFlix UI.
window.StormFlixJellyfinWebViewBridge = true;
