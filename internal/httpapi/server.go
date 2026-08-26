package httpapi

import (
	"context"
	"database/sql"
	"io/fs"
	"net/http"
	"time"

	"github.com/danilostorm/stormflix/internal/admin"
	"github.com/danilostorm/stormflix/internal/assets"
	"github.com/danilostorm/stormflix/internal/auth"
	"github.com/danilostorm/stormflix/internal/config"
	"github.com/danilostorm/stormflix/internal/library"
	"github.com/danilostorm/stormflix/internal/media"
	"github.com/danilostorm/stormflix/internal/metadata"
	appsettings "github.com/danilostorm/stormflix/internal/settings"
	"github.com/danilostorm/stormflix/internal/subtitles"
	"github.com/danilostorm/stormflix/internal/webui"
)

const sessionCookie = "stormflix_session"
const version = "0.10.0-profiles-cast-scan"

type contextKey string

const userKey contextKey = "user"

type server struct {
	db         *sql.DB
	libraries  *library.Service
	media      *media.Service
	auth       *auth.Service
	admin      *admin.Service
	metadata   *metadata.Service
	subtitles  *subtitles.Service
	assets     *assets.Store
	settings   *appsettings.Service
	baseConfig config.Config
	config     config.Config
	startedAt  time.Time
}

