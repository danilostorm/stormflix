# StormFlix — instructions for coding agents

Before modifying this repository, **read `PROJECT_STATE.md` first**. It is the authoritative handoff for architecture decisions, deployed behavior, current compatibility work and known pending items.

## Mandatory workflow

1. Read `PROJECT_STATE.md` before changing code.
2. Preserve the native StormFlix API under `/api/v1`; Jellyfin compatibility stays isolated in its compatibility facade.
3. Preserve **Direct Play first**. Never introduce video transcoding as a silent fallback. Audio-only AAC compatibility is allowed where explicitly designed.
4. Scanner-owned series identity (library root → show → season → episode) is authoritative. Metadata providers enrich that identity; they must not redefine folder structure blindly.
5. A manual match on an episodic title should be stored at **series level** whenever possible, not repeated per episode.
6. Do not weaken profile/library access controls when adding caches or compatibility aliases.
7. Run CI/tests/build before presenting an update as ready.
8. After every meaningful architecture, compatibility, schema, playback or deployment change:
   - update `PROJECT_STATE.md` with the new current state and pending work;
   - append a concise entry to `CHANGELOG.md`;
   - include the relevant commit/feature description, not secrets or credentials.

## Deployment reference

Typical Unraid checkout:

```bash
cd /mnt/user/appdata/stormflix
git pull origin main
docker compose down
docker compose up -d --build
curl -s http://127.0.0.1:8090/healthz
echo
```

Never place API keys, passwords, tokens, personal identifiers or private media paths beyond generic examples in project documentation.
