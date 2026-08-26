package database

import (
	"database/sql"
	"fmt"
)

func migratePhase3(db *sql.DB) error {
	const schema = `
CREATE TABLE IF NOT EXISTS library_categories (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE COLLATE NOCASE,
    kind TEXT NOT NULL DEFAULT 'mixed',
    sort_order INTEGER NOT NULL DEFAULT 0,
    active INTEGER NOT NULL DEFAULT 1,
    system INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS library_category_libraries (
    category_id INTEGER NOT NULL,
    library_id INTEGER NOT NULL,
    sort_order INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY(category_id, library_id),
    FOREIGN KEY(category_id) REFERENCES library_categories(id) ON DELETE CASCADE,
    FOREIGN KEY(library_id) REFERENCES libraries(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS profiles (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    avatar_key TEXT NOT NULL DEFAULT 'storm-red',
    avatar_url TEXT NOT NULL DEFAULT '',
    is_kids INTEGER NOT NULL DEFAULT 0,
    active INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS profile_progress (
    profile_id INTEGER NOT NULL,
    media_id INTEGER NOT NULL,
    position_seconds REAL NOT NULL DEFAULT 0,
    duration_seconds REAL NOT NULL DEFAULT 0,
    completed INTEGER NOT NULL DEFAULT 0,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY(profile_id, media_id),
    FOREIGN KEY(profile_id) REFERENCES profiles(id) ON DELETE CASCADE,
    FOREIGN KEY(media_id) REFERENCES media(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_category_libraries_library ON library_category_libraries(library_id, category_id);
CREATE INDEX IF NOT EXISTS idx_profiles_user ON profiles(user_id, active, id);
CREATE INDEX IF NOT EXISTS idx_profile_progress_updated ON profile_progress(profile_id, updated_at);

INSERT OR IGNORE INTO library_categories(name,slug,kind,sort_order,active,system) VALUES
('Filmes','movie','movie',10,1,1),
('Séries','series','series',20,1,1),
('Animes','anime','anime',30,1,1);

INSERT OR IGNORE INTO library_category_libraries(category_id,library_id)
SELECT c.id,l.id FROM libraries l JOIN library_categories c ON c.slug='movie' WHERE l.kind IN ('movies','mixed');
INSERT OR IGNORE INTO library_category_libraries(category_id,library_id)
SELECT c.id,l.id FROM libraries l JOIN library_categories c ON c.slug='series' WHERE l.kind='series';
INSERT OR IGNORE INTO library_category_libraries(category_id,library_id)
SELECT c.id,l.id FROM libraries l JOIN library_categories c ON c.slug='anime' WHERE l.kind IN ('anime','mixed');

INSERT INTO profiles(user_id,name,avatar_key)
SELECT u.id,u.display_name,'storm-red' FROM users u
WHERE NOT EXISTS(SELECT 1 FROM profiles p WHERE p.user_id=u.id);

CREATE TRIGGER IF NOT EXISTS trg_users_default_profile
AFTER INSERT ON users
BEGIN
    INSERT INTO profiles(user_id,name,avatar_key) VALUES(NEW.id,NEW.display_name,'storm-red');
END;

CREATE TRIGGER IF NOT EXISTS trg_library_default_categories_insert
AFTER INSERT ON libraries
BEGIN
    INSERT OR IGNORE INTO library_category_libraries(category_id,library_id)
      SELECT id,NEW.id FROM library_categories WHERE system=1 AND slug='movie' AND NEW.kind IN ('movies','mixed');
    INSERT OR IGNORE INTO library_category_libraries(category_id,library_id)
      SELECT id,NEW.id FROM library_categories WHERE system=1 AND slug='series' AND NEW.kind='series';
    INSERT OR IGNORE INTO library_category_libraries(category_id,library_id)
      SELECT id,NEW.id FROM library_categories WHERE system=1 AND slug='anime' AND NEW.kind IN ('anime','mixed');
END;

CREATE TRIGGER IF NOT EXISTS trg_library_default_categories_update
AFTER UPDATE OF kind ON libraries
BEGIN
    DELETE FROM library_category_libraries
      WHERE library_id=NEW.id AND category_id IN (SELECT id FROM library_categories WHERE system=1);
    INSERT OR IGNORE INTO library_category_libraries(category_id,library_id)
      SELECT id,NEW.id FROM library_categories WHERE system=1 AND slug='movie' AND NEW.kind IN ('movies','mixed');
    INSERT OR IGNORE INTO library_category_libraries(category_id,library_id)
      SELECT id,NEW.id FROM library_categories WHERE system=1 AND slug='series' AND NEW.kind='series';
    INSERT OR IGNORE INTO library_category_libraries(category_id,library_id)
      SELECT id,NEW.id FROM library_categories WHERE system=1 AND slug='anime' AND NEW.kind IN ('anime','mixed');
END;
`
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("migrate phase3 database: %w", err)
	}
	return nil
}
