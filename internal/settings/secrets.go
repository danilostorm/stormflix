package settings

import "strings"

// SealSecret stores integration credentials with the same AES-GCM key used by
// runtime settings. Callers persist only the returned ciphertext.
func (s *Service) SealSecret(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	return s.encrypt(value)
}

// OpenSecret decrypts integration credentials previously returned by
// SealSecret. Empty values are kept empty for optional integrations.
func (s *Service) OpenSecret(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	return s.decrypt(value)
}
