# Deployment

Arch Portfolio Lab can be run as a single binary or in a container (Docker or Podman).
Both are supported with identical behavior — the container runs the same binary.

## Contents
- [Binary](#binary)
- [Container (Docker or Podman)](#container-docker-or-podman)
- [Configuration via environment variables](#configuration-via-environment-variables)
- [Updating](#updating)
- [Troubleshooting](#troubleshooting)

## Binary

```bash
go build -o portfoliolab ./cmd/server
./portfoliolab --config config/config.yaml   # or: ./portfoliolab  (uses defaults + env vars)
```

Requires a config file (or env vars) and a writable `data/` directory for the SQLite
database. Full reference: [config.example.yaml](../config/config.example.yaml).

## Container (Docker or Podman)

A multi-arch OCI image (`linux/amd64`, `linux/arm64`) is published to GitHub Container
Registry on every push to `main` and on each release tag:

```
ghcr.io/eddiectc/portfoliolab:latest   # newest main build
ghcr.io/eddiectc/portfoliolab:<tag>    # pinned release, e.g. :0.1.0
ghcr.io/eddiectc/portfoliolab:<sha>    # immutable commit build
```

No `ghcr.io` login is required to **pull** (the repo is public).

### Quick start

The commands below are identical for Docker and Podman (both use Compose v2).
`docker` and `podman` are used interchangeably in this document — pick whichever is
installed on your host.

```bash
# 1. Pull the image
docker pull ghcr.io/eddiectc/portfoliolab:latest

# 2. Run with a named volume for the database
docker run -d --name portfoliolab \
  -p 127.0.0.1:8080:8080 \
  -v portfoliolab-data:/app/data \
  ghcr.io/eddiectc/portfoliolab:latest

# 3. Check it's healthy
curl http://127.0.0.1:8080/health
# → {"status":"ok"}
```

Podman notes (rootless):
- Podman 4+ includes `podman compose` (or install `podman-compose` for older versions).
- Named volumes in rootless podman are stored per-user; no `sudo` needed.
- Podman pulls the exact same OCI image Docker does — nothing engine-specific.

### Using the compose file

The repo ships a [compose.yaml](../compose.yaml) pinned to the published
image:

```bash
docker compose up -d        # or: podman compose up -d
docker compose logs -f
docker compose down
```

Compose runs the app with its data in a `portfoliolab-data` volume and port 8080
bound to all interfaces by default. For local-only access, change the mapping to
`127.0.0.1:8080:8080`, or put a reverse proxy in front of the app.

### Data persistence

The SQLite database and WAL files live on the `/app/data` volume (the image's default
DB path is `data/portfoliolab.db` relative to the working directory `/app`). The
`/app/data` directory is pre-created in the image and owned by the non-root user, so
a fresh volume gets correct ownership on both engines.

Back up by copying the volume contents out with a helper image (the app image is
distroless and has no shell or `cp`):

```bash
docker run --rm \
  -v portfoliolab-data:/data:ro \
  -v "$PWD":/backup \
  alpine:3 cp -r /data /backup/portfoliolab-backup
```

### Access from another machine

Bind to all interfaces (`-p 8080:8080`) **and** put the app behind a reverse proxy
with authentication (e.g. Caddy with basic auth) — the app itself is single-user and
has no auth.

## Configuration via environment variables

Every `config.yaml` field can be set with an environment variable instead (or to
override the file). Useful inside containers:

| Environment variable | Config field | Default |
|---|---|---|
| `PORTFOLIOLAB_HOST` | `server.host` | `0.0.0.0` |
| `PORTFOLIOLAB_PORT` | `server.port` | `8080` |
| `PORTFOLIOLAB_DB_PATH` | `database.path` | `data/portfoliolab.db` |
| `PORTFOLIOLAB_LOG_LEVEL` | `log.level` | `info` |

Precedence: **environment variable > config file > built-in default.**
Example: run on a different port without a config file:

```bash
docker run -d -p 8081:8081 -e PORTFOLIOLAB_PORT=8081 \
  -v portfoliolab-data:/app/data ghcr.io/eddiectc/portfoliolab:latest
```

## Updating

```bash
docker pull ghcr.io/eddiectc/portfoliolab:latest
docker stop portfoliolab && docker rm portfoliolab
# re-run the same docker run command from Quick start (volume is reused — data intact)
```

The database schema migrates automatically on startup (goose). Back up the
`portfoliolab-data` volume before updating, as with any schema migration.

## Troubleshooting

| Symptom | Fix |
|---|---|
| `dial tcp ... connection refused` after `up` | App may still be starting; check `docker compose logs`. Port already in use? Change the host-side port in the mapping. |
| `permission denied` writing the database (podman rootless) | Use a named volume (recommended). With a bind mount, `chown` the host directory to uid 65532 — the container runs as the `nonroot` user. |
| Health check fails but logs show a startup error | Read the logs — config or migration errors print before the listener starts. |
| Old image still running after `pull` | Containers keep the old image until removed and re-created; `docker compose pull && docker compose up -d` handles this. |
