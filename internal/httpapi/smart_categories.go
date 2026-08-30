package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"unicode"

	"github.com/danilostorm/stormflix/internal/media"
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
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "rules", "both":
		return strings.ToLower(strings.TrimSpace(value))
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
	out := []string{}
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
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			// Common Portuguese accents are intentionally folded so TMDB English,
			// Portuguese labels and Admin input can be compared consistently.
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
		default:
			if b.Len() > 0 && !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func (s *server) categoryRule(ctx context.Context, categoryID int64) (categoryRuleConfig, error) {
	var config categoryRuleConfig
	var raw string
	config.CategoryID = categoryID
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

func (s *server) browseSmartCategory(w http.ResponseWriter, r *http.Request) {
	slug := strings.ToLower(strings.TrimSpace(r.PathValue("slug")))
	var c libraryCategory
	var parent sql.NullInt64
	err := s.db.QueryRowContext(r.Context(), `SELECT id,name,slug,kind,parent_id,sort_order,active,system FROM library_categories WHERE slug=? AND active=1`, slug).
		Scan(&c.ID, &c.Name, &c.Slug, &c.Kind, &parent, &c.SortOrder, &c.Active, &c.System)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, errors.New("category not found"))
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if parent.Valid {
		v := parent.Int64
		c.ParentID = &v
	}
	config, _ := s.categoryRule(r.Context(), c.ID)
	ids, err := s.smartCategoryLibraries(r.Context(), c, config.RuleMode)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	u := currentUser(r)
	if roleLevel(u.Role) < 2 {
		ids = intersectIDs(ids, u.LibraryIDs)
	}
	response := struct {
		Category libraryCategory       `json:"category"`
		Media    []media.Item          `json:"media"`
		Series   []media.SeriesSummary `json:"series"`
		RuleMode string                `json:"rule_mode"`
		Rules    categoryRules         `json:"rules"`
	}{Category: c, Media: []media.Item{}, Series: []media.SeriesSummary{}, RuleMode: config.RuleMode, Rules: config.Rules}
	response.Category.LibraryIDs = ids
	if len(ids) == 0 {
		writeJSON(w, http.StatusOK, response)
		return
	}
	series, _ := s.media.SeriesList(r.Context(), ids, "")
	items, _ := s.media.List(r.Context(), 0, "", 500, 0, ids)
	technicalPending := false
	for _, seriesItem := range series {
		if s.smartSeriesMatches(r.Context(), seriesItem, config.Rules, config.RuleMode, &technicalPending) {
			response.Series = append(response.Series, seriesItem)
		}
	}
	for _, item := range items {
		if item.EpisodeNumber > 0 || item.MediaType == "series" {
			continue
		}
		if s.smartItemMatches(r.Context(), item.ID, item.MediaType, item.Year, item.Rating, item.Genres, item.ModifiedUnix, item.MetadataStatus, config.Rules, config.RuleMode, &technicalPending) {
			response.Media = append(response.Media, item)
		}
	}
	if technicalPending {
		s.kickTechnicalIndexer()
	}
	if s.selectedProfileRestriction(r, u.ID).Restricted {
		response.Media = s.filterRestrictedItems(r, u.ID, response.Media)
		response.Series = s.filterRestrictedSeries(r, u.ID, response.Series)
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *server) smartCategoryLibraries(ctx context.Context, c libraryCategory, mode string) ([]int64, error) {
	own, err := s.categoryLibraries(ctx, c.ID)
	if err != nil {
		return nil, err
	}
	if mode == "libraries" || mode == "both" && len(own) > 0 {
		return own, nil
	}
	if c.ParentID != nil {
		return s.categoryTreeLibraries(ctx, *c.ParentID)
	}
	return s.categoryTreeLibraries(ctx, c.ID)
}

func (s *server) smartSeriesMatches(ctx context.Context, item media.SeriesSummary, rules categoryRules, mode string, technicalPending *bool) bool {
	return s.smartItemMatches(ctx, item.RepresentativeMediaID, item.MediaType, item.Year, item.Rating, item.Genres, item.ModifiedUnix, "matched", rules, mode, technicalPending)
}

func (s *server) smartItemMatches(ctx context.Context, mediaID int64, mediaType string, year int, rating float64, genres []string, modifiedUnix int64, metadataStatus string, rules categoryRules, mode string, technicalPending *bool) bool {
	if mode == "libraries" {
		return true
	}
	if len(rules.MediaTypes) > 0 && !ruleContains(rules.MediaTypes, categoryRuleKey(mediaType)) {
		return false
	}
	if rules.YearFrom > 0 && year < rules.YearFrom {
		return false
	}
	if rules.YearTo > 0 && year > rules.YearTo {
		return false
	}
	if rules.MinRating > 0 && rating < rules.MinRating {
		return false
	}
	if rules.RequireMetadata && (metadataStatus == "" || metadataStatus == "pending" || metadataStatus == "error") {
		return false
	}
	if rules.RecentDays > 0 && modifiedUnix > 0 {
		cutoff := timeNowUnix() - int64(rules.RecentDays*86400)
		if modifiedUnix < cutoff {
			return false
		}
	}
	if len(rules.Genres) > 0 {
		matched := false
		for _, genre := range genres {
			if ruleContains(rules.Genres, categoryRuleKey(genre)) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	technicalRequired := rules.MinHeight > 0 || rules.MaxHeight > 0 || rules.HDR != "" || rules.DubStatus != "" || rules.AudioPTBR || rules.SubtitlePTBR
	if !technicalRequired {
		return true
	}
	tech, ok := s.technicalSnapshotFor(ctx, mediaID)
	if !ok || tech.Status != "ok" {
		*technicalPending = true
		return false
	}
	if rules.MinHeight > 0 && tech.Height < rules.MinHeight {
		return false
	}
	if rules.MaxHeight > 0 && tech.Height > rules.MaxHeight {
		return false
	}
	if rules.AudioPTBR && !tech.AudioPTBR {
		return false
	}
	if rules.SubtitlePTBR && !tech.SubtitlePTBR {
		return false
	}
	if rules.DubStatus != "" && rules.DubStatus != "qualquer" && tech.DubStatus != rules.DubStatus {
		return false
	}
	switch rules.HDR {
	case "hdr":
		if tech.HDR == "" {
			return false
		}
	case "sdr":
		if tech.HDR != "" {
			return false
		}
	case "hdr10", "hlg":
		if tech.HDR != rules.HDR {
			return false
		}
	}
	return true
}

func ruleContains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

var nowUnix = func() int64 { return time.Now().Unix() }
func timeNowUnix() int64 { return nowUnix() }

func (s *server) previewSmartCategory(w http.ResponseWriter, r *http.Request) {
	var in struct {
		CategoryID int64         `json:"category_id"`
		ParentID   int64         `json:"parent_id"`
		RuleMode   string        `json:"rule_mode"`
		LibraryIDs []int64       `json:"library_ids"`
		Rules      categoryRules `json:"rules"`
	}
	if decodeJSON(w, r, &in) != nil {
		return
	}
	in.RuleMode = normalizeRuleMode(in.RuleMode)
	in.Rules = normalizeRules(in.Rules)
	ids := append([]int64(nil), in.LibraryIDs...)
	if (in.RuleMode == "rules" || len(ids) == 0) && in.ParentID > 0 {
		ids, _ = s.categoryTreeLibraries(r.Context(), in.ParentID)
	}
	items, _ := s.media.List(r.Context(), 0, "", 500, 0, ids)
	series, _ := s.media.SeriesList(r.Context(), ids, "")
	count := 0
	pending := false
	samples := []map[string]any{}
	seen := map[string]bool{}
	add := func(key, title, poster string, id int64) {
		if seen[key] {
			return
		}
		seen[key] = true
		count++
		if len(samples) < 10 {
			samples = append(samples, map[string]any{"id": id, "title": title, "poster_url": poster})
		}
	}
	for _, v := range series {
		if s.smartSeriesMatches(r.Context(), v, in.Rules, in.RuleMode, &pending) {
			add("s:"+v.ID, v.Title, v.PosterURL, v.RepresentativeMediaID)
		}
	}
	for _, v := range items {
		if v.EpisodeNumber > 0 || v.MediaType == "series" {
			continue
		}
		if s.smartItemMatches(r.Context(), v.ID, v.MediaType, v.Year, v.Rating, v.Genres, v.ModifiedUnix, v.MetadataStatus, in.Rules, in.RuleMode, &pending) {
			add("m:"+idString(v.ID), v.Title, v.PosterURL, v.ID)
		}
	}
	if pending {
		s.kickTechnicalIndexer()
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": count, "samples": samples, "technical_pending": pending})
}

func (s *server) reorderCategories(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ParentID *int64  `json:"parent_id"`
		IDs      []int64 `json:"ids"`
	}
	if decodeJSON(w, r, &in) != nil {
		return
	}
	if len(in.IDs) == 0 {
		writeError(w, http.StatusBadRequest, errors.New("category order is empty"))
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer tx.Rollback()
	for i, id := range in.IDs {
		var res sql.Result
		if in.ParentID == nil || *in.ParentID == 0 {
			res, err = tx.ExecContext(r.Context(), `UPDATE library_categories SET sort_order=?,updated_at=CURRENT_TIMESTAMP WHERE id=? AND parent_id IS NULL`, (i+1)*10, id)
		} else {
			res, err = tx.ExecContext(r.Context(), `UPDATE library_categories SET sort_order=?,updated_at=CURRENT_TIMESTAMP WHERE id=? AND parent_id=?`, (i+1)*10, id, *in.ParentID)
		}
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if n, _ := res.RowsAffected(); n == 0 {
			writeError(w, http.StatusBadRequest, errors.New("invalid category in order"))
			return
		}
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	uid := currentUser(r).ID
	s.recordCatalogChange(r.Context(), "category", "order", "reorder", "Ordem de menus/seções alterada", "", "", &uid)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func sortSmartItems(items []media.Item) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].ModifiedUnix == items[j].ModifiedUnix {
			return strings.ToLower(items[i].Title) < strings.ToLower(items[j].Title)
		}
		return items[i].ModifiedUnix > items[j].ModifiedUnix
	})
}
