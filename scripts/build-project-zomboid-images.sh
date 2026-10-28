#!/usr/bin/env bash
set -euo pipefail
usage() {
  cat <<'USAGE'
Usage: scripts/build-project-zomboid-images.sh [runtime|base] [options]

Builds locally with --load; never downloads games or pushes images.
base reuses a complete official Steam 380870 public install (Build 25485538).
runtime changes only helpers above an explicitly selected base.

Options:
  --game-files-dir DIR   Required for base: complete official installation
  --base-build-id ID     Required for base: 25485538
  --base-image REF       Required for runtime: digest-pinned base
  --local-base-image REF Runtime only: explicit local candidate tag (no push)
  --registry NAME       Namespace, default gamepanel-local
  --version TAG         Runtime tag, default candidate-42.21
  --builder NAME        Optional buildx builder
  --network MODE        Build network: default or host
  --platform PLATFORM   linux/amd64 only
  --no-cache            Disable build cache (still no Steam download)
  --load                Load locally (default and only output)
  -h, --help            Show help
USAGE
}
fail() { echo "$*" >&2; exit 2; }
root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
target=runtime
if [[ $# -gt 0 && "$1" != -* ]]; then target="$1"; shift; fi
game_files=""; build_id=""; base_image=""; local_base=""; registry=gamepanel-local
version=candidate-42.21; builder=""; platform=linux/amd64; no_cache=""; network=default
while [[ $# -gt 0 ]]; do
  case "$1" in
    --game-files-dir|--base-build-id|--base-image|--local-base-image|--registry|--version|--builder|--platform|--network)
      [[ $# -ge 2 && -n "$2" && "$2" != --* ]] || fail "Missing value for $1"
      case "$1" in
        --game-files-dir) game_files="$2";; --base-build-id) build_id="$2";;
        --base-image) base_image="$2";; --local-base-image) local_base="$2";;
        --registry) registry="${2%/}";; --version) version="$2";;
        --builder) builder="$2";; --platform) platform="$2";; --network) network="$2";;
      esac
      shift 2;;
    --no-cache) no_cache=1; shift;; --load) shift;;
    -h|--help) usage; exit 0;;
    *) fail "Unknown option: $1";;
  esac
done
[[ "$target" == base || "$target" == runtime ]] || fail "Unknown target: $target"
[[ "$platform" == linux/amd64 ]] || fail "Project Zomboid requires linux/amd64"
[[ "$version" =~ ^[a-zA-Z0-9_][a-zA-Z0-9_.-]{0,127}$ ]] || fail "Invalid version tag"
[[ "$registry" =~ ^[a-z0-9][a-z0-9./:_-]*$ ]] || fail "Invalid registry namespace"
[[ "$network" == default || "$network" == host ]] || fail "Invalid build network"
args=(buildx build --platform "$platform" --network "$network" --load)
[[ -z "$builder" ]] || args+=(--builder "$builder")
[[ -z "$no_cache" ]] || args+=(--no-cache)
if [[ "$target" == base ]]; then
  [[ "$build_id" == 25485538 ]] || fail "base requires --base-build-id 25485538"
  [[ -z "$base_image$local_base" ]] || fail "Base image inputs require runtime target"
  [[ -n "$game_files" && -d "$game_files" ]] || fail "base requires --game-files-dir"
  game_files="$(cd "$game_files" && pwd)"
  [[ -f "$game_files/ProjectZomboid64" && -f "$game_files/steamapps/appmanifest_380870.acf" ]] || fail "Incomplete official game files"
  args+=(-f "$root_dir/docker/project-zomboid/Dockerfile.base" --build-context "gamefiles=$game_files" --build-arg "PZ_BUILD_ID=$build_id")
  image="project-zomboid-base:build-$build_id"
else
  [[ -z "$game_files$build_id" ]] || fail "Game-file inputs require base target"
  [[ -z "$base_image" || -z "$local_base" ]] || fail "Choose one base input"
  if [[ -n "$local_base" ]]; then
    [[ "$local_base" =~ ^[a-z0-9][a-zA-Z0-9./:_-]*$ ]] || fail "Invalid local candidate base"
    base_image="$local_base"
  else
    [[ "$base_image" =~ ^[^[:space:]@]+@sha256:[a-f0-9]{64}$ ]] || fail "runtime requires --base-image pinned by digest (or explicit --local-base-image for a candidate)"
  fi
  image="project-zomboid:$version"
  args+=(-f "$root_dir/docker/project-zomboid/Dockerfile" --build-arg "PZ_BASE_IMAGE=$base_image")
fi
args+=(-t "$registry/$image")
echo "Building local $registry/$image ($target)"
docker "${args[@]}" "$root_dir"
