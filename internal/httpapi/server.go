package httpapi

import (
	"context"
	"database/sql"
	"io/fs"
	"net/http"
	"time"

	"github.com/danilostorm/stormflix/internal/admin"
	"github.com/danilostorm/stormflix/internal/auth"
	"github.com/danilostorm/stormflix/internal/config"
	"github.com/danilostorm/stormflix/internal/library"
	"github.com/danilostorm/stormflix/internal/media"
	"github.com/danilostorm/stormflix/internal/webui"
)

const sessionCookie = "stormflix_session"
const version = "0.2.1-admin"

type contextKey string

const userKey contextKey = "user"

type server struct {
	db        *sql.DB
	libraries *library.Service
	media     *media.Service
	auth      *auth.Service
	admin     *admin.Service
	config    config.Config
	startedAt time.Time
}

func New(db *sql.DB, libraries *library.Service, cfg config.Config) http.Handler {
	s := &server{db: db, libraries: libraries, media: media.NewService(db), auth: auth.NewService(db), admin: admin.NewService(db), config: cfg, startedAt: time.Now()}
	s.auth.Cleanup(context.Background())
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /api/v1/system/info", s.systemInfo)
	mux.HandleFunc("GET /api/v1/setup/status", s.setupStatus)
	mux.HandleFunc("POST /api/v1/setup/admin", s.createFirstAdmin)
	mux.HandleFunc("POST /api/v1/auth/login", s.login)
	mux.HandleFunc("POST /api/v1/auth/logout", s.requireAuth(s.logout))
	mux.HandleFunc("GET /api/v1/auth/me", s.requireAuth(s.me))
	mux.HandleFunc("GET /api/v1/libraries", s.requireAuth(s.listLibraries))
	mux.HandleFunc("POST /api/v1/libraries", s.requireRole("manager", s.createLibrary))
	mux.HandleFunc("PUT /api/v1/libraries/{id}", s.requireRole("manager", s.updateLibrary))
	mux.HandleFunc("DELETE /api/v1/libraries/{id}", s.requireRole("manager", s.deleteLibrary))
	mux.HandleFunc("POST /api/v1/libraries/{id}/scan", s.requireRole("operator", s.scanLibrary))
	mux.HandleFunc("GET /api/v1/media", s.requireAuth(s.listMedia))
	mux.HandleFunc("GET /api/v1/media/{id}/stream", s.requireAuth(s.streamMedia))
	mux.HandleFunc("GET /api/v1/admin/dashboard", s.requireRole("operator", s.adminDashboard))
	mux.HandleFunc("GET /api/v1/admin/users", s.requireRole("admin", s.listUsers))
	mux.HandleFunc("POST /api/v1/admin/users", s.requireRole("admin", s.createUser))
	mux.HandleFunc("PUT /api/v1/admin/users/{id}", s.requireRole("admin", s.updateUser))
	mux.HandleFunc("DELETE /api/v1/admin/users/{id}", s.requireRole("admin", s.deleteUser))
	mux.HandleFunc("GET /api/v1/admin/sessions", s.requireRole("admin", s.listSessions))
	mux.HandleFunc("DELETE /api/v1/admin/sessions/{id}", s.requireRole("admin", s.revokeSession))
	mux.HandleFunc("GET /api/v1/admin/logs", s.requireRole("operator", s.logs))
	mux.HandleFunc("GET /api/v1/admin/storage", s.requireRole("operator", s.storage))
	mux.HandleFunc("GET /api/v1/admin/playbacks", s.requireRole("operator", s.playbacks))
	mux.HandleFunc("GET /api/v1/admin/server", s.requireRole("operator", s.serverInfo))
	mux.HandleFunc("GET /api/v1/admin/filesystem", s.requireRole("manager", s.browseFilesystem))
	staticFS, err := fs.Sub(webui.Static, "static")
	if err != nil {
		panic(err)
	}
	mux.Handle("/", http.FileServer(http.FS(staticFS)))
	return requestLogger(recoverer(securityHeaders(mux)))
}
