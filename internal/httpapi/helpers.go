package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/danilostorm/stormflix/internal/auth"
)

func (s *server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var u auth.User
		var sessionErr error
		if c, err := r.Cookie(sessionCookie); err == nil {
			u, sessionErr = s.auth.CurrentUser(r.Context(), c.Value)
		} else {
			sessionErr = err
		}

		if sessionErr != nil {
			grantUser, profileID, grantErr := s.playbackGrantUser(r)
			if grantErr != nil {
				writeError(w, 401, errors.New("authentication required"))
				return
			}
			u = grantUser
			if profileID > 0 {
				r.AddCookie(&http.Cookie{Name: profileCookie, Value: strconv.FormatInt(profileID, 10), Path: "/"})
			}
			// A temporary playback grant is intentionally usable by a Chromecast,
			// Smart TV or external player that does not share browser cookies.
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Expose-Headers", "Accept-Ranges, Content-Length, Content-Range, Content-Type")
		}

		// The current StormFlix model is full-catalog by default. Older users may
		// have no user_libraries rows because library selection used to be manual;
		// treat that legacy empty set as access to every enabled library.
		if roleLevel(u.Role) < 2 && len(u.LibraryIDs) == 0 {
			u.LibraryIDs = s.allEnabledLibraryIDs(r.Context())
		}
		next(w, r.WithContext(context.WithValue(r.Context(), userKey, u)))
	}
}
func (s *server) requireRole(min string, next http.HandlerFunc) http.HandlerFunc {
	return s.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		if roleLevel(currentUser(r).Role) < roleLevel(min) {
			writeError(w, 403, errors.New("insufficient permissions"))
			return
		}
		next(w, r)
	})
}
func roleLevel(role string) int {
	switch role {
	case "admin":
		return 4
	case "manager":
		return 3
	case "operator":
		return 2
	default:
		return 1
	}
}
func currentUser(r *http.Request) auth.User {
	u, _ := r.Context().Value(userKey).(auth.User)
	return u
}
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeError(w, 400, errors.New("invalid JSON body"))
		return err
	}
	return nil
}
func clientIP(r *http.Request) string {
	if x := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0]); x != "" {
		return x
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}
func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}
func shortDevice(ua string) string {
	ua = strings.TrimSpace(ua)
	if len(ua) > 120 {
		return ua[:120]
	}
	return ua
}
func mediaContentType(ext string) string {
	switch strings.ToLower(ext) {
	case ".mkv":
		return "video/x-matroska"
	case ".mp4", ".m4v":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	case ".ts", ".m2ts":
		return "video/mp2t"
	case ".avi":
		return "video/x-msvideo"
	case ".mov":
		return "video/quicktime"
	case ".mpg", ".mpeg":
		return "video/mpeg"
	}
	if v := mime.TypeByExtension(ext); v != "" {
		return v
	}
	return "application/octet-stream"
}
func parseID(v string) (int64, error) {
	id, err := strconv.ParseInt(v, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid id")
	}
	return id, nil
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		next.ServeHTTP(w, r)
	})
}
func recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				slog.Error("request panic", "panic", v)
				writeError(w, 500, errors.New("internal server error"))
			}
		}()
		next.ServeHTTP(w, r)
	})
}
func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		next.ServeHTTP(w, r)
		slog.Info("request", "method", r.Method, "path", r.URL.Path, "duration", time.Since(started))
	})
}
