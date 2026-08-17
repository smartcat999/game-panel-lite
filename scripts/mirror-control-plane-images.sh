#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
builder="${GAMEPANEL_BUILDX_BUILDER:-my-builder}"
platform="${GAMEPANEL_BUILD_PLATFORM:-linux/amd64}"
dockerhub_registry="${GAMEPANEL_DOCKERHUB_REGISTRY:-smartcat99999}"
aliyun_registry="${GAMEPANEL_ALIYUN_REGISTRY:-registry.cn-hangzhou.aliyuncs.com/gamepanel-lite}"
output="--load"
selected_image=""

images=(
  "nginx|1.27-alpine|nginx:1.27-alpine@sha256:65645c7bb6a0661892a8b03b89d0743208a18dd2f3f17a54ef4b76fb8e2f2a10"
  "certbot|v2.11.0|certbot/certbot:v2.11.0@sha256:ddf9e5d226a56e886986838fa0ebedc0237511c78664352e8d0f4346ee022cd8"
  "prometheus|v2.55.1|prom/prometheus:v2.55.1@sha256:2659f4c2ebb718e7695cb9b25ffa7d6be64db013daba13e05c875451cf51b0d3"
  "cadvisor|v0.52.1|gcr.io/cadvisor/cadvisor:v0.52.1@sha256:f40e65878e25c2e78ea037f73a449527a0fb994e303dc3e34cb6b187b4b91435"
  "node-exporter|v1.10.2|prom/node-exporter:v1.10.2@sha256:3ac34ce007accad95afed72149e0d2b927b7e42fd1c866149b945b84737c62c3"
)

usage() {
  cat <<'EOF'
Usage: scripts/mirror-control-plane-images.sh [options]

Options:
  --dockerhub-registry REGISTRY  Docker Hub namespace (default: smartcat99999)
  --aliyun-registry REGISTRY     Alibaba Cloud namespace
  --builder BUILDER              buildx builder (default: my-builder)
  --platform PLATFORM            target platform (default: linux/amd64)
  --image NAME                   mirror only one managed image
  --push                         push both registry tags
  --load                         load mirrored tags locally (default)
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --dockerhub-registry) dockerhub_registry="$2"; shift 2 ;;
    --aliyun-registry) aliyun_registry="$2"; shift 2 ;;
    --builder) builder="$2"; shift 2 ;;
    --platform) platform="$2"; shift 2 ;;
    --image) selected_image="$2"; shift 2 ;;
    --push) output="--push"; shift ;;
    --load) output="--load"; shift ;;
    -h|--help) usage; exit 0 ;;
    *) usage >&2; exit 2 ;;
  esac
done

if [[ "$platform" != "linux/amd64" ]]; then
  echo "Control-plane mirrors must target linux/amd64, got: $platform" >&2
  exit 2
fi

docker buildx inspect "$builder" >/dev/null

matched="false"
for definition in "${images[@]}"; do
  IFS='|' read -r name tag upstream <<<"$definition"
  if [[ -n "$selected_image" && "$selected_image" != "$name" ]]; then
    continue
  fi
  matched="true"
  echo "Mirroring $upstream"
  docker buildx build \
    --builder "$builder" \
    --platform "$platform" \
    --pull \
    --provenance=false \
    "$output" \
    --build-arg "UPSTREAM_IMAGE=$upstream" \
    -f "$root_dir/docker/upstream-mirror/Dockerfile" \
    -t "$dockerhub_registry/$name:$tag" \
    -t "$aliyun_registry/$name:$tag" \
    "$root_dir/docker/upstream-mirror"
done

if [[ "$matched" != "true" ]]; then
  echo "Unknown managed image: $selected_image" >&2
  exit 2
fi
