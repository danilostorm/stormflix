package playback

// IsUHD includes cropped cinema sources (e.g. 3840x1600), whose decode cost
// cannot be classified from height alone. It also includes resolutions above 4K.
func IsUHD(width, height int) bool {
	return width >= 3200 || height >= 2000
}
