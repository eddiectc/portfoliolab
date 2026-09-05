# Portfoliolab — single static Go binary (pure Go, no CGO, no libc).
# Builds with both `docker build` and `podman build`.

FROM golang:1.26 AS build
WORKDIR /src
# Download dependencies first to leverage layer caching
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/portfoliolab ./cmd/server
# Empty dir copied into the final image so container runtimes (docker,
# rootless podman) seed the data volume with the correct ownership.
RUN mkdir /out/data

# Distroless static: no shell, no package manager, minimal attack surface.
# The non-root variant runs as user "nonroot" (uid 65532).
FROM gcr.io/distroless/static:nonroot
USER nonroot:nonroot
WORKDIR /app
COPY --from=build --chown=nonroot:nonroot /out/portfoliolab /app/portfoliolab
COPY --from=build --chown=nonroot:nonroot /out/data /app/data

# SQLite database + WAL files live here. Mount a volume to persist data
# across container updates (see compose.yaml).
VOLUME ["/app/data"]
EXPOSE 8080

# Configuration: mount a config file at /app/config/config.yaml and pass
# --config, or use PORTFOLIOLAB_* environment variables.
# Defaults: 0.0.0.0:8080, data/portfoliolab.db (relative to /app), log level info.
ENTRYPOINT ["/app/portfoliolab"]
