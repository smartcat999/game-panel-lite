FROM golang:1.25-bookworm AS builder

WORKDIR /src

COPY go.mod go.sum ./
COPY vendor ./vendor
COPY internal ./internal
COPY apps/api ./apps/api

ARG GAMEPANEL_ENTRY

RUN set -eu; \
  case "${GAMEPANEL_ENTRY}" in \
    global-control|global-receiver|order-maintainer|outbox-publisher|region-control|region-fetcher|region-migrate|region-node|region-node-access|region-operations|region-receiver|region-repair-manifests|region-scheduler|region-status-publisher|region-status-receiver|region-upload-worker|regional-migration-audit) ;; \
    *) echo "unsupported control-plane entry: ${GAMEPANEL_ENTRY}" >&2; exit 1 ;; \
  esac; \
  CGO_ENABLED=0 GOOS=linux go build -mod=vendor -ldflags "-s -w" \
    -o /out/gamepanel-service "./apps/api/cmd/${GAMEPANEL_ENTRY}"

FROM debian:bookworm-slim AS runner

ARG GAMEPANEL_ENTRY
LABEL org.opencontainers.image.title="GamePanel Lite ${GAMEPANEL_ENTRY}" \
      io.gamepanel.composition-root="${GAMEPANEL_ENTRY}"

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /out/gamepanel-service /usr/local/bin/gamepanel-service

USER 65532:65532
ENTRYPOINT ["/usr/local/bin/gamepanel-service"]
