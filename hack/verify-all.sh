#!/usr/bin/env bash
# Copyright 2026 The GamePanel Cloud Authors.
# Runs all Kubernetes / KubeSphere-style backend verification checks.

set -o errexit
set -o nounset
set -o pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "${ROOT_DIR}"

CHECKS=(
  "hack/verify-gofmt.sh"
  "hack/verify-govet.sh"
  "hack/verify-imports.sh"
  "hack/verify-rules.sh"
)

if [[ "${VERIFY_GOLANGCI_LINT:-0}" == "1" ]]; then
  CHECKS+=("hack/verify-golangci-lint.sh")
fi

for check in "${CHECKS[@]}"; do
  echo "==> Running ${check}..."
  bash "${ROOT_DIR}/${check}"
  echo "    [PASS] ${check}"
done

echo "All backend verification checks passed."
