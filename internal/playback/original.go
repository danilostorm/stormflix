package playback

// OriginalRequest is the StormFlix playback policy. Compatibility must happen
// on the client; the v6 decoder requires server remuxing and is not eligible.
// Kept explicit so the isolated Jellyfin facade retains its own contract.
func OriginalRequest(request Request) Request {
	request.OriginalOnly = true
	request.Quality = "original"
	request.Capabilities.AllowRemux = false
	request.Capabilities.AllowAudioCompatibility = false
	request.Capabilities.AllowVideoTranscode = false
	request.LocalDecode.HEVCWASM = false
	request.LocalDecode.AV1WASM = false
	return request
}
