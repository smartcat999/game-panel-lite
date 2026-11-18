#!/usr/bin/env bash
# Copyright 2026 The GamePanel Cloud Authors.
# Inspired by k8s.io/kubernetes/hack/verify-gofmt.sh & kubesphere/kubesphere/hack/verify-gofmt.sh.

set -o errexit
set -o nounset
set -o pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "${ROOT_DIR}"

diff=$(git ls-files -z -- 'cmd/*.go' 'cmd/**/*.go' 'pkg/*.go' 'pkg/**/*.go' 'tests/*.go' 'tests/**/*.go' | xargs -0 gofmt -s -l)
if [[ -n "${diff}" ]]; then
  echo "The following Go files are not properly formatted (run 'make fmt' or 'gofmt -s -w cmd pkg tests'):" >&2
  printf '%s\n' "${diff}" >&2
  exit 1
fi
