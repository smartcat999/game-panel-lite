#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
builder="${GAMEPANEL_BUILDX_BUILDER:-my-builder}"
platform="${GAMEPANEL_BUILD_PLATFORM:-linux/amd64}"
registry="${GAMEPANEL_IMAGE_REGISTRY:-smartcat99999}"
version="${GAMEPANEL_IMAGE_TAG:-v0.2.5}"
output="--load"

usage() {
  echo "Usage: $0 [--registry REGISTRY] [--version VERSION] [--builder BUILDER] [--platform PLATFORM] [--push|--load]"
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --registry) registry="$2"; shift 2 ;;
    --version) version="$2"; shift 2 ;;
    --builder) builder="$2"; shift 2 ;;
    --platform) platform="$2"; shift 2 ;;
    --push) output="--push"; shift ;;
    --load) output="--load"; shift ;;
    -h|--help) usage; exit 0 ;;
    *) usage >&2; exit 2 ;;
  esac
done

if [[ ! "$version" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+([+-][0-9A-Za-z.-]+)?$ ]]; then
  echo "Invalid version: $version" >&2
  exit 2
fi

commit="$(git -C "$root_dir" rev-parse --short=12 HEAD)"
build_time="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
common=(buildx build --builder "$builder" --platform "$platform" "$output")

docker "${common[@]}" \
  --build-arg "GAMEPANEL_VERSION=$version" \
  --build-arg "GAMEPANEL_COMMIT=$commit" \
  --build-arg "GAMEPANEL_BUILD_TIME=$build_time" \
  -f "$root_dir/docker/api/Dockerfile" \
  -t "$registry/game-panel-lite-api:$version" \
  "$root_dir"

for component in web exporter updater; do
  docker "${common[@]}" \
    -f "$root_dir/docker/$component/Dockerfile" \
    -t "$registry/game-panel-lite-$component:$version" \
    "$root_dir"
done
