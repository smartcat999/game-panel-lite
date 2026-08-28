FROM golang:1.25-bookworm AS builder

WORKDIR /src
COPY go.mod go.sum ./
COPY vendor ./vendor
COPY apps/updater ./apps/updater
RUN CGO_ENABLED=0 GOOS=linux go build -mod=vendor -o /out/gamepanel-updater ./apps/updater/cmd/server

FROM docker:28-cli
RUN docker compose version
COPY --from=builder /out/gamepanel-updater /usr/local/bin/gamepanel-updater
ENV GAMEPANEL_UPDATER_HOST=0.0.0.0 \
    GAMEPANEL_UPDATER_PORT=4020 \
    GAMEPANEL_UPDATER_WORKSPACE=/workspace
EXPOSE 4020
CMD ["gamepanel-updater"]
