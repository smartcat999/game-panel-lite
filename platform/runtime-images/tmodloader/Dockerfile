FROM debian:bookworm-slim

ARG TMOD_VERSION=v2026.07.3.0
ARG TMOD_SHA256=6f51610f4b0f167d1620d0e8c83264ba14e047421e97d9f8053eb40e3d764e1a
ARG STEAMCMD_SHA256=cebf0046bfd08cf45da6bc094ae47aa39ebf4155e5ede41373b579b8f1071e7c

RUN dpkg --add-architecture i386 \
    && apt-get update \
    && apt-get install --no-install-recommends -y bash ca-certificates curl lib32gcc-s1 libc6:i386 libicu72 libsdl2-2.0-0 unzip \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --create-home --uid 10001 --shell /usr/sbin/nologin tml \
    && mkdir -p /opt/tmodloader /opt/gamepanel /data

RUN curl --fail --location --retry 3 --output /tmp/steamcmd.tar.gz https://steamcdn-a.akamaihd.net/client/installer/steamcmd_linux.tar.gz \
    && echo "${STEAMCMD_SHA256}  /tmp/steamcmd.tar.gz" | sha256sum --check - \
    && mkdir -p /opt/steamcmd-seed \
    && tar -xzf /tmp/steamcmd.tar.gz -C /opt/steamcmd-seed \
    && rm /tmp/steamcmd.tar.gz

RUN curl --fail --location --retry 3 --output /tmp/tmodloader.zip "https://github.com/tModLoader/tModLoader/releases/download/${TMOD_VERSION}/tModLoader.zip" \
    && echo "${TMOD_SHA256}  /tmp/tmodloader.zip" | sha256sum --check - \
    && unzip -q /tmp/tmodloader.zip -d /opt/tmodloader \
    && rm /tmp/tmodloader.zip \
    && chmod 0755 /opt/tmodloader/start-tModLoaderServer.sh /opt/tmodloader/LaunchUtils/*.sh \
    && bash -c 'cd /opt/tmodloader/LaunchUtils && root_dir=/opt/tmodloader && LogFile=/tmp/dotnet.log && . ./BashUtils.sh && . ./DotNetVersion.sh && ./InstallDotNet.sh' \
    && chown -R tml:tml /opt/tmodloader /opt/steamcmd-seed /opt/gamepanel /data /home/tml

COPY --chown=tml:tml start-tmodloader.sh /opt/gamepanel/start-tmodloader.sh
RUN chmod 0755 /opt/gamepanel/start-tmodloader.sh

USER tml
ENV HOME=/home/tml
EXPOSE 7777/tcp
ENTRYPOINT []
CMD ["/opt/gamepanel/start-tmodloader.sh", "-nosteam", "-config", "/data/serverconfig.txt"]
