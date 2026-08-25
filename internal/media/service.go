package media

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

type Item struct {
	ID           int64  `json:"id"`
	LibraryID    int64  `json:"library_id"`
	Title        string `json:"title"`
	Extension    string `json:"extension"`
	SizeBytes    int64  `json:"size_bytes"`
	ModifiedUnix int64  `json:"modified_unix"`
	Available    bool   `json:"available"`
}
type StreamItem struct {
	Item
	Path string
}
type Service struct{ db *sql.DB }

func NewService(db *sql.DB) *Service { return &Service{db: db} }

// allowedLibraryIDs: nil means unrestricted; non-nil empty means no access.
func (s *Service) List(ctx context.Context, libraryID int64, query string, limit, offset int, allowedLibraryIDs []int64) ([]Item, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	if allowedLibraryIDs != nil && len(allowedLibraryIDs) == 0 {
		return []Item{}, nil
	}
	args := []any{}
	sqlText := `SELECT id,library_id,title,extension,size_bytes,modified_unix,available FROM media WHERE available=1`
	if libraryID > 0 {
		sqlText += ` AND library_id=?`
		args = append(args, libraryID)
	}
	if allowedLibraryIDs != nil {
		marks := make([]string, len(allowedLibraryIDs))
		for i, id := range allowedLibraryIDs {
			marks[i] = "?"
			args = append(args, id)
		}
		sqlText += ` AND library_id IN (` + strings.Join(marks, ",") + `)`
	}
	if query != "" {
		sqlText += ` AND title LIKE ?`
		args = append(args, "%"+query+"%")
	}
	sqlText += ` ORDER BY title COLLATE NOCASE LIMIT ? OFFSET ?`
	args = append(args, limit, offset)
	rows, err := s.db.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Item
	for rows.Next() {
		var v Item
		if err := rows.Scan(&v.ID, &v.LibraryID, &v.Title, &v.Extension, &v.SizeBytes, &v.ModifiedUnix, &v.Available); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Service) GetStreamItem(ctx context.Context, id int64) (StreamItem, error) {
	var v StreamItem
	err := s.db.QueryRowContext(ctx, `SELECT id,library_id,title,path,extension,size_bytes,modified_unix,available FROM media WHERE id=?`, id).Scan(&v.ID, &v.LibraryID, &v.Title, &v.Path, &v.Extension, &v.SizeBytes, &v.ModifiedUnix, &v.Available)
	if errors.Is(err, sql.ErrNoRows) {
		return StreamItem{}, sql.ErrNoRows
	}
	return v, err
}
func ContainsLibrary(ids []int64, id int64) bool {
	if ids == nil {
		return true
	}
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}
func (s *Service) Count(ctx context.Context) (int64, error) {
	var n int64
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media WHERE available=1`).Scan(&n)
	return n, err
}
func (s *Service) DeleteCatalogItem(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM media WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("media not found")
	}
	return nil
}
