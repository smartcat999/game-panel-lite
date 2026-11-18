FROM golang:1.26-alpine AS build

ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY}
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY pkg/ ./pkg/
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/node-agent ./cmd/node-agent

FROM alpine:3.22

RUN apk add --no-cache xfsprogs xfsprogs-extra && test -x /usr/sbin/xfs_quota && mkdir -p /var/lib/gamepanel/instances
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /out/node-agent /usr/local/bin/node-agent
EXPOSE 8082
ENTRYPOINT ["/usr/local/bin/node-agent"]
