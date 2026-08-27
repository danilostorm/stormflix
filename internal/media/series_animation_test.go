package media

import "testing"

func TestDeriveSeriesIdentitySkipsCartoonTechnicalFolders(t *testing.T) {
	path, name := deriveSeriesIdentity("/media/akumanimes/Desenhos/Pica-Pau e seus Amigos/Remux/002PP-BD1080pRemux.mkv")
	if name != "Pica-Pau e seus Amigos" {
		t.Fatalf("name=%q want Pica-Pau e seus Amigos", name)
	}
	if path != "/media/akumanimes/Desenhos/Pica-Pau e seus Amigos" {
		t.Fatalf("path=%q", path)
	}
}

func TestEpisodeFromCompactCartoonName(t *testing.T) {
	season, episode := episodeFromPath("/media/Desenhos/Pica-Pau e seus Amigos/Remux/002PP-BD1080pRemux.mkv")
	if season != 1 || episode != 2 {
		t.Fatalf("season/episode=%d/%d want 1/2", season, episode)
	}
}
