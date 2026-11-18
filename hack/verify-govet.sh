#!/usr/bin/env bash
# Copyright 2026 The GamePanel Cloud Authors.
# Inspired by k8s.io/kubernetes/hack/verify-govet.sh & kubesphere/kubesphere/hack/verify-govet.sh.

set -o errexit
set -o nounset
set -o pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "${ROOT_DIR}"

go vet ./cmd/... ./pkg/... ./tests/...
