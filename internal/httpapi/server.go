package httpapi

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/danilostorm/stormflix/internal/admin"
	"github.com/danilostorm/stormflix/internal/assets"
	"github.com/danilostorm/stormflix/internal/auth"
	"github.com/danilostorm/stormflix/internal/config"
	"github.com/danilostorm/stormflix/internal/games"
	"github.com/danilostorm/stormflix/internal/library"
	"github.com/danilostorm/stormflix/internal/media"
	"github.com/danilostorm/stormflix/internal/metadata"
	"github.com/danilostorm/stormflix/internal/music"
	appsettings "github.com/danilostorm/stormflix/internal/settings"
	"github.com/danilostorm/stormflix/internal/streaming"
	"github.com/danilostorm/stormflix/internal/subtitles"
	"github.com/danilostorm/stormflix/internal/transcode"
	"github.com/danilostorm/stormflix/internal/webcompat"
	"github.com/danilostorm/stormflix/internal/webui"
)

const sessionCookie = "stormflix_session"
const version = "0.28.0-playback-engine-v7"

type contextKey string

const userKey contextKey = "user"

type server struct {
	db          *sql.DB
	libraries   *library.Service
	media       *media.Service
	music       *music.Service
	games       *games.Service
	auth        *auth.Service
	admin       *admin.Service
	metadata    *metadata.Service
	subtitles   *subtitles.Service
	assets      *assets.Store
	settings    *appsettings.Service
	compatCache *webcompat.CacheManager
	hlsCache    *webcompat.HLSManager
	baseConfig  config.Config
	config      config.Config
	startedAt   time.Time
	lifecycle   context.Context
	homeMetrics homeTelemetry
}

func New(db *sql.DB, libraries *library.Service, cfg config.Config) http.Handler {
	return NewWithContext(context.Background(), db, libraries, cfg)
}

