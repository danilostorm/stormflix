package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/danilostorm/stormflix/internal/media"
)

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
	config, err := s.categoryRule(r.Context(), c.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
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

	series, err := s.media.SeriesList(r.Context(), ids, "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	items, err := s.media.CatalogList(r.Context(), 0, "", 500, 0, ids)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	clientCaps := clientMediaCapsFromRequest(r)
	gateUHDByDevice := shouldGateUHDCategoryByDevice(config.RuleMode, config.Rules)
	technicalPending := false
	allowedByDevice := func(mediaID int64) bool {
		if !gateUHDByDevice || !clientCaps.Explicit {
			return true
		}
		tech, ok := s.technicalSnapshotFor(r.Context(), mediaID)
		if !ok || tech.Status != "ok" {
			return true
		}
		return clientAllows4KMedia(clientCaps, tech)
	}
	for _, item := range series {
		if s.smartItemMatches(r.Context(), item.RepresentativeMediaID, item.MediaType, item.Year, item.Rating, item.Genres, item.ModifiedUnix, "matched", config.Rules, config.RuleMode, &technicalPending) && allowedByDevice(item.RepresentativeMediaID) {
			response.Series = append(response.Series, item)
		}
	}
	for _, item := range items {
		if item.EntityType == "series" || item.EpisodeNumber > 0 || item.MediaType == "series" {
			continue
		}
		if s.smartItemMatches(r.Context(), item.ID, item.MediaType, item.Year, item.Rating, item.Genres, item.ModifiedUnix, item.MetadataStatus, config.Rules, config.RuleMode, &technicalPending) && allowedByDevice(item.ID) {
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
	if mode == "libraries" || (mode == "both" && len(own) > 0) {
		return own, nil
	}
	if c.ParentID != nil {
		return s.categoryTreeLibraries(ctx, *c.ParentID)
	}
	return s.categoryTreeLibraries(ctx, c.ID)
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
		if modifiedUnix < time.Now().Unix()-int64(rules.RecentDays)*86400 {
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
	needsTechnical := rules.MinHeight > 0 || rules.MaxHeight > 0 || rules.HDR != "" || rules.DubStatus != "" || rules.AudioPTBR || rules.SubtitlePTBR
	if !needsTechnical {
		return true
	}
	tech, ok := s.technicalSnapshotFor(ctx, mediaID)
	if !ok || tech.Status != "ok" {
		if technicalPending != nil {
			*technicalPending = true
		}
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
		if strings.TrimSpace(tech.HDR) == "" {
			return false
		}
	case "sdr":
		if strings.TrimSpace(tech.HDR) != "" {
			return false
		}
	case "hdr10", "hlg":
		if strings.ToLower(tech.HDR) != rules.HDR {
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
