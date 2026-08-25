# StormFlix

StormFlix é um servidor de mídia leve, focado em **Direct Play Only**.

A ideia é oferecer uma experiência semelhante a Plex/Emby/Jellyfin sem carregar um pipeline de transcodificação no servidor. O arquivo é lido do armazenamento montado (por exemplo Google Drive via rclone) e entregue diretamente ao cliente com suporte a HTTP Range.

## Princípios

- zero transcoding no servidor;
- zero GPU obrigatória;
- armazenamento externo/montado permanece como fonte dos arquivos;
- servidor pequeno em Go;
- SQLite para catálogo/configuração;
- web, desktop, mobile e TV usando a mesma API;
- bibliotecas montadas preferencialmente como somente leitura.

## Estado atual

Primeiro MVP do backend:

- [x] servidor HTTP em Go;
- [x] SQLite em WAL;
- [x] cadastro de bibliotecas por caminho;
- [x] scanner recursivo de vídeos;
- [x] catálogo básico;
- [x] busca básica;
- [x] streaming direto com HTTP Range/206;
- [x] frontend web mínimo embutido no binário;
- [x] Docker/Compose;
- [ ] autenticação e usuários;
- [ ] progresso/continuar assistindo;
- [ ] identificação de filmes/séries;
- [ ] metadados/posters;
- [ ] faixas de áudio/legendas;
- [ ] app desktop;
- [ ] Android mobile;
- [ ] Android TV.

## Rodar localmente

Requer Go 1.23+.

```bash
go run ./cmd/stormflix
```

Abra:

```text
http://localhost:8090
```

Por padrão os dados ficam em `./data/stormflix.db`.

### Variáveis de ambiente

```text
STORMFLIX_ADDR=:8090
STORMFLIX_DATA_DIR=./data
STORMFLIX_BOOTSTRAP_LIBRARY_NAME=Filmes
STORMFLIX_BOOTSTRAP_LIBRARY_PATH=/mnt/gdrive/Filmes
```

As duas variáveis `BOOTSTRAP` são opcionais. Quando definidas e ainda não existe nenhuma biblioteca, o StormFlix cadastra a primeira automaticamente.

## Docker Compose

Edite `docker-compose.yml` e troque:

```yaml
- /mnt/gdrive:/media:ro
```

pelo caminho do seu mount real. Depois:

```bash
docker compose up -d --build
```

Abra `http://IP-DO-SERVIDOR:8090`.

Dentro do container, uma biblioteca deverá usar o caminho `/media/...`, não o caminho original do host.

## API inicial

```text
GET  /healthz
GET  /api/v1/system/info
GET  /api/v1/libraries
POST /api/v1/libraries
POST /api/v1/libraries/{id}/scan
GET  /api/v1/media
GET  /api/v1/media/{id}/stream
```

Exemplo de biblioteca:

```json
{
  "name": "Filmes 4K",
  "kind": "movies",
  "path": "/media/Filmes 4K"
}
```

## Direct Play

O endpoint de stream usa `http.ServeContent`, que suporta requisições HTTP `Range`. Assim o cliente pode buscar partes do arquivo e fazer seek sem o StormFlix converter o vídeo.

O servidor adiciona:

```text
X-StormFlix-Playback: direct
Accept-Ranges: bytes
```

Compatibilidade de codecs/containers é responsabilidade do cliente. Se um navegador não reproduzir um MKV/HEVC/DTS específico, a solução planejada é usar o app StormFlix com player nativo, **não transcodificar no servidor**.

## Segurança atual

Este é o primeiro MVP e ainda não possui autenticação. Não exponha a porta diretamente à internet antes da etapa de usuários/sessões. Para testes, mantenha o serviço em rede privada ou protegido pelo seu reverse proxy.

## Licença

Licença ainda não definida. O código inicial do StormFlix foi criado do zero para evitar herdar a licença/código de servidores de mídia existentes.
