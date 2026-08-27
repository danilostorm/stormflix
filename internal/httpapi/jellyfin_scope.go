package httpapi

import (
	"context"
	"net/http"
)

// jellyfinNormalizeLibraryScope keeps the library scope used by Jellyfin list
// endpoints and item/detail endpoints consistent. Privileged StormFlix users
// can browse every enabled library, so passing a stale explicit LibraryIDs
// slice into media.Detail/SeriesDetail would make an item visible in a list but
// return 404 when Android TV opened it. nil is the media service's unrestricted
// scope. Normal users retain their assignments, with the existing legacy
// full-catalog fallback when no explicit assignments exist.
func (s *server) jellyfinNormalizeLibraryScope(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := currentUser(r)
		if roleLevel(u.Role) >= 2 {
			u.LibraryIDs = nil
		} else if len(u.LibraryIDs) == 0 {
			u.LibraryIDs = s.allEnabledLibraryIDs(r.Context())
		}
		next(w, r.WithContext(context.WithValue(r.Context(), userKey, u)))
	}
}
