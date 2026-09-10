FROM golang:1.25-alpine AS build

ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY}
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/control-plane ./cmd/control-plane \
    && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/region-controller ./cmd/region-controller \
    && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/node-agent ./cmd/node-agent \
    && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/schema-migrate ./cmd/schema-migrate \
    && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/bootstrap-platform ./cmd/bootstrap-platform

FROM alpine:3.22

RUN addgroup -S -g 10001 gamepanel && adduser -S -D -H -u 10001 -G gamepanel gamepanel
COPY --from=build /out/control-plane /usr/local/bin/control-plane
COPY --from=build /out/region-controller /usr/local/bin/region-controller
COPY --from=build /out/node-agent /usr/local/bin/node-agent
COPY --from=build /out/schema-migrate /usr/local/bin/schema-migrate
COPY --from=build /out/bootstrap-platform /usr/local/bin/bootstrap-platform
USER gamepanel
ENTRYPOINT ["/usr/local/bin/control-plane"]