func NewWithContext(lifecycle context.Context, db *sql.DB, libraries *library.Service, cfg config.Config) http.Handler {
	if lifecycle == nil {
		lifecycle = context.Background()
	}
	settingsService, err := appsettings.New(db, cfg.DataDir)
	if err != nil {
		panic(err)
	}
	effective, err := settingsService.Apply(lifecycle, cfg)
	if err != nil {
		panic(err)
	}
	effective = config.NormalizeCredentials(effective)
	transcode.ConfigureProcessScheduler(effective.MaxFFmpegProcesses, effective.MaxVideoTranscodes)
	transcode.ConfigureCPUThreadLimit(effective.TranscodeCPUThreads)
	streamPolicy := streaming.DefaultPolicy()
	streamPolicy.MaxBytes = effective.WebStreamCacheMaxBytes
	streamPolicy.MinFreeBytes = effective.CompatCacheMinFreeBytes
	streamPolicy.MinFreePercent = effective.CompatCacheMinFreePercent
	streamPolicy.IdleTTL = effective.HLSCacheIdleTTL
	streamPolicy.WorkerIdleTTL = effective.WebStreamWorkerIdleTTL
	streamPolicy.MaxAheadSegments = int(effective.WebStreamMaxAhead / (2 * time.Second))
	streamPolicy.KeepBehindSegments = int(effective.WebStreamKeepBehind / (2 * time.Second))
	if _, err := streaming.ForDataDirWithPolicy(effective.DataDir, streamPolicy); err != nil {
		panic(err)
	}
	assetStore, err := assets.New(effective.AssetDir, effective.AssetPublicBaseURL)
	if err != nil {
		panic(err)
	}
	compatCache, err := newCompatCache(effective)
	if err != nil {
		panic(err)
	}
	hlsCache, err := newHLSCache(effective)
	if err != nil {
		panic(err)
	}
	if err := admin.EnsureMonitoring(db); err != nil {
		panic(err)
	}
	music.ConfigureProviders(effective.LastFMAPIKey)
	s := &server{db: db, libraries: libraries, media: media.NewServiceWithContext(lifecycle, db), music: music.NewService(db), games: games.NewService(db), auth: auth.NewService(db), admin: admin.NewService(db), assets: assetStore, settings: settingsService, compatCache: compatCache, hlsCache: hlsCache, baseConfig: cfg, config: effective, startedAt: time.Now(), lifecycle: lifecycle}
	s.metadata = metadata.NewService(db, effective, assetStore)
	s.metadata.ResumeQueuedJobs()
	s.games.ResumeMetadataJobs()
	s.subtitles = subtitles.NewService(db, effective, assetStore)
	s.auth.Cleanup(lifecycle)
	s.compatCache.Start(lifecycle)
	s.hlsCache.Start(lifecycle)
	s.startTechnicalIndexer(lifecycle)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /api/v1/system/info", s.systemInfo)
	mux.HandleFunc("GET /api/v1/setup/status", s.setupStatus)
	mux.HandleFunc("POST /api/v1/setup/admin", s.createFirstAdmin)
	mux.HandleFunc("POST /api/v1/auth/login", s.login)
	mux.HandleFunc("POST /api/v1/auth/logout", s.requireAuth(s.logout))
	mux.HandleFunc("GET /api/v1/auth/me", s.requireAuth(s.me))

	mux.HandleFunc("GET /api/v1/profiles", s.requireAuth(s.listProfiles))
	mux.HandleFunc("GET /api/v1/profiles/home-menus", s.requireAuth(s.selectedProfileHomeMenus))
	mux.HandleFunc("GET /api/v1/profiles/continue", s.requireAuth(s.continueWatching))
	mux.HandleFunc("GET /api/v1/profiles/history", s.requireAuth(s.profileHistory))
	mux.HandleFunc("GET /api/v1/profiles/stats", s.requireAuth(s.profileStats))
	mux.HandleFunc("GET /api/v1/community/ranking", s.requireAuth(s.communityRanking))
	mux.HandleFunc("POST /api/v1/profiles", s.requireAuth(s.createOwnProfile))
	mux.HandleFunc("PUT /api/v1/profiles/{id}", s.requireAuth(s.updateOwnProfile))
	mux.HandleFunc("DELETE /api/v1/profiles/{id}", s.requireAuth(s.deleteOwnProfile))
	mux.HandleFunc("POST /api/v1/profiles/{id}/select", s.requireAuth(s.selectProfile))
	mux.HandleFunc("POST /api/v1/profiles/{id}/avatar", s.requireAuth(s.uploadOwnProfileAvatar))
	mux.HandleFunc("GET /api/v1/profiles/{id}/trakt", s.requireAuth(s.profileTraktStatus))
	mux.HandleFunc("POST /api/v1/profiles/{id}/trakt/device", s.requireAuth(s.profileTraktDevice))
	mux.HandleFunc("POST /api/v1/profiles/{id}/trakt/device/poll", s.requireAuth(s.profileTraktPoll))
	mux.HandleFunc("DELETE /api/v1/profiles/{id}/trakt", s.requireAuth(s.profileTraktDisconnect))

	mux.HandleFunc("GET /api/v1/libraries", s.requireAuth(s.listLibraries))
	mux.HandleFunc("POST /api/v1/libraries", s.requireRole("manager", s.createLibrary))
	mux.HandleFunc("PUT /api/v1/libraries/{id}", s.requireRole("manager", s.updateLibraryWithBackup))
	mux.HandleFunc("DELETE /api/v1/libraries/{id}", s.requireRole("manager", s.deleteLibrary))
	mux.HandleFunc("POST /api/v1/libraries/{id}/scan", s.requireRole("operator", s.scanLibraryDispatchWithBackup))
	mux.HandleFunc("POST /api/v1/libraries/{id}/scan/cancel", s.requireRole("operator", s.cancelLibraryScanDispatch))

	mux.HandleFunc("GET /api/v1/categories", s.requireAuth(s.listCategories))
	mux.HandleFunc("GET /api/v1/categories/{slug}", s.requireAuth(s.browseCategory))
	mux.HandleFunc("GET /api/v1/categories/{slug}/smart", s.requireAuth(s.browseSmartCategory))
	mux.HandleFunc("GET /api/v1/home", s.requireAuth(s.homeFeed))
	mux.HandleFunc("POST /api/v1/telemetry/home", s.requireAuth(s.homeClientTelemetry))
	mux.HandleFunc("GET /api/v1/people", s.requireAuth(s.personTitles))
	mux.HandleFunc("GET /api/v1/series", s.requireAuth(s.listSeries))
	mux.HandleFunc("GET /api/v1/series/{id}", s.requireAuth(s.seriesDetails))
	mux.HandleFunc("GET /api/v1/media", s.requireAuth(s.listMedia))
	mux.HandleFunc("GET /api/v1/media/{id}", s.requireAuth(s.mediaDetails))
	mux.HandleFunc("GET /api/v1/media/{id}/neighbors", s.requireAuth(s.mediaEpisodeNeighbors))
	mux.HandleFunc("GET /api/v1/media/{id}/stream", s.requireAuth(s.streamMedia))
	mux.HandleFunc("GET /api/v1/media/{id}/compatibility", s.requireAuth(s.mediaCompatibility))
	mux.HandleFunc("POST /api/v1/media/{id}/remux/prepare", s.requireAuth(s.prepareRemuxMedia))
	mux.HandleFunc("GET /api/v1/media/{id}/remux", s.requireAuth(s.remuxMedia))
	mux.HandleFunc(hlsRoutePattern, s.requireAuth(s.hlsDispatch))
	mux.HandleFunc("GET /api/v1/media/{id}/versions", s.requireAuth(s.mediaVersions))
	mux.HandleFunc("GET /api/v1/media/{id}/subtitles", s.requireAuth(s.mediaSubtitles))
	mux.HandleFunc("GET /api/v1/media/{id}/subtitles/{subtitle_id}/vtt", s.requireAuth(s.subtitleVTT))
	mux.HandleFunc("GET /api/v1/media/{id}/playback/streams", s.requireAuth(s.playbackStreams))
	mux.HandleFunc("POST /api/v1/media/{id}/playback/plan", s.requireAuth(s.playbackPlan))
	mux.HandleFunc("POST /api/v1/media/{id}/playback/grant", s.requireAuth(s.createPlaybackGrant))
	mux.HandleFunc("GET /api/v1/media/{id}/webstream/{session}/index.m3u8", s.requireAuth(s.webStreamPlaylist))
	mux.HandleFunc("GET /api/v1/media/{id}/webstream/{session}/{file}", s.requireAuth(s.webStreamFragment))
	mux.HandleFunc("POST /api/v1/media/{id}/playback", s.requireAuth(s.playbackHeartbeat))
	mux.HandleFunc("POST /api/v1/media/{id}/playback/event", s.requireAuth(s.playbackEvent))
	mux.HandleFunc("POST /api/v1/media/{id}/playback/telemetry", s.requireAuth(s.playbackTelemetry))
	mux.HandleFunc("DELETE /api/v1/media/{id}/playback", s.requireAuth(s.playbackStop))

	mux.HandleFunc("GET /api/v1/music/home", s.requireAuth(s.musicHome))
	mux.HandleFunc("GET /api/v1/music/tracks", s.requireAuth(s.musicTracks))
	mux.HandleFunc("GET /api/v1/music/tracks/{id}", s.requireAuth(s.musicTrack))
	mux.HandleFunc("GET /api/v1/music/tracks/{id}/stream", s.requireAuth(s.musicStream))
	mux.HandleFunc("POST /api/v1/music/tracks/{id}/listening", s.requireAuth(s.musicListening))
	mux.HandleFunc("POST /api/v1/music/tracks/{id}/favorite", s.requireAuth(s.musicFavorite))
	mux.HandleFunc("GET /api/v1/music/tracks/{id}/lyrics", s.requireAuth(s.musicLyrics))

	mux.HandleFunc("GET /api/v1/games/runtime/nostalgist.js", s.requireAuth(s.gameRuntimeNostalgist))
	mux.HandleFunc("GET /api/v1/games/runtime/cores/{asset}", s.requireAuth(s.gameRuntimeCore))
	mux.HandleFunc("GET /api/v1/games/home", s.requireAuth(s.gamesHome))
	mux.HandleFunc("GET /api/v1/games", s.requireAuth(s.gamesList))
	mux.HandleFunc("GET /api/v1/games/saves", s.requireAuth(s.gameSavesGallery))
	mux.HandleFunc("GET /api/v1/games/{id}", s.requireAuth(s.gameDetail))
	mux.HandleFunc("GET /api/v1/games/{id}/cover", s.requireAuth(s.gameCover))
	mux.HandleFunc("GET /api/v1/games/{id}/rom", s.requireAuth(s.gameROM))
	mux.HandleFunc("GET /api/v1/games/{id}/saves", s.requireAuth(s.gameSaveStatus))
	mux.HandleFunc("GET /api/v1/games/{id}/saves/{kind}", s.requireAuth(s.gameSaveRead))
	mux.HandleFunc("PUT /api/v1/games/{id}/saves/{kind}", s.requireAuth(s.gameSaveWrite))
	mux.HandleFunc("POST /api/v1/games/{id}/playback", s.requireAuth(s.gamePlaybackHeartbeat))
	mux.HandleFunc("POST /api/v1/games/{id}/favorite", s.requireAuth(s.gameFavorite))

	// The compatibility gateway is available both at /jellyfin-api and at the
	// root paths expected when an official client is given https://host directly.
	s.registerJellyfinRoutes(mux, "/jellyfin-api")
	s.registerJellyfinRoutes(mux, "")

	mux.HandleFunc("GET /api/v1/admin/dashboard", s.requireRole("operator", s.adminDashboard))
	mux.HandleFunc("GET /api/v1/admin/games/overview", s.requireRole("operator", s.adminGamesOverview))
	mux.HandleFunc("GET /api/v1/admin/games/catalog", s.requireRole("operator", s.adminGamesCatalog))
	mux.HandleFunc("GET /api/v1/admin/games/providers", s.requireRole("operator", s.adminGamesProviders))
	mux.HandleFunc("PUT /api/v1/admin/games/providers/{provider}", s.requireRole("admin", s.adminUpdateGameProvider))
	mux.HandleFunc("GET /api/v1/admin/games/metadata/jobs", s.requireRole("operator", s.adminGamesMetadataJobs))
	mux.HandleFunc("POST /api/v1/admin/games/metadata", s.requireRole("operator", s.adminStartAllGamesMetadata))
	mux.HandleFunc("POST /api/v1/admin/games/libraries/{id}/metadata", s.requireRole("operator", s.adminStartGamesMetadata))
	mux.HandleFunc("PUT /api/v1/admin/games/catalog/{id}/metadata-lock", s.requireRole("manager", s.adminGameMetadataLock))
	mux.HandleFunc("GET /api/v1/admin/users", s.requireRole("admin", s.listUsers))
	mux.HandleFunc("POST /api/v1/admin/users", s.requireRole("admin", s.createUser))
	mux.HandleFunc("PUT /api/v1/admin/users/{id}", s.requireRole("admin", s.updateUser))
	mux.HandleFunc("DELETE /api/v1/admin/users/{id}", s.requireRole("admin", s.deleteUser))
	mux.HandleFunc("GET /api/v1/admin/users/{id}/profiles", s.requireRole("admin", s.adminUserProfiles))
	mux.HandleFunc("POST /api/v1/admin/users/{id}/profiles", s.requireRole("admin", s.adminCreateProfile))
	mux.HandleFunc("PUT /api/v1/admin/profiles/{id}", s.requireRole("admin", s.adminUpdateProfile))
	mux.HandleFunc("DELETE /api/v1/admin/profiles/{id}", s.requireRole("admin", s.adminDeleteProfile))
	mux.HandleFunc("GET /api/v1/admin/profile-home", s.requireRole("admin", s.adminProfileHomeOverview))
	mux.HandleFunc("PUT /api/v1/admin/profiles/{id}/home-menus", s.requireRole("admin", s.updateProfileHomeMenus))
	mux.HandleFunc("GET /api/v1/admin/catalog", s.requireRole("operator", s.adminCatalog))
	mux.HandleFunc("GET /api/v1/admin/catalog/works", s.requireRole("operator", s.adminCatalogWorks))
	mux.HandleFunc("GET /api/v1/admin/catalog/health", s.requireRole("operator", s.catalogHealth))
	mux.HandleFunc("GET /api/v1/admin/catalog/health/items", s.requireRole("operator", s.catalogHealthItems))
	mux.HandleFunc("GET /api/v1/admin/catalog/duplicates", s.requireRole("operator", s.catalogDuplicates))
	mux.HandleFunc("GET /api/v1/admin/catalog/technical/status", s.requireRole("operator", s.technicalCatalogStatus))
	mux.HandleFunc("POST /api/v1/admin/catalog/technical/scan", s.requireRole("operator", s.restartTechnicalCatalog))
	mux.HandleFunc("GET /api/v1/admin/catalog/history", s.requireRole("operator", s.catalogHistory))
	mux.HandleFunc("GET /api/v1/admin/catalog/{id}/matches", s.requireRole("manager", s.adminCatalogMatches))
	mux.HandleFunc("POST /api/v1/admin/catalog/{id}/match", s.requireRole("manager", s.adminCatalogMatch))
	mux.HandleFunc("POST /api/v1/admin/catalog/{id}/auto", s.requireRole("manager", s.adminCatalogAuto))
	mux.HandleFunc("GET /api/v1/admin/categories", s.requireRole("manager", s.adminCategories))
	mux.HandleFunc("GET /api/v1/admin/category-rules", s.requireRole("manager", s.adminCategoryRules))
	mux.HandleFunc("POST /api/v1/admin/categories", s.requireRole("manager", s.createCategory))
	mux.HandleFunc("POST /api/v1/admin/categories/organize", s.requireRole("manager", s.organizeRecommendedCategoriesWithBackup))
	mux.HandleFunc("POST /api/v1/admin/categories/preview", s.requireRole("operator", s.previewSmartCategory))
	mux.HandleFunc("PUT /api/v1/admin/categories/order", s.requireRole("manager", s.reorderCategories))
	mux.HandleFunc("PUT /api/v1/admin/categories/{id}", s.requireRole("manager", s.updateCategory))
	mux.HandleFunc("PUT /api/v1/admin/categories/{id}/rules", s.requireRole("manager", s.updateCategoryRules))
	mux.HandleFunc("DELETE /api/v1/admin/categories/{id}", s.requireRole("manager", s.deleteCategory))
	mux.HandleFunc("GET /api/v1/admin/jobs", s.requireRole("operator", s.adminJobs))
	mux.HandleFunc("POST /api/v1/admin/libraries/scan-all", s.requireRole("operator", s.scanAllLibrariesWithBackup))
	mux.HandleFunc("POST /api/v1/admin/libraries/{id}/scan-preview", s.requireRole("operator", s.previewLibraryScan))
	mux.HandleFunc("POST /api/v1/admin/libraries/{id}/consolidate", s.requireRole("manager", s.consolidateLibraryCopies))
	mux.HandleFunc("POST /api/v1/admin/music/index", s.requireRole("operator", s.adminMusicIndex))
	mux.HandleFunc("GET /api/v1/admin/cleanup", s.requireRole("admin", s.cleanupStatus))
	mux.HandleFunc("POST /api/v1/admin/cleanup", s.requireRole("admin", s.runCleanup))
	mux.HandleFunc("GET /api/v1/admin/sessions", s.requireRole("admin", s.listSessions))
	mux.HandleFunc("DELETE /api/v1/admin/sessions/{id}", s.requireRole("admin", s.revokeSession))
	mux.HandleFunc("GET /api/v1/admin/logs", s.requireRole("operator", s.logs))
	mux.HandleFunc("GET /api/v1/admin/storage", s.requireRole("operator", s.storage))
	mux.HandleFunc("GET /api/v1/admin/playbacks", s.requireRole("operator", s.playbacks))
	mux.HandleFunc("GET /api/v1/admin/playbacks/diagnostics", s.requireRole("operator", s.playbackDiagnostics))
	mux.HandleFunc("GET /api/v1/admin/playback/cache", s.requireRole("admin", s.compatCacheStatus))
	mux.HandleFunc("POST /api/v1/admin/playback/cache/cleanup", s.requireRole("admin", s.cleanupCompatCache))
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
	mux.HandleFunc("GET /api/v1/admin/backups", s.requireRole("admin", s.listBackups))
	mux.HandleFunc("POST /api/v1/admin/backups", s.requireRole("admin", s.manualBackup))
	mux.HandleFunc("POST /api/v1/admin/backups/{id}/restore", s.requireRole("admin", s.scheduleBackupRestore))

	mux.HandleFunc("GET /assets/", s.requireAuth(s.serveAsset))
	mux.Handle("/", webui.Handler())
	return requestLogger(recoverer(securityHeaders(responseCompression(s.jellyfinTrace(mux)))))
}
