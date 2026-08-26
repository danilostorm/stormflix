package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

type Profile struct {
	ID                 int64  `json:"id"`
	UserID             int64  `json:"user_id"`
	Name               string `json:"name"`
	AvatarKey          string `json:"avatar_key"`
	AvatarURL          string `json:"avatar_url"`
	IsKids             bool   `json:"is_kids"`
	ContentRatingLimit int    `json:"content_rating_limit"`
	Active             bool   `json:"active"`
	PINEnabled         bool   `json:"pin_enabled"`
	AutoplayNext       bool   `json:"autoplay_next"`
	AutoplayPreviews   bool   `json:"autoplay_previews"`
	PreferredAudio     string `json:"preferred_audio"`
	PreferredSubtitle  string `json:"preferred_subtitle"`
	CreatedAt          string `json:"created_at"`
	UpdatedAt          string `json:"updated_at"`
}

const profileSelect = `SELECT id,user_id,name,avatar_key,avatar_url,is_kids,content_rating_limit,active,(pin_hash<>''),autoplay_next,autoplay_previews,preferred_audio,preferred_subtitle,created_at,updated_at FROM profiles`

func (s *Service) Profiles(ctx context.Context, userID int64) ([]Profile, error) {
	rows, err := s.db.QueryContext(ctx, profileSelect+` WHERE user_id=? ORDER BY id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Profile{}
	for rows.Next() {
		var p Profile
		if err := scanProfile(rows, &p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Service) Profile(ctx context.Context, userID, profileID int64) (Profile, error) {
	var p Profile
	row := s.db.QueryRowContext(ctx, profileSelect+` WHERE id=? AND user_id=?`, profileID, userID)
	err := scanProfile(row, &p)
	return p, err
}

type profileScanner interface{ Scan(...any) error }

func scanProfile(scanner profileScanner, p *Profile) error {
	return scanner.Scan(&p.ID, &p.UserID, &p.Name, &p.AvatarKey, &p.AvatarURL, &p.IsKids, &p.ContentRatingLimit, &p.Active, &p.PINEnabled, &p.AutoplayNext, &p.AutoplayPreviews, &p.PreferredAudio, &p.PreferredSubtitle, &p.CreatedAt, &p.UpdatedAt)
}

func (s *Service) CreateProfile(ctx context.Context, userID int64, name, avatarKey, avatarURL string, isKids bool) (Profile, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Profile{}, errors.New("profile name is required")
	}
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM profiles WHERE user_id=?`, userID).Scan(&count); err != nil {
		return Profile{}, err
	}
	if count >= 8 {
		return Profile{}, errors.New("maximum of 8 profiles per account")
	}
	avatarKey = normalizeAvatarKey(avatarKey)
	avatarURL, err := normalizeAvatarURL(avatarURL)
	if err != nil {
		return Profile{}, err
	}
	limit := 18
	if isKids {
		limit = 10
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO profiles(user_id,name,avatar_key,avatar_url,is_kids,content_rating_limit) VALUES(?,?,?,?,?,?)`, userID, name, avatarKey, avatarURL, isKids, limit)
	if err != nil {
		return Profile{}, err
	}
	id, _ := res.LastInsertId()
	return s.Profile(ctx, userID, id)
}

func (s *Service) UpdateProfile(ctx context.Context, userID, profileID int64, name, avatarKey, avatarURL string, isKids, active bool) (Profile, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Profile{}, errors.New("profile name is required")
	}
	avatarKey = normalizeAvatarKey(avatarKey)
	avatarURL, err := normalizeAvatarURL(avatarURL)
	if err != nil {
		return Profile{}, err
	}
	res, err := s.db.ExecContext(ctx, `UPDATE profiles SET name=?,avatar_key=?,avatar_url=?,is_kids=?,active=?,updated_at=CURRENT_TIMESTAMP WHERE id=? AND user_id=?`, name, avatarKey, avatarURL, isKids, active, profileID, userID)
	if err != nil {
		return Profile{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Profile{}, sql.ErrNoRows
	}
	return s.Profile(ctx, userID, profileID)
}

func (s *Service) SetProfileAvatarURL(ctx context.Context, userID, profileID int64, avatarURL string) (Profile, error) {
	avatarURL, err := normalizeAvatarURL(avatarURL)
	if err != nil {
		return Profile{}, err
	}
	res, err := s.db.ExecContext(ctx, `UPDATE profiles SET avatar_url=?,updated_at=CURRENT_TIMESTAMP WHERE id=? AND user_id=?`, avatarURL, profileID, userID)
	if err != nil {
		return Profile{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Profile{}, sql.ErrNoRows
	}
	return s.Profile(ctx, userID, profileID)
}

func (s *Service) SetProfileRatingLimit(ctx context.Context, userID, profileID int64, limit int) (Profile, error) {
	limit = normalizeRatingLimit(limit)
	res, err := s.db.ExecContext(ctx, `UPDATE profiles SET content_rating_limit=?,updated_at=CURRENT_TIMESTAMP WHERE id=? AND user_id=?`, limit, profileID, userID)
	if err != nil {
		return Profile{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Profile{}, sql.ErrNoRows
	}
	return s.Profile(ctx, userID, profileID)
}

func normalizeRatingLimit(value int) int {
	switch value {
	case 0, 10, 12, 14, 16, 18:
		return value
	default:
		return 18
	}
}

func (s *Service) UpdateProfilePreferences(ctx context.Context, userID, profileID int64, pin string, clearPIN, autoplayNext, autoplayPreviews bool, preferredAudio, preferredSubtitle string) (Profile, error) {
	preferredAudio = normalizeLanguagePreference(preferredAudio)
	preferredSubtitle = normalizeLanguagePreference(preferredSubtitle)
	var pinHash any = nil
	if clearPIN {
		pinHash = ""
	} else if strings.TrimSpace(pin) != "" {
		hash, err := hashProfilePIN(pin)
		if err != nil {
			return Profile{}, err
		}
		pinHash = hash
	}
	var res sql.Result
	var err error
	if pinHash == nil {
		res, err = s.db.ExecContext(ctx, `UPDATE profiles SET autoplay_next=?,autoplay_previews=?,preferred_audio=?,preferred_subtitle=?,updated_at=CURRENT_TIMESTAMP WHERE id=? AND user_id=?`, autoplayNext, autoplayPreviews, preferredAudio, preferredSubtitle, profileID, userID)
	} else {
		res, err = s.db.ExecContext(ctx, `UPDATE profiles SET pin_hash=?,autoplay_next=?,autoplay_previews=?,preferred_audio=?,preferred_subtitle=?,updated_at=CURRENT_TIMESTAMP WHERE id=? AND user_id=?`, pinHash, autoplayNext, autoplayPreviews, preferredAudio, preferredSubtitle, profileID, userID)
	}
	if err != nil {
		return Profile{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Profile{}, sql.ErrNoRows
	}
	return s.Profile(ctx, userID, profileID)
}

func (s *Service) VerifyProfilePIN(ctx context.Context, userID, profileID int64, pin string) error {
	var encoded string
	if err := s.db.QueryRowContext(ctx, `SELECT pin_hash FROM profiles WHERE id=? AND user_id=? AND active=1`, profileID, userID).Scan(&encoded); err != nil {
		return err
	}
	if encoded == "" {
		return nil
	}
	if !verifyProfilePIN(pin, encoded) {
		return errors.New("PIN inválido")
	}
	return nil
}

func (s *Service) DeleteProfile(ctx context.Context, userID, profileID int64) error {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM profiles WHERE user_id=?`, userID).Scan(&count); err != nil {
		return err
	}
	if count <= 1 {
		return errors.New("an account must keep at least one profile")
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM profiles WHERE id=? AND user_id=?`, profileID, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func normalizeAvatarKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "storm-red", "ocean-blue", "anime-pink", "matrix-green", "sunset-orange", "nebula-purple", "midnight", "kids-yellow":
		return value
	default:
		return "storm-red"
	}
}

func normalizeAvatarURL(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if strings.HasPrefix(value, "/assets/avatars/") {
		return value, nil
	}
	u, err := url.Parse(value)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return "", errors.New("avatar URL must be a valid http/https URL or a StormFlix avatar")
	}
	return value, nil
}

func normalizeLanguagePreference(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "pt-BR"
	}
	if len(value) > 16 {
		return value[:16]
	}
	return value
}

func validProfilePIN(pin string) bool {
	pin = strings.TrimSpace(pin)
	if len(pin) < 4 || len(pin) > 6 {
		return false
	}
	for _, r := range pin {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func hashProfilePIN(pin string) (string, error) {
	pin = strings.TrimSpace(pin)
	if !validProfilePIN(pin) {
		return "", errors.New("PIN must contain 4 to 6 digits")
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := pbkdf2SHA256([]byte(pin), salt, passwordIterations)
	return fmt.Sprintf("pbkdf2_pin$%d$%s$%s", passwordIterations, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

func verifyProfilePIN(pin, encoded string) bool {
	if !validProfilePIN(pin) {
		return false
	}
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2_pin" {
		return false
	}
	iterations, err := strconv.Atoi(parts[1])
	if err != nil || iterations < 1 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil {
		return false
	}
	got := pbkdf2SHA256([]byte(strings.TrimSpace(pin)), salt, iterations)
	if len(got) != len(want) {
		return false
	}
	var diff byte
	for i := range got {
		diff |= got[i] ^ want[i]
	}
	return diff == 0
}
