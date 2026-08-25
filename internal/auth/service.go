package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const sessionTTL = 30 * 24 * time.Hour
const passwordIterations = 120000

type User struct {
	ID          int64   `json:"id"`
	Username    string  `json:"username"`
	DisplayName string  `json:"display_name"`
	Role        string  `json:"role"`
	Active      bool    `json:"active"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
	LastLoginAt *string `json:"last_login_at"`
	LibraryIDs  []int64 `json:"library_ids"`
}

type Session struct {
	ID          int64  `json:"id"`
	UserID      int64  `json:"user_id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	UserAgent   string `json:"user_agent"`
	IP          string `json:"ip"`
	CreatedAt   string `json:"created_at"`
	LastSeenAt  string `json:"last_seen_at"`
	ExpiresAt   string `json:"expires_at"`
}

type Service struct{ db *sql.DB }

func NewService(db *sql.DB) *Service { return &Service{db: db} }

func (s *Service) NeedsSetup(ctx context.Context) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count)
	return count == 0, err
}

func (s *Service) CreateFirstAdmin(ctx context.Context, username, displayName, password string) (User, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback()
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		return User{}, err
	}
	if count != 0 {
		return User{}, errors.New("StormFlix setup already completed")
	}
	user, err := createUserTx(ctx, tx, username, displayName, password, "admin", true)
	if err != nil {
		return User{}, err
	}
	if err := tx.Commit(); err != nil {
		return User{}, err
	}
	return user, nil
}

func (s *Service) Login(ctx context.Context, username, password, userAgent, ip string) (User, string, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	var u User
	var passwordHash string
	err := s.db.QueryRowContext(ctx, `SELECT id,username,display_name,password_hash,role,active,created_at,updated_at,last_login_at FROM users WHERE username=?`, username).Scan(
		&u.ID, &u.Username, &u.DisplayName, &passwordHash, &u.Role, &u.Active, &u.CreatedAt, &u.UpdatedAt, &u.LastLoginAt)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !verifyPassword(password, passwordHash)) {
		return User{}, "", errors.New("invalid username or password")
	}
	if err != nil {
		return User{}, "", err
	}
	if !u.Active {
		return User{}, "", errors.New("user is disabled")
	}
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return User{}, "", err
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	expires := time.Now().UTC().Add(sessionTTL).Format(time.RFC3339)
	if _, err := s.db.ExecContext(ctx, `INSERT INTO sessions(user_id,token_hash,user_agent,ip,expires_at) VALUES(?,?,?,?,?)`, u.ID, hashToken(token), userAgent, ip, expires); err != nil {
		return User{}, "", err
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE users SET last_login_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE id=?`, u.ID)
	u.LibraryIDs, _ = s.LibraryIDs(ctx, u.ID)
	return u, token, nil
}

func (s *Service) CurrentUser(ctx context.Context, token string) (User, error) {
	if token == "" {
		return User{}, sql.ErrNoRows
	}
	var u User
	err := s.db.QueryRowContext(ctx, `SELECT u.id,u.username,u.display_name,u.role,u.active,u.created_at,u.updated_at,u.last_login_at FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=? AND s.expires_at > CURRENT_TIMESTAMP AND u.active=1`, hashToken(token)).Scan(&u.ID, &u.Username, &u.DisplayName, &u.Role, &u.Active, &u.CreatedAt, &u.UpdatedAt, &u.LastLoginAt)
	if err != nil {
		return User{}, err
	}
	u.LibraryIDs, _ = s.LibraryIDs(ctx, u.ID)
	return u, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash=?`, hashToken(token))
	return err
}
func (s *Service) Cleanup(ctx context.Context) {
	_, _ = s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= CURRENT_TIMESTAMP`)
}

func (s *Service) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,username,display_name,role,active,created_at,updated_at,last_login_at FROM users ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.DisplayName, &u.Role, &u.Active, &u.CreatedAt, &u.UpdatedAt, &u.LastLoginAt); err != nil {
			return nil, err
		}
		u.LibraryIDs, _ = s.LibraryIDs(ctx, u.ID)
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Service) CreateUser(ctx context.Context, username, displayName, password, role string, active bool, libraryIDs []int64) (User, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback()
	u, err := createUserTx(ctx, tx, username, displayName, password, role, active)
	if err != nil {
		return User{}, err
	}
	if err := replaceLibraries(ctx, tx, u.ID, libraryIDs); err != nil {
		return User{}, err
	}
	if err := tx.Commit(); err != nil {
		return User{}, err
	}
	u.LibraryIDs = libraryIDs
	return u, nil
}

func (s *Service) UpdateUser(ctx context.Context, id int64, displayName, password, role string, active bool, libraryIDs []int64) (User, error) {
	if !validRole(role) {
		return User{}, errors.New("invalid role")
	}
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		return User{}, errors.New("display name is required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback()
	if password != "" {
		h, err := hashPassword(password)
		if err != nil {
			return User{}, err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE users SET display_name=?,password_hash=?,role=?,active=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, displayName, h, role, active, id); err != nil {
			return User{}, err
		}
	} else {
		if _, err = tx.ExecContext(ctx, `UPDATE users SET display_name=?,role=?,active=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, displayName, role, active, id); err != nil {
			return User{}, err
		}
	}
	if err := replaceLibraries(ctx, tx, id, libraryIDs); err != nil {
		return User{}, err
	}
	if err := tx.Commit(); err != nil {
		return User{}, err
	}
	return s.GetUser(ctx, id)
}

