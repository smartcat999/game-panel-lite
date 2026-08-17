FROM golang:1.25-bookworm AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY apps/api ./apps/api

ARG GAMEPANEL_VERSION=v0.2.2
ARG GAMEPANEL_COMMIT=unknown
ARG GAMEPANEL_BUILD_TIME=unknown
RUN CGO_ENABLED=1 GOOS=linux go build \
  -ldflags "-X github.com/smartcat999/game-panel-lite/apps/api/internal/buildinfo.Version=${GAMEPANEL_VERSION} -X github.com/smartcat999/game-panel-lite/apps/api/internal/buildinfo.Commit=${GAMEPANEL_COMMIT} -X github.com/smartcat999/game-panel-lite/apps/api/internal/buildinfo.BuildTime=${GAMEPANEL_BUILD_TIME}" \
  -o /out/gamepanel-api ./apps/api/cmd/server

FROM debian:bookworm-slim AS runner

RUN apt-get update \
  && apt-get install -y --no-install-recommends ca-certificates \
  && rm -rf /var/lib/apt/lists/*

COPY --from=builder /out/gamepanel-api /usr/local/bin/gamepanel-api

ENV GAMEPANEL_HOST=0.0.0.0 \
  GAMEPANEL_PORT=4000 \
  GAMEPANEL_DATA_DIR=/app/data \
  GAMEPANEL_DB_PATH=/app/data/gamepanel.db

EXPOSE 4000

CMD ["gamepanel-api"]
