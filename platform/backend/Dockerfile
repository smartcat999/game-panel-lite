FROM golang:1.25-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/control-plane ./cmd/control-plane \
    && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/region-controller ./cmd/region-controller \
    && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/node-agent ./cmd/node-agent

FROM alpine:3.22

RUN addgroup -S -g 10001 gamepanel && adduser -S -D -H -u 10001 -G gamepanel gamepanel
COPY --from=build /out/control-plane /usr/local/bin/control-plane
COPY --from=build /out/region-controller /usr/local/bin/region-controller
COPY --from=build /out/node-agent /usr/local/bin/node-agent
USER gamepanel
ENTRYPOINT ["/usr/local/bin/control-plane"]