func (s *Service) GetUser(ctx context.Context, id int64) (User, error) {
	var u User
	err := s.db.QueryRowContext(ctx, `SELECT id,username,display_name,role,active,created_at,updated_at,last_login_at FROM users WHERE id=?`, id).Scan(&u.ID, &u.Username, &u.DisplayName, &u.Role, &u.Active, &u.CreatedAt, &u.UpdatedAt, &u.LastLoginAt)
	if err != nil {
		return User{}, err
	}
	u.LibraryIDs, _ = s.LibraryIDs(ctx, id)
	return u, nil
}
func (s *Service) DeleteUser(ctx context.Context, id int64) error {
	var admins int
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE role='admin' AND active=1`).Scan(&admins)
	var role string
	if err := s.db.QueryRowContext(ctx, `SELECT role FROM users WHERE id=?`, id).Scan(&role); err != nil {
		return err
	}
	if role == "admin" && admins <= 1 {
		return errors.New("cannot delete the last active administrator")
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE id=?`, id)
	return err
}
func (s *Service) LibraryIDs(ctx context.Context, userID int64) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT library_id FROM user_libraries WHERE user_id=? ORDER BY library_id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
func (s *Service) ListSessions(ctx context.Context) ([]Session, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT s.id,s.user_id,u.username,u.display_name,s.user_agent,s.ip,s.created_at,s.last_seen_at,s.expires_at FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.expires_at>CURRENT_TIMESTAMP ORDER BY s.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Session
	for rows.Next() {
		var v Session
		if err := rows.Scan(&v.ID, &v.UserID, &v.Username, &v.DisplayName, &v.UserAgent, &v.IP, &v.CreatedAt, &v.LastSeenAt, &v.ExpiresAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Service) RevokeSession(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id=?`, id)
	return err
}

func createUserTx(ctx context.Context, tx *sql.Tx, username, displayName, password, role string, active bool) (User, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	displayName = strings.TrimSpace(displayName)
	if len(username) < 3 {
		return User{}, errors.New("username must have at least 3 characters")
	}
	for _, r := range username {
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-') {
			return User{}, errors.New("username contains invalid characters")
		}
	}
	if displayName == "" {
		displayName = username
	}
	if !validRole(role) {
		return User{}, errors.New("invalid role")
	}
	h, err := hashPassword(password)
	if err != nil {
		return User{}, err
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO users(username,display_name,password_hash,role,active) VALUES(?,?,?,?,?)`, username, displayName, h, role, active)
	if err != nil {
		return User{}, fmt.Errorf("create user: %w", err)
	}
	id, _ := res.LastInsertId()
	return User{ID: id, Username: username, DisplayName: displayName, Role: role, Active: active}, nil
}
func replaceLibraries(ctx context.Context, tx *sql.Tx, userID int64, ids []int64) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM user_libraries WHERE user_id=?`, userID); err != nil {
		return err
	}
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO user_libraries(user_id,library_id) VALUES(?,?)`, userID, id); err != nil {
			return err
		}
	}
	return nil
}
func validRole(role string) bool {
	return role == "admin" || role == "manager" || role == "operator" || role == "user"
}
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
func hashPassword(password string) (string, error) {
	if len(password) < 8 {
		return "", errors.New("password must have at least 8 characters")
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := pbkdf2SHA256([]byte(password), salt, passwordIterations)
	return fmt.Sprintf("pbkdf2_sha256$%d$%s$%s", passwordIterations, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}
func verifyPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2_sha256" {
		return false
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter < 1 {
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
	got := pbkdf2SHA256([]byte(password), salt, iter)
	return len(got) == len(want) && subtle.ConstantTimeCompare(got, want) == 1
}
func pbkdf2SHA256(password, salt []byte, iterations int) []byte {
	mac := hmac.New(sha256.New, password)
	mac.Write(salt)
	mac.Write([]byte{0, 0, 0, 1})
	u := mac.Sum(nil)
	out := append([]byte(nil), u...)
	for i := 1; i < iterations; i++ {
		mac = hmac.New(sha256.New, password)
		mac.Write(u)
		u = mac.Sum(nil)
		for j := range out {
			out[j] ^= u[j]
		}
	}
	return out
}
