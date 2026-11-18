# GamePanel Lite — Unified x86_64 Base Runtime Image (gamepanel/runtime:latest)
# Includes:
# - Debian GNU/Linux x86_64 glibc + libstdc++6 + lib32gcc-s1 + curl + jq + tar + unzip + procps
# - Debian Official OpenJDK 25 JRE (/opt/java/openjdk)
# - SteamCMD & Steam SDK (sdk32/steamclient.so & sdk64/steamclient.so) + rcon-cli
# - Unified /usr/local/bin/gamepanel-entrypoint.sh executing game packages mounted in /data

FROM gamepanel/tmodloader:v2026.07.3.0 AS dotnet-runtime

FROM registry.cn-hangzhou.aliyuncs.com/gamepanel-lite/palworld-server:v2.7.1

USER 0:0

# Copy DotNet 8 runtime
COPY --from=dotnet-runtime /opt/tmodloader/dotnet /opt/dotnet

# Setup PATH and symlinks for java, dotnet, steamcmd, steamclient.so, and console helpers
ENV JAVA_HOME=/opt/java/openjdk \
    DOTNET_ROOT=/opt/dotnet \
    STEAMCMDDIR=/opt/steamcmd \
    LANG=C.UTF-8 \
    LC_ALL=C.UTF-8 \
    PATH="/opt/java/openjdk/bin:/opt/dotnet:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin" \
    HOME=/data

RUN apt-get update \
    && apt-get install -y --no-install-recommends libcurl3t64-gnutls xvfb libxi6 libxcursor1 libxrandr2 \
    && rm -rf /var/lib/apt/lists/* \
    && chmod 0755 /home/steam \
    && mv /home/steam/steamcmd /opt/steamcmd \
    && ln -s /opt/steamcmd /home/steam/steamcmd \
    && mkdir -p /opt/steam-home \
    && env -u HTTP_PROXY -u HTTPS_PROXY -u http_proxy -u https_proxy HOME=/opt/steam-home /opt/steamcmd/steamcmd.sh +login anonymous +app_info_update 1 +quit \
    && chmod -R a+rX /opt/steamcmd /opt/steam-home \
    && ln -sf /opt/dotnet/dotnet /usr/local/bin/dotnet \
    && printf '#!/bin/sh\nset -e\nif [ -n "${STEAMCMD_RUNTIME_DIR:-}" ]; then\n  RUNTIME_STEAMCMD="$STEAMCMD_RUNTIME_DIR"\n  STEAM_HOME="${STEAMCMD_HOME_DIR:-/tmp/steam-home}"\nelif [ -d /data ] && [ -w /data ]; then\n  RUNTIME_STEAMCMD="/data/.steam/steamcmd"\n  STEAM_HOME="/data/.steam/home"\nelse\n  RUNTIME_STEAMCMD="/tmp/steamcmd"\n  STEAM_HOME="/tmp/steam-home"\nfi\nif [ ! -x "$RUNTIME_STEAMCMD/steamcmd.sh" ]; then\n  mkdir -p "$RUNTIME_STEAMCMD"\n  cp -a /opt/steamcmd/. "$RUNTIME_STEAMCMD/"\nfi\nif [ ! -d "$STEAM_HOME/Steam" ] && [ -d /opt/steam-home/Steam ]; then\n  mkdir -p "$STEAM_HOME"\n  cp -a /opt/steam-home/. "$STEAM_HOME/"\nfi\nmkdir -p "$STEAM_HOME"\nHOME="$STEAM_HOME" exec "$RUNTIME_STEAMCMD/steamcmd.sh" "$@"\n' > /usr/local/bin/steamcmd \
    && chmod 0755 /usr/local/bin/steamcmd \
    && mkdir -p /root/.steam/sdk32 /root/.steam/sdk64 /data/.steam/sdk32 /data/.steam/sdk64 \
    && ln -sf /opt/steamcmd/linux32/steamclient.so /root/.steam/sdk32/steamclient.so \
    && ln -sf /opt/steamcmd/linux64/steamclient.so /root/.steam/sdk64/steamclient.so \
    && ln -sf /opt/steamcmd/linux32/steamclient.so /data/.steam/sdk32/steamclient.so \
    && ln -sf /opt/steamcmd/linux64/steamclient.so /data/.steam/sdk64/steamclient.so \
    && printf '#!/bin/sh\nprintf "%%s\\n" "$*" > /proc/1/fd/0\n' > /usr/local/bin/mc-send-to-console \
    && chmod 0755 /usr/local/bin/mc-send-to-console \
    && printf '#!/bin/sh\ncmd="$*"\nif [ -f /data/serverconfig.txt ] && grep -qi "^language=zh-Hans" /data/serverconfig.txt; then\n  case "$cmd" in\n    save) cmd="保存" ;;\n    playing) cmd="游戏中" ;;\n    settle) cmd="放置" ;;\n    dawn) cmd="黎明" ;;\n    noon) cmd="正午" ;;\n    dusk) cmd="黄昏" ;;\n    midnight) cmd="午夜" ;;\n    "say "*) cmd="说 ${cmd#say }" ;;\n    "kick "*) cmd="剔除 ${cmd#kick }" ;;\n    "ban "*) cmd="封禁 ${cmd#ban }" ;;\n  esac\nfi\nprintf "%%s\\n" "$cmd" > /proc/1/fd/0\n' > /usr/local/bin/tml-send-to-console \
    && chmod 0755 /usr/local/bin/tml-send-to-console

# Install OpenJDK 25 JRE from official Debian repositories after SteamCMD init so JRE updates preserve SteamCMD layer cache
RUN apt-get update \
    && apt-get install -y --no-install-recommends openjdk-25-jre-headless \
    && rm -rf /var/lib/apt/lists/* \
    && rm -rf /opt/java/openjdk \
    && mkdir -p /opt/java \
    && ln -sfn /usr/lib/jvm/java-25-openjdk-amd64 /opt/java/openjdk \
    && ln -sf /opt/java/openjdk/bin/java /usr/local/bin/java

COPY --chmod=0755 steamcmd-wrapper.sh /usr/local/bin/steamcmd
COPY --chmod=0755 dst-send-to-console.sh /usr/local/bin/dst-send-to-console
COPY --chmod=0755 tml-send-to-console.sh /usr/local/bin/tml-send-to-console
COPY --chmod=0755 7dtd-send-to-console.py /usr/local/bin/7dtd-send-to-console
COPY --chmod=0755 palworld-send-to-console.py /usr/local/bin/palworld-send-to-console
COPY --chmod=0755 entrypoint.sh /usr/local/bin/gamepanel-entrypoint.sh

WORKDIR /data
HEALTHCHECK NONE
STOPSIGNAL SIGTERM
ENTRYPOINT ["/usr/local/bin/gamepanel-entrypoint.sh"]
CMD []
