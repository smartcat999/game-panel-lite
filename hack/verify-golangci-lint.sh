#!/usr/bin/env bash
# Copyright 2026 The GamePanel Cloud Authors.
# Inspired by k8s.io/kubernetes/hack/verify-golangci-lint.sh & kubesphere/kubesphere/hack.

set -o errexit
set -o nounset
set -o pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "${ROOT_DIR}"

GOLANGCI_LINT_VERSION="${GOLANGCI_LINT_VERSION:-v2.4.0}"
LOCAL_BIN="${ROOT_DIR}/bin/golangci-lint"

resolve_golangci_lint() {
  if [[ -x "${LOCAL_BIN}" ]]; then
    echo "${LOCAL_BIN}"
    return
  fi
  if command -v golangci-lint >/dev/null 2>&1; then
    # Verify the installed golangci-lint was built with a Go version compatible with go.mod.
    if golangci-lint run --version >/dev/null 2>&1; then
      local ver_out
      ver_out="$(golangci-lint --version 2>/dev/null || true)"
      if [[ "${ver_out}" == *"go1.25"* || "${ver_out}" == *"go1.26"* ]]; then
        command -v golangci-lint
        return
      fi
    fi
  fi
  echo "Installing golangci-lint ${GOLANGCI_LINT_VERSION} into ${LOCAL_BIN}..." >&2
  mkdir -p "${ROOT_DIR}/bin"
  GOBIN="${ROOT_DIR}/bin" go install "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@${GOLANGCI_LINT_VERSION}"
  echo "${LOCAL_BIN}"
}

LINTER="$(resolve_golangci_lint)"
"${LINTER}" run --config "${ROOT_DIR}/.golangci.yml" ./cmd/... ./pkg/... ./tests/...
