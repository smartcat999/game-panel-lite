#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: scripts/build-seven-days-images.sh [runtime|base] [options]

runtime builds only startup/configuration scripts on the digest-pinned base.
base explicitly builds game programs and dependencies; it never runs implicitly.

Options:
  --registry NAME          Output namespace. Default: smartcat99999
  --mirror-registry NAME   Also push to this namespace; requires --push
  --builder NAME          Docker buildx builder
  --platform PLATFORM     linux/amd64 only (default)
  --version TAG           Runtime tag. Default: v3.3.0
  --base-image REF        Runtime input, must use @sha256:<64 hex digits>
  --base-build-id ID      Required for base; verifies installed Steam Build ID
  --game-source-image REF Base only: reuse verified game/dependency layers
                          pinned by digest instead of downloading from Steam
  --debian-mirror ORIGIN  Base only: optional Debian mirror origin
  --push | --load         Push to registries or load locally (default)
  --no-cache              Disable build cache; runtime still never invokes Steam
  -h, --help              Show help
USAGE
}

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
target="runtime"
if [[ $# -gt 0 && "$1" != -* ]]; then target="$1"; shift; fi
registry="smartcat99999"
mirror_registry=""
builder=""
platform="linux/amd64"
version="v3.3.0"
version_set=""
base_image=""
base_build_id=""
game_source_image=""
debian_mirror=""
output="--load"
no_cache=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --registry|--mirror-registry|--builder|--platform|--version|--base-image|--base-build-id|--game-source-image|--debian-mirror)
      if [[ $# -lt 2 || -z "$2" || "$2" == --* ]]; then
        echo "Missing value for $1" >&2; exit 2
      fi
      case "$1" in
        --registry) registry="${2%/}" ;;
        --mirror-registry) mirror_registry="${2%/}" ;;
        --builder) builder="$2" ;;
        --platform) platform="$2" ;;
        --version) version="$2"; version_set=1 ;;
        --base-image) base_image="$2" ;;
        --base-build-id) base_build_id="$2" ;;
        --game-source-image) game_source_image="$2" ;;
        --debian-mirror) debian_mirror="${2%/}" ;;
      esac
      shift 2 ;;
    --push|--load) output="$1"; shift ;;
    --no-cache) no_cache="--no-cache"; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; usage >&2; exit 2 ;;
  esac
done

fail() { echo "$*" >&2; exit 2; }
is_pinned() { [[ "$1" =~ ^[^[:space:]@]+@sha256:[a-f0-9]{64}$ ]]; }
[[ "$target" == runtime || "$target" == base ]] || fail "Unknown target: $target"
[[ "$platform" == linux/amd64 ]] || fail "7 Days to Die supports linux/amd64 only"
[[ "$version" =~ ^[a-zA-Z0-9_][a-zA-Z0-9_.-]{0,127}$ ]] || fail "Invalid runtime tag"
[[ -z "$mirror_registry" || "$output" == --push ]] || fail "--mirror-registry requires --push"

args=(buildx build --platform "$platform" "$output")
if [[ -n "$builder" ]]; then args+=(--builder "$builder"); fi
if [[ -n "$no_cache" ]]; then args+=(--no-cache); fi
if [[ "$target" == base ]]; then
  [[ "$base_build_id" =~ ^[1-9][0-9]*$ ]] || fail "base requires --base-build-id with the expected Steam Build ID"
  [[ -z "$base_image" ]] || fail "--base-image is for runtime builds"
  [[ -z "$version_set" ]] || fail "--version is for runtime builds; base tags follow the Build ID"
  image="seven-days-to-die-base:build-${base_build_id}"
  args+=(-f "$root_dir/docker/seven-days-to-die/Dockerfile.base" --build-arg "SEVEN_DAYS_BUILD_ID=$base_build_id")
  if [[ -n "$debian_mirror" ]]; then
    [[ "$debian_mirror" =~ ^https?://[a-zA-Z0-9.-]+(:[0-9]+)?$ ]] || fail "--debian-mirror must be an HTTP(S) origin"
    args+=(--build-arg "DEBIAN_MIRROR=$debian_mirror")
  fi
  if [[ -n "$game_source_image" ]]; then
    [[ -z "$debian_mirror" ]] || fail "Migration keeps verified dependencies; --debian-mirror requires a Steam base build"
    is_pinned "$game_source_image" || fail "--game-source-image must be pinned by digest"
    args+=(--build-arg "SEVEN_DAYS_BASE_STAGE=imported-base" --build-context "verified-runtime=docker-image://$game_source_image")
  fi
else
  [[ -z "$base_build_id" && -z "$game_source_image" && -z "$debian_mirror" ]] || fail "Game-file inputs require the base target"
  image="seven-days-to-die:$version"
  args+=(-f "$root_dir/docker/seven-days-to-die/Dockerfile")
  if [[ -z "$base_image" ]]; then
    base_image="$(sed -n 's/^ARG SEVEN_DAYS_BASE_IMAGE=//p' "$root_dir/docker/seven-days-to-die/Dockerfile")"
  fi
  is_pinned "$base_image" || fail "--base-image and the Dockerfile default must be pinned by digest"
  args+=(--build-arg "SEVEN_DAYS_BASE_IMAGE=$base_image")
fi
args+=(-t "$registry/$image")
if [[ -n "$mirror_registry" ]]; then args+=(-t "$mirror_registry/$image"); fi
echo "Building $registry/$image ($target)"
docker "${args[@]}" "$root_dir"
