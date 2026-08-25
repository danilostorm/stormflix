package media

import (
	"context"
	"database/sql"
	"errors"
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

type Service struct {
	db *sql.DB
}

func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

func (s *Service) List(ctx context.Context, libraryID int64, query string, limit, offset int) ([]Item, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	args := []any{}
	sqlText := `SELECT id, library_id, title, extension, size_bytes, modified_unix, available FROM media WHERE available = 1`
	if libraryID > 0 {
		sqlText += ` AND library_id = ?`
		args = append(args, libraryID)
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

	var items []Item
	for rows.Next() {
		var item Item
		if err := rows.Scan(&item.ID, &item.LibraryID, &item.Title, &item.Extension, &item.SizeBytes, &item.ModifiedUnix, &item.Available); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) GetStreamItem(ctx context.Context, id int64) (StreamItem, error) {
	var item StreamItem
	err := s.db.QueryRowContext(ctx, `
SELECT id, library_id, title, path, extension, size_bytes, modified_unix, available
FROM media WHERE id = ?`, id).Scan(
		&item.ID, &item.LibraryID, &item.Title, &item.Path, &item.Extension,
		&item.SizeBytes, &item.ModifiedUnix, &item.Available,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return StreamItem{}, sql.ErrNoRows
	}
	return item, err
}
