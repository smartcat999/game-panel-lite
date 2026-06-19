FROM golang:1.25-bookworm AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY apps/api ./apps/api

RUN CGO_ENABLED=1 GOOS=linux go build -o /out/gamepanel-exporter ./apps/api/cmd/gamepanel-exporter

FROM debian:bookworm-slim AS runner

RUN apt-get update \
  && apt-get install -y --no-install-recommends ca-certificates \
  && rm -rf /var/lib/apt/lists/*

COPY --from=builder /out/gamepanel-exporter /usr/local/bin/gamepanel-exporter

ENV GAMEPANEL_HOST=0.0.0.0 \
  GAMEPANEL_PORT=4010 \
  GAMEPANEL_DATA_DIR=/app/data \
  GAMEPANEL_DB_PATH=/app/data/gamepanel.db

EXPOSE 4010

CMD ["gamepanel-exporter"]
