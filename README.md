# Sentinel

[![Build & Test](https://github.com/JeremiahM37/sentinel/actions/workflows/test.yml/badge.svg)](https://github.com/JeremiahM37/sentinel/actions/workflows/test.yml)
[![Release](https://img.shields.io/github/v/release/JeremiahM37/sentinel?include_prereleases)](https://github.com/JeremiahM37/sentinel/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

Media download guardian and library verification for the *arr ecosystem.

Sentinel ensures media downloads actually land in your libraries by:

1. Accepting download requests (movie, TV, audiobook, ebook, comic)
2. Trying multiple sources in priority order (Jellyseerr, Prowlarr)
3. Monitoring download progress via qBittorrent
4. Verifying content **definitively** appears in the library (file paths, durations, page counts)
5. Retrying with alternative sources if verification fails
6. Persisting jobs in SQLite (survives restarts)

## Quick Start

```bash
# Build
go build -o sentinel ./cmd/sentinel/

# Configure
cp .env.example .env
# Edit .env with your service URLs and API keys

# Run
./sentinel
```

## Docker

```bash
docker compose up -d
```

## API

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Health check |
| POST | `/api/jobs` | Create a guardian job |
| GET | `/api/jobs` | List jobs (filter: `?status=`, `?media_type=`) |
| GET | `/api/jobs/{id}` | Job detail |
| POST | `/api/jobs/{id}/cancel` | Cancel an active job |
| POST | `/api/jobs/{id}/retry` | Retry a failed/cancelled job |
| DELETE | `/api/jobs/{id}` | Delete a job |
| GET | `/api/stats` | Job statistics by status |
| POST | `/api/verify` | Manual library verification |

## Library Verification

Verification is **definitive**, not fuzzy matching:

- **Jellyfin**: Items API -> check Path exists, MediaSources, RunTimeTicks > 0
- **Audiobookshelf**: search -> check isMissing=false, numAudioFiles > 0, duration > 0
- **Kavita**: login -> search -> series detail -> check pages > 0
- **Sonarr**: series list -> check episodeFileCount > 0
- **Radarr**: movie list -> check hasFile=true, movieFile exists

## Architecture

Single static binary, zero CGO dependencies, pure-Go SQLite via `modernc.org/sqlite`.

```
cmd/sentinel/main.go          Entry point
internal/
  config/config.go             Env var configuration
  db/db.go                     SQLite persistence
  models/models.go             Core types
  api/                         HTTP handlers + router
  guardian/guardian.go          Background job runner
  sources/                     Download source plugins
  libraries/                   Library verification checkers
  notify/                      Notification channels
  titleutil/titleutil.go       Title matching (Jaccard + stopwords)
```

## License

MIT

## Disclaimer

This software is provided for **educational and personal use only**. Users are responsible for ensuring their use complies with all applicable laws and regulations in their jurisdiction. The developers do not condone or encourage copyright infringement or any illegal activity. This tool does not host, store, or distribute any copyrighted content.
