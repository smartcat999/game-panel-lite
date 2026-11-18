FROM golang:1.26-alpine AS build

ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY}
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY pkg/ ./pkg/
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/controller-manager ./cmd/controller-manager

FROM alpine:3.22

RUN addgroup -S -g 10001 gamepanel && adduser -S -D -H -u 10001 -G gamepanel gamepanel
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /out/controller-manager /usr/local/bin/controller-manager
USER gamepanel
ENTRYPOINT ["/usr/local/bin/controller-manager"]
