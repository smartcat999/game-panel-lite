#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Build GamePanel Lite game runtime images.

Usage:
  scripts/build-game-images.sh [all|vanilla|tmodloader|dst] [options]

Targets:
  all           Build vanilla Terraria and tModLoader images. Default.
  vanilla       Build only vanilla Terraria images.
  tmodloader    Build only tModLoader images.
  dst           Build only Don't Starve Together images. linux/amd64 only.

Options:
  --registry NAME       Image namespace. Default: smartcat99999
  --platform LIST       Docker platform list. Default: current Docker builder platform
  --builder NAME        Docker buildx builder name
  --push                Push images to registry. Required for multi-platform builds
  --load                Load images into local Docker. Default when --push is not set
  --no-cache            Build without cache
  -h, --help            Show this help

Examples:
  scripts/build-game-images.sh
  scripts/build-game-images.sh vanilla --platform linux/arm64 --load
  scripts/build-game-images.sh all --platform linux/amd64,linux/arm64 --push
  scripts/build-game-images.sh dst --platform linux/amd64 --load
  scripts/build-game-images.sh dst --builder amd64-builder --platform linux/amd64 --push
USAGE
}

target="all"
registry="smartcat99999"
platform=""
builder=""
output="--load"
no_cache=""

if [[ $# -gt 0 && "$1" != --* ]]; then
  target="$1"
  shift
fi

while [[ $# -gt 0 ]]; do
  case "$1" in
    --registry)
      registry="${2:?missing value for --registry}"
      shift 2
      ;;
    --platform)
      platform="${2:?missing value for --platform}"
      shift 2
      ;;
    --builder)
      builder="${2:?missing value for --builder}"
      shift 2
      ;;
    --push)
      output="--push"
      shift
      ;;
    --load)
      output="--load"
      shift
      ;;
    --no-cache)
      no_cache="--no-cache"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

case "$target" in
  all|vanilla|tmodloader|dst) ;;
  *)
    echo "Unknown target: $target" >&2
    usage >&2
    exit 1
    ;;
esac

if [[ "$platform" == *,* && "$output" == "--load" ]]; then
  echo "Multi-platform builds cannot use --load. Use --push instead." >&2
  exit 1
fi

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

buildx_args=(buildx build)
if [[ -n "$builder" ]]; then
  buildx_args+=(--builder "$builder")
fi
if [[ -n "$platform" ]]; then
  buildx_args+=(--platform "$platform")
fi
if [[ -n "$no_cache" ]]; then
  buildx_args+=("$no_cache")
fi
buildx_args+=("$output")

build_vanilla() {
  local version="$1"
  local download_id="$2"
  local image="${registry}/terraria-vanilla:${version}"

  echo "==> Building ${image}"
  docker "${buildx_args[@]}" \
    -f docker/terraria-vanilla/Dockerfile \
    --build-arg "TERRARIA_VERSION=${version}" \
    --build-arg "TERRARIA_DOWNLOAD_ID=${download_id}" \
    -t "${image}" \
    "${root_dir}"
}

build_tmodloader() {
  local version="$1"
  local image="${registry}/tmodloader:${version}"

  echo "==> Building ${image}"
  docker "${buildx_args[@]}" \
    -f docker/tmodloader/Dockerfile \
    --build-arg "TML_VERSION=${version}" \
    -t "${image}" \
    "${root_dir}"
}

build_dst() {
  local version="$1"
  local image="${registry}/dst-server:${version}"
  local dst_buildx_args=("${buildx_args[@]}")

  if [[ -n "$platform" && "$platform" != "linux/amd64" ]]; then
    echo "Don't Starve Together image builds are currently supported only for linux/amd64." >&2
    exit 1
  fi
  if [[ -n "${STEAM_USERNAME:-}" && -z "${STEAM_PASSWORD:-}" || -z "${STEAM_USERNAME:-}" && -n "${STEAM_PASSWORD:-}" ]]; then
    echo "Set both STEAM_USERNAME and STEAM_PASSWORD when building the Don't Starve Together image with Steam credentials." >&2
    exit 1
  fi
  if [[ -n "${STEAM_USERNAME:-}" ]]; then
    dst_buildx_args+=(--secret id=steam_username,env=STEAM_USERNAME)
  fi
  if [[ -n "${STEAM_PASSWORD:-}" ]]; then
    dst_buildx_args+=(--secret id=steam_password,env=STEAM_PASSWORD)
  fi

  echo "==> Building ${image}"
  docker "${dst_buildx_args[@]}" \
    -f docker/dst/Dockerfile \
    -t "${image}" \
    "${root_dir}"
}

cd "$root_dir"

if [[ "$target" == "all" || "$target" == "vanilla" ]]; then
  build_vanilla "1.4.5.6" "1456"
fi

if [[ "$target" == "all" || "$target" == "tmodloader" ]]; then
  build_tmodloader "v2026.04.3.0"
  build_tmodloader "v2026.02.3.1"
fi

if [[ "$target" == "dst" ]]; then
  build_dst "v2026.06.21"
fi

echo "Done."
