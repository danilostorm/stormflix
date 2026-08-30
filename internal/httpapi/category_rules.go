package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"unicode"
)

type categoryRules struct {
	Genres          []string `json:"genres,omitempty"`
	MediaTypes      []string `json:"media_types,omitempty"`
	YearFrom        int      `json:"year_from,omitempty"`
	YearTo          int      `json:"year_to,omitempty"`
	MinRating       float64  `json:"min_rating,omitempty"`
	MinHeight       int      `json:"min_height,omitempty"`
	MaxHeight       int      `json:"max_height,omitempty"`
	HDR             string   `json:"hdr,omitempty"`
	DubStatus       string   `json:"dub_status,omitempty"`
	AudioPTBR       bool     `json:"audio_pt_br,omitempty"`
	SubtitlePTBR    bool     `json:"subtitle_pt_br,omitempty"`
	RecentDays      int      `json:"recent_days,omitempty"`
	RequireMetadata bool     `json:"require_metadata,omitempty"`
}

type categoryRuleConfig struct {
	CategoryID int64         `json:"category_id"`
	RuleMode   string        `json:"rule_mode"`
	Rules      categoryRules `json:"rules"`
}

func normalizeRuleMode(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "rules", "both":
		return value
	default:
		return "libraries"
	}
}

func normalizeRules(in categoryRules) categoryRules {
	if in.YearFrom < 0 {
		in.YearFrom = 0
	}
	if in.YearTo < 0 {
		in.YearTo = 0
	}
	if in.MinRating < 0 {
		in.MinRating = 0
	}
	if in.MinRating > 10 {
		in.MinRating = 10
	}
	if in.MinHeight < 0 {
		in.MinHeight = 0
	}
	if in.MaxHeight < 0 {
		in.MaxHeight = 0
	}
	if in.RecentDays < 0 {
		in.RecentDays = 0
	}
	in.HDR = strings.ToLower(strings.TrimSpace(in.HDR))
	in.DubStatus = strings.ToLower(strings.TrimSpace(in.DubStatus))
	in.Genres = normalizeRuleStrings(in.Genres)
	in.MediaTypes = normalizeRuleStrings(in.MediaTypes)
	return in
}

func normalizeRuleStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		key := categoryRuleKey(value)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, key)
	}
	return out
}

func categoryRuleKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			switch r {
			case 'á', 'à', 'â', 'ã', 'ä':
				r = 'a'
			case 'é', 'è', 'ê', 'ë':
				r = 'e'
			case 'í', 'ì', 'î', 'ï':
				r = 'i'
			case 'ó', 'ò', 'ô', 'õ', 'ö':
				r = 'o'
			case 'ú', 'ù', 'û', 'ü':
				r = 'u'
			case 'ç':
				r = 'c'
			}
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if b.Len() > 0 && !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func (s *server) categoryRule(ctx context.Context, categoryID int64) (categoryRuleConfig, error) {
	config := categoryRuleConfig{CategoryID: categoryID, RuleMode: "libraries"}
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT rule_mode,rules_json FROM library_categories WHERE id=?`, categoryID).Scan(&config.RuleMode, &raw)
	if err != nil {
		return config, err
	}
	config.RuleMode = normalizeRuleMode(config.RuleMode)
	_ = json.Unmarshal([]byte(raw), &config.Rules)
	config.Rules = normalizeRules(config.Rules)
	return config, nil
}

func (s *server) adminCategoryRules(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT id,rule_mode,rules_json FROM library_categories ORDER BY id`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer rows.Close()
	out := []categoryRuleConfig{}
	for rows.Next() {
		var config categoryRuleConfig
		var raw string
		if err := rows.Scan(&config.CategoryID, &config.RuleMode, &raw); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		config.RuleMode = normalizeRuleMode(config.RuleMode)
		_ = json.Unmarshal([]byte(raw), &config.Rules)
		config.Rules = normalizeRules(config.Rules)
		out = append(out, config)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) updateCategoryRules(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var in categoryRuleConfig
	if decodeJSON(w, r, &in) != nil {
		return
	}
	in.RuleMode = normalizeRuleMode(in.RuleMode)
	in.Rules = normalizeRules(in.Rules)
	raw, _ := json.Marshal(in.Rules)
	res, err := s.db.ExecContext(r.Context(), `UPDATE library_categories SET rule_mode=?,rules_json=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, in.RuleMode, string(raw), id)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, errors.New("category not found"))
		return
	}
	uid := currentUser(r).ID
	s.recordCatalogChange(r.Context(), "category", idString(id), "rules", "Regras da seção atualizadas", "", string(raw), &uid)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "category_id": id, "rule_mode": in.RuleMode, "rules": in.Rules})
}
