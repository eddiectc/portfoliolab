---
task: 01
feature: Project Skeleton + Portfolio CRUD
depends-on: []
status: pending
---
# Task 01: Go Module + Project Skeleton

## Goal
Initialize the Go module, create the project directory structure, config loading, and a minimal HTTP server that starts and responds to a health check endpoint.

## Files to Create/Modify
| File | Action | Description |
|------|--------|-------------|
| `go.mod` | create | Module definition: `github.com/arch-portfolio-lab/portfoliolab` |
| `cmd/server/main.go` | create | Entry point: parse flags, load config, start HTTP server |
| `internal/config/config.go` | create | Config struct + YAML loading |
| `config/config.example.yaml` | create | Example config with server port, db path, log level |
| `internal/api/router.go` | create | chi router setup with health check route |
| `internal/api/middleware/logging.go` | create | Request logging middleware using `slog` |
| `Makefile` | create | Build, run, test, lint, migrate targets |
| `.gitignore` | create | Standard Go ignores (`/bin/`, `*.db`, etc.) |

## Implementation Steps
1. Run `go mod init github.com/arch-portfolio-lab/portfoliolab`
2. Create directory structure per README project layout
3. Implement config loading from YAML file (flag `--config` to override path)
4. Create chi router with `/health` endpoint returning `{"status": "ok"}`
5. Add request logging middleware
6. Wire up `main.go`: load config → create router → start server
7. Write Makefile targets: `make run`, `make build`, `make test`, `make lint`
8. Write unit tests for config loading (valid/invalid YAML, missing file)

## Acceptance Criteria
- [ ] `go build ./...` compiles with no errors
- [ ] Server starts on configured port (default 8080)
- [ ] `GET /health` returns `200 OK` with `{"status": "ok"}`
- [ ] Config loads from `config/config.yaml` or `--config` flag
- [ ] Request logging middleware logs method, path, status, duration
- [ ] Unit tests pass for config loading

## Testing Requirements
- Unit tests: config loading (valid YAML, invalid YAML, missing file, default values)
- Manual verification: `make run` → curl `http://localhost:8080/health`

## Notes
- Keep dependencies minimal: `chi`, `gopkg.in/yaml.v3`, `slog` (std)
- Config struct should include: server port, database path, log level
