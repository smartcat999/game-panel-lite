FROM golang:1.26-alpine AS build

ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY}
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY pkg/ ./pkg/
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/registry-worker ./cmd/registry-worker

FROM registry.cn-hangzhou.aliyuncs.com/gamepanel-lite/palworld-server:v2.7.1

USER 0:0
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates libcurl3t64-gnutls openjdk-25-jre-headless tini wget \
    && rm -rf /var/lib/apt/lists/* \
    && chmod 0755 /home/steam \
    && mv /home/steam/steamcmd /opt/steamcmd \
    && ln -s /opt/steamcmd /home/steam/steamcmd \
    && mkdir -p /opt/steam-home /var/lib/gamepanel/registry-worker \
    && chmod -R a+rX /opt/steamcmd /opt/steam-home \
    && chown -R 1000:1000 /var/lib/gamepanel/registry-worker

COPY --chmod=0755 build/runtime-images/steamcmd-wrapper.sh /usr/local/bin/steamcmd
COPY --from=build /out/registry-worker /usr/local/bin/registry-worker

USER 1000:1000
WORKDIR /var/lib/gamepanel/registry-worker
ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/registry-worker"]
