FROM golang:1.26-alpine AS build

ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY}
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY pkg/ ./pkg/
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/apiserver ./cmd/apiserver

FROM alpine:3.22

RUN addgroup -S -g 10001 gamepanel && adduser -S -D -H -u 10001 -G gamepanel gamepanel
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /out/apiserver /usr/local/bin/apiserver
USER gamepanel
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/apiserver"]
