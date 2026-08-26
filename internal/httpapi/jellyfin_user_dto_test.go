package httpapi

import (
	"testing"

	"github.com/danilostorm/stormflix/internal/auth"
	"github.com/danilostorm/stormflix/internal/config"
)

func TestJellyfinStrictUserObjectIncludesRequiredAndroidTVFields(t *testing.T) {
	s := &server{config: config.Config{DataDir: t.TempDir(), ServerName: "StormFlix Test"}}
	user := s.jellyfinStrictUserObject(auth.User{
		ID:          1,
		Username:    "tester",
		DisplayName: "Tester",
		Role:        "user",
		Active:      true,
	})

	for _, key := range []string{"Id", "Name", "HasPassword", "HasConfiguredPassword", "HasConfiguredEasyPassword"} {
		if _, ok := user[key]; !ok {
			t.Fatalf("required Jellyfin UserDto field %q is missing: %#v", key, user)
		}
	}
	if value, ok := user["HasConfiguredEasyPassword"].(bool); !ok || value {
		t.Fatalf("HasConfiguredEasyPassword=%#v, want false", user["HasConfiguredEasyPassword"])
	}
	if user["Configuration"] != nil {
		t.Fatalf("Configuration must stay nil for cross-SDK compatibility: %#v", user["Configuration"])
	}
	if user["Policy"] != nil {
		t.Fatalf("Policy must stay nil for cross-SDK compatibility: %#v", user["Policy"])
	}
}
