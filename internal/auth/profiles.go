package auth

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"strings"
)

type Profile struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"user_id"`
	Name      string `json:"name"`
	AvatarKey string `json:"avatar_key"`
	AvatarURL string `json:"avatar_url"`
	IsKids    bool   `json:"is_kids"`
	Active    bool   `json:"active"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func (s *Service) Profiles(ctx context.Context, userID int64) ([]Profile, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,user_id,name,avatar_key,avatar_url,is_kids,active,created_at,updated_at FROM profiles WHERE user_id=? ORDER BY id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Profile{}
	for rows.Next() {
		var p Profile
		if err := rows.Scan(&p.ID, &p.UserID, &p.Name, &p.AvatarKey, &p.AvatarURL, &p.IsKids, &p.Active, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Service) Profile(ctx context.Context, userID, profileID int64) (Profile, error) {
	var p Profile
	err := s.db.QueryRowContext(ctx, `SELECT id,user_id,name,avatar_key,avatar_url,is_kids,active,created_at,updated_at FROM profiles WHERE id=? AND user_id=?`, profileID, userID).
		Scan(&p.ID, &p.UserID, &p.Name, &p.AvatarKey, &p.AvatarURL, &p.IsKids, &p.Active, &p.CreatedAt, &p.UpdatedAt)
	return p, err
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
	res, err := s.db.ExecContext(ctx, `INSERT INTO profiles(user_id,name,avatar_key,avatar_url,is_kids) VALUES(?,?,?,?,?)`, userID, name, avatarKey, avatarURL, isKids)
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
	u, err := url.Parse(value)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return "", errors.New("avatar URL must be a valid http/https URL")
	}
	return value, nil
}
