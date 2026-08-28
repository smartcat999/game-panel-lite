FROM golang:1.25-bookworm AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download
COPY apps/agent ./apps/agent

RUN CGO_ENABLED=0 GOOS=linux go build \
  -ldflags "-s -w" \
  -o /out/gamepanel-agent ./apps/agent

FROM debian:bookworm-slim AS runner

RUN apt-get update \
  && apt-get install -y --no-install-recommends ca-certificates curl \
  && rm -rf /var/lib/apt/lists/*

COPY --from=builder /out/gamepanel-agent /usr/local/bin/gamepanel-agent

ENTRYPOINT ["/usr/local/bin/gamepanel-agent"]