func New(db *sql.DB, libraries *library.Service, cfg config.Config) http.Handler {
	settingsService, err := appsettings.New(db, cfg.DataDir)
	if err != nil {
		panic(err)
	}
	effective, err := settingsService.Apply(context.Background(), cfg)
	if err != nil {
		panic(err)
	}
	effective = config.NormalizeCredentials(effective)
	assetStore, err := assets.New(effective.AssetDir, effective.AssetPublicBaseURL)
	if err != nil {
		panic(err)
	}
	if err := admin.EnsureMonitoring(db); err != nil {
		panic(err)
	}
	s := &server{
		db: db, libraries: libraries, media: media.NewService(db), auth: auth.NewService(db), admin: admin.NewService(db),
		assets: assetStore, settings: settingsService, baseConfig: cfg, config: effective, startedAt: time.Now(),
	}
	s.metadata = metadata.NewService(db, effective, assetStore)
	s.metadata.RecoverInterruptedJobs()
	s.subtitles = subtitles.NewService(db, effective, assetStore)
	s.auth.Cleanup(context.Background())

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /api/v1/system/info", s.systemInfo)
	mux.HandleFunc("GET /api/v1/setup/status", s.setupStatus)
	mux.HandleFunc("POST /api/v1/setup/admin", s.createFirstAdmin)
	mux.HandleFunc("POST /api/v1/auth/login", s.login)
	mux.HandleFunc("POST /api/v1/auth/logout", s.requireAuth(s.logout))
	mux.HandleFunc("GET /api/v1/auth/me", s.requireAuth(s.me))

	mux.HandleFunc("GET /api/v1/profiles", s.requireAuth(s.listProfiles))
	mux.HandleFunc("GET /api/v1/profiles/continue", s.requireAuth(s.continueWatching))
	mux.HandleFunc("POST /api/v1/profiles", s.requireAuth(s.createOwnProfile))
	mux.HandleFunc("PUT /api/v1/profiles/{id}", s.requireAuth(s.updateOwnProfile))
	mux.HandleFunc("DELETE /api/v1/profiles/{id}", s.requireAuth(s.deleteOwnProfile))
	mux.HandleFunc("POST /api/v1/profiles/{id}/select", s.requireAuth(s.selectProfile))

	mux.HandleFunc("GET /api/v1/libraries", s.requireAuth(s.listLibraries))
	mux.HandleFunc("POST /api/v1/libraries", s.requireRole("manager", s.createLibrary))
	mux.HandleFunc("PUT /api/v1/libraries/{id}", s.requireRole("manager", s.updateLibrary))
	mux.HandleFunc("DELETE /api/v1/libraries/{id}", s.requireRole("manager", s.deleteLibrary))
	mux.HandleFunc("POST /api/v1/libraries/{id}/scan", s.requireRole("operator", s.scanLibrary))
	mux.HandleFunc("POST /api/v1/libraries/{id}/scan/cancel", s.requireRole("operator", s.cancelLibraryScan))

	mux.HandleFunc("GET /api/v1/categories", s.requireAuth(s.listCategories))
	mux.HandleFunc("GET /api/v1/categories/{slug}", s.requireAuth(s.browseCategory))
	mux.HandleFunc("GET /api/v1/home", s.requireAuth(s.homeFeed))
	mux.HandleFunc("GET /api/v1/people", s.requireAuth(s.personTitles))
	mux.HandleFunc("GET /api/v1/series", s.requireAuth(s.listSeries))
	mux.HandleFunc("GET /api/v1/series/{id}", s.requireAuth(s.seriesDetails))
	mux.HandleFunc("GET /api/v1/media", s.requireAuth(s.listMedia))
	mux.HandleFunc("GET /api/v1/media/{id}", s.requireAuth(s.mediaDetails))
	mux.HandleFunc("GET /api/v1/media/{id}/stream", s.requireAuth(s.streamMedia))
	mux.HandleFunc("GET /api/v1/media/{id}/compatibility", s.requireAuth(s.mediaCompatibility))
	mux.HandleFunc("GET /api/v1/media/{id}/remux", s.requireAuth(s.remuxMedia))
	mux.HandleFunc("GET /api/v1/media/{id}/versions", s.requireAuth(s.mediaVersions))
	mux.HandleFunc("GET /api/v1/media/{id}/subtitles", s.requireAuth(s.mediaSubtitles))
	mux.HandleFunc("GET /api/v1/media/{id}/subtitles/{subtitle_id}/vtt", s.requireAuth(s.subtitleVTT))
	mux.HandleFunc("POST /api/v1/media/{id}/playback", s.requireAuth(s.playbackHeartbeat))
	mux.HandleFunc("DELETE /api/v1/media/{id}/playback", s.requireAuth(s.playbackStop))

	mux.HandleFunc("GET /api/v1/admin/dashboard", s.requireRole("operator", s.adminDashboard))
	mux.HandleFunc("GET /api/v1/admin/users", s.requireRole("admin", s.listUsers))
	mux.HandleFunc("POST /api/v1/admin/users", s.requireRole("admin", s.createUser))
	mux.HandleFunc("PUT /api/v1/admin/users/{id}", s.requireRole("admin", s.updateUser))
	mux.HandleFunc("DELETE /api/v1/admin/users/{id}", s.requireRole("admin", s.deleteUser))
	mux.HandleFunc("GET /api/v1/admin/users/{id}/profiles", s.requireRole("admin", s.adminUserProfiles))
	mux.HandleFunc("POST /api/v1/admin/users/{id}/profiles", s.requireRole("admin", s.adminCreateProfile))
	mux.HandleFunc("PUT /api/v1/admin/profiles/{id}", s.requireRole("admin", s.adminUpdateProfile))
	mux.HandleFunc("DELETE /api/v1/admin/profiles/{id}", s.requireRole("admin", s.adminDeleteProfile))

	mux.HandleFunc("GET /api/v1/admin/categories", s.requireRole("manager", s.adminCategories))
	mux.HandleFunc("POST /api/v1/admin/categories", s.requireRole("manager", s.createCategory))
	mux.HandleFunc("PUT /api/v1/admin/categories/{id}", s.requireRole("manager", s.updateCategory))
	mux.HandleFunc("DELETE /api/v1/admin/categories/{id}", s.requireRole("manager", s.deleteCategory))
	mux.HandleFunc("POST /api/v1/admin/libraries/{id}/consolidate", s.requireRole("manager", s.consolidateLibraryCopies))

	mux.HandleFunc("GET /api/v1/admin/cleanup", s.requireRole("admin", s.cleanupStatus))
	mux.HandleFunc("POST /api/v1/admin/cleanup", s.requireRole("admin", s.runCleanup))
	mux.HandleFunc("GET /api/v1/admin/sessions", s.requireRole("admin", s.listSessions))
	mux.HandleFunc("DELETE /api/v1/admin/sessions/{id}", s.requireRole("admin", s.revokeSession))
	mux.HandleFunc("GET /api/v1/admin/logs", s.requireRole("operator", s.logs))
	mux.HandleFunc("GET /api/v1/admin/storage", s.requireRole("operator", s.storage))
	mux.HandleFunc("GET /api/v1/admin/playbacks", s.requireRole("operator", s.playbacks))
	mux.HandleFunc("GET /api/v1/admin/monitoring", s.requireRole("operator", s.monitoringOverview))
	mux.HandleFunc("GET /api/v1/admin/server", s.requireRole("operator", s.serverInfo))
	mux.HandleFunc("GET /api/v1/admin/filesystem", s.requireRole("manager", s.browseFilesystem))
	mux.HandleFunc("GET /api/v1/admin/agents", s.requireRole("operator", s.agentStatus))
	mux.HandleFunc("POST /api/v1/admin/agents/tmdb/test", s.requireRole("operator", s.testTMDBAgent))
	mux.HandleFunc("GET /api/v1/admin/settings", s.requireRole("admin", s.getSettings))
	mux.HandleFunc("PUT /api/v1/admin/settings", s.requireRole("admin", s.updateSettings))
	mux.HandleFunc("GET /api/v1/admin/metadata/status", s.requireRole("operator", s.metadataStatus))
	mux.HandleFunc("GET /api/v1/admin/metadata/errors", s.requireRole("operator", s.metadataErrors))
	mux.HandleFunc("GET /api/v1/admin/metadata/jobs", s.requireRole("operator", s.metadataJobs))
	mux.HandleFunc("POST /api/v1/admin/libraries/{id}/metadata", s.requireRole("operator", s.startMetadataJob))
	mux.HandleFunc("POST /api/v1/admin/libraries/{id}/metadata/errors/retry", s.requireRole("operator", s.retryMetadataErrors))
	mux.HandleFunc("POST /api/v1/admin/media/{id}/metadata", s.requireRole("operator", s.refreshMediaMetadata))
	mux.HandleFunc("GET /api/v1/admin/media/{id}/artwork", s.requireRole("operator", s.mediaArtwork))
	mux.HandleFunc("POST /api/v1/admin/media/{id}/artwork/{artwork_id}/select", s.requireRole("manager", s.selectMediaArtwork))
	mux.HandleFunc("GET /api/v1/admin/subtitles/jobs", s.requireRole("operator", s.subtitleJobs))
	mux.HandleFunc("POST /api/v1/admin/libraries/{id}/subtitles", s.requireRole("operator", s.startSubtitleJob))

	mux.HandleFunc("GET /assets/", s.requireAuth(s.serveAsset))

	staticFS, err := fs.Sub(webui.Static, "static")
	if err != nil {
		panic(err)
	}
	mux.Handle("/", http.FileServer(http.FS(staticFS)))
	return requestLogger(recoverer(securityHeaders(mux)))
}
