package database

import (
	"database/sql"
	"fmt"
)

// migratePhase8 introduces the anime_series library kind used for dubbed anime
// and other episodic anime collections. It belongs to both the Séries and
// Animes system categories, mirroring how mixed belongs to Filmes + Animes.
func migratePhase8(db *sql.DB) error {
	const schema = `
INSERT OR IGNORE INTO library_category_libraries(category_id,library_id)
SELECT c.id,l.id FROM libraries l JOIN library_categories c ON c.slug='series'
WHERE l.kind='anime_series';

INSERT OR IGNORE INTO library_category_libraries(category_id,library_id)
SELECT c.id,l.id FROM libraries l JOIN library_categories c ON c.slug='anime'
WHERE l.kind='anime_series';

DROP TRIGGER IF EXISTS trg_library_default_categories_insert;
DROP TRIGGER IF EXISTS trg_library_default_categories_update;

CREATE TRIGGER trg_library_default_categories_insert
AFTER INSERT ON libraries
BEGIN
    INSERT OR IGNORE INTO library_category_libraries(category_id,library_id)
      SELECT id,NEW.id FROM library_categories WHERE system=1 AND slug='movie' AND NEW.kind IN ('movies','mixed');
    INSERT OR IGNORE INTO library_category_libraries(category_id,library_id)
      SELECT id,NEW.id FROM library_categories WHERE system=1 AND slug='series' AND NEW.kind IN ('series','anime_series');
    INSERT OR IGNORE INTO library_category_libraries(category_id,library_id)
      SELECT id,NEW.id FROM library_categories WHERE system=1 AND slug='anime' AND NEW.kind IN ('anime','mixed','anime_series');
END;

CREATE TRIGGER trg_library_default_categories_update
AFTER UPDATE OF kind ON libraries
BEGIN
    DELETE FROM library_category_libraries
      WHERE library_id=NEW.id AND category_id IN (SELECT id FROM library_categories WHERE system=1);
    INSERT OR IGNORE INTO library_category_libraries(category_id,library_id)
      SELECT id,NEW.id FROM library_categories WHERE system=1 AND slug='movie' AND NEW.kind IN ('movies','mixed');
    INSERT OR IGNORE INTO library_category_libraries(category_id,library_id)
      SELECT id,NEW.id FROM library_categories WHERE system=1 AND slug='series' AND NEW.kind IN ('series','anime_series');
    INSERT OR IGNORE INTO library_category_libraries(category_id,library_id)
      SELECT id,NEW.id FROM library_categories WHERE system=1 AND slug='anime' AND NEW.kind IN ('anime','mixed','anime_series');
END;
`
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("migrate phase8 anime series: %w", err)
	}
	return nil
}
