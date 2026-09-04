# StormFlix

StormFlix é um servidor de mídia em Go focado em **Direct Play**, catálogo próprio, perfis, metadados, clientes web/Android e uma camada de compatibilidade com clientes Jellyfin.

> Para continuar o desenvolvimento, leia primeiro **[`PROJECT_STATE.md`](PROJECT_STATE.md)** e **[`AGENTS.md`](AGENTS.md)**. O histórico de mudanças fica em **[`CHANGELOG.md`](CHANGELOG.md)**.

## Princípios

- Direct Play como caminho principal;
- nenhuma transcodificação silenciosa de vídeo;
- compatibilidade de áudio pode converter somente a faixa necessária para AAC mantendo o vídeo em stream-copy;
- armazenamento externo/montado continua sendo a fonte dos arquivos;
- backend pequeno em Go;
- SQLite em WAL para catálogo/configuração/progresso;
- múltiplas bibliotecas/origens;
- scanner episódico separado dos agentes externos de metadata;
- API nativa `/api/v1` preservada e compatibilidade Jellyfin isolada.

## Estado atual

O projeto já possui, entre outros:

- autenticação, usuários, perfis e permissões por biblioteca;
- catálogo, busca, categorias e **subcategorias**;
- scanner recursivo e proteção contra mounts temporariamente offline;
- filmes, séries, animes, anime com temporadas, desenhos/séries de animação e música;
- scanner persistente de identidade `série → temporada → episódio`;
- TMDB, TheTVDB v4 opcional, AniList, AniDB/MAL recovery, Fanart.tv e ponte HAMA/Anime-Lists;
- correspondência manual de metadata no nível da **série inteira**, estilo Plex;
- posters, backdrops, logos, elenco, classificação, trailers e metadata de episódios;
- Continue Watching por perfil e progresso ordenado por sessão;
- HTTP Range/206 e Direct Play;
- Playback Engine v7 com demux/decode do arquivo original no desktop cliente, automaticamente e sem FFmpeg no servidor quando elegível;
- fallback de compatibilidade que mantém vídeo original e converte apenas áudio incompatível para AAC;
- frontend web e painel administrativo;
- Android mobile/TV baseado em Media3;
- facade Jellyfin para clientes oficiais mobile/Android TV/desktop;
- logs administrativos, monitoramento e recebimento de crash report Jellyfin;
- Home com projeção lógica persistente, cache por revisão e métricas p50/p95/p99;
- SQLite WAL com migrações auditadas, pool limitado e diagnóstico no Admin;
- limites globais de FFmpeg/CPU/cache e overlay NVIDIA opcional;
- assets/API comprimidos, ETags e renderização progressiva para catálogos maiores.

Consulte `PROJECT_STATE.md` para detalhes, decisões e pendências atuais.

## Rodar localmente

Requer Go compatível com o `go.mod` atual.

```bash
go run ./cmd/stormflix
```

Abra:

```text
http://localhost:8090
```

Os dados ficam no diretório configurado por `STORMFLIX_DATA_DIR`.

## Docker Compose

Monte o armazenamento de mídia dentro do container, preferencialmente somente leitura. Exemplo:

```yaml
- /mnt/media:/media:ro
```

Depois:

```bash
docker compose up -d --build
```

Em um host com NVIDIA Container Toolkit, exponha a GPU usando o overlay incluído:

```bash
docker compose -f docker-compose.yml -f docker-compose.nvidia.yml up -d --build
docker exec stormflix ffmpeg -hide_banner -encoders 2>/dev/null | grep -E 'h264_nvenc|hevc_nvenc'
```

Uma instalação Unraid existente normalmente é atualizada com:

```bash
cd /mnt/user/appdata/stormflix
git pull --ff-only origin main
docker compose down
docker compose up -d --build
sleep 15
git rev-parse HEAD
docker compose ps
curl -sS http://127.0.0.1:8090/healthz
echo
```

O servidor Unraid principal é somente CPU e usa apenas `docker-compose.yml`. O overlay NVIDIA é opcional para outras instalações. A decisão de manter SQLite e os gatilhos objetivos para uma futura migração PostgreSQL estão em [`docs/PERFORMANCE_FOUNDATION_V3.md`](docs/PERFORMANCE_FOUNDATION_V3.md).

## Direct Play e compatibilidade

Streams nativos usam HTTP Range. O servidor não deve converter vídeo só porque um cliente possui uma limitação de áudio. No Android/Fire, o arquivo multi-áudio original é entregue primeiro para que o cliente escolha o idioma preferido. Se a faixa preferida não puder ser decodificada, o cliente solicita explicitamente o modo de compatibilidade: o vídeo continua stream-copy e somente o áudio é convertido para AAC-LC em um MP4 seekable/cacheado.

## Metadados episódicos

Em bibliotecas de séries/animes/desenhos, o scanner resolve a identidade lógica antes dos provedores externos:

```text
raiz da biblioteca
  → pasta da obra
    → temporada
      → episódio
```

Assim, pastas técnicas como `Remux`, `BluRay` ou `1080p` não viram séries. A correspondência manual pode ser salva na obra principal e aplicada aos episódios atuais e futuros.

## Jellyfin

StormFlix não é um fork do Jellyfin. Ele mantém sua API nativa e expõe uma **facade de compatibilidade** para permitir conexão de clientes Jellyfin oficiais. A implementação e o estado atual estão documentados em `PROJECT_STATE.md`.

## Segurança

Não publique uma instalação sem autenticação, reverse proxy/TLS e configuração adequada de usuários/perfis. Não grave segredos no repositório; chaves de provedores são configuradas pelo painel/ambiente e armazenadas conforme a camada de settings do StormFlix.

## Licença

Consulte o arquivo de licença do repositório, quando presente. O código StormFlix é mantido separadamente dos projetos usados apenas como referência de protocolo/comportamento.
