package transcode

import "time"

// Touch keeps an active transcode session alive from normal playback
// heartbeats without requiring another media probe or cache operation.
func (m *Manager) Touch(userID int64, id string) bool {
	if !IsSessionID(id) {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.sessions[id]
	if s == nil || s.UserID != userID {
		return false
	}
	s.LastTouch = time.Now()
	return true
}
