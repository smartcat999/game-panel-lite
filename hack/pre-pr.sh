#!/usr/bin/env bash
# Copyright 2026 The GamePanel Cloud Authors.
# Local pre-PR gate. Remote CI is manual-only (workflow_dispatch), so every PR must pass this
# script locally before `gh pr create`. It runs only the suites affected by the branch diff
# against origin/main; set PRE_PR_ALL=1 to force every suite.
#
#   backend (cmd/ pkg/ tests/ go.mod go.sum hack/ .golangci.yml):
#     hack/verify-all.sh (gofmt, go vet, import-boss, <=60-line rule, golangci-lint) + go test
#   web (web/):
#     eslint --max-warnings=0 + tsc --noEmit
#   miniprogram (miniprogram/):
#     eslint --max-warnings=0 + tsc --noEmit + unit tests + project validation

set -o errexit
set -o nounset
set -o pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "${ROOT_DIR}"

BASE_REF="${PRE_PR_BASE:-origin/main}"

changed_files() {
  if ! git rev-parse --verify --quiet "${BASE_REF}" >/dev/null; then
    return 1
  fi
  {
    git diff --name-only "${BASE_REF}...HEAD"
    git diff --name-only HEAD
    git ls-files --others --exclude-standard
  } | sort -u
}

run_backend=0
run_web=0
run_miniprogram=0
if [[ "${PRE_PR_ALL:-0}" == "1" ]] || ! files="$(changed_files)"; then
  run_backend=1
  run_web=1
  run_miniprogram=1
else
  if grep -qE '^(cmd/|pkg/|tests/|hack/|go\.mod$|go\.sum$|\.golangci\.yml$|Makefile$)' <<<"${files}"; then
    run_backend=1
  fi
  if grep -qE '^web/' <<<"${files}"; then
    run_web=1
  fi
  if grep -qE '^miniprogram/' <<<"${files}"; then
    run_miniprogram=1
  fi
fi

if [[ "${run_backend}" -eq 0 && "${run_web}" -eq 0 && "${run_miniprogram}" -eq 0 ]]; then
  echo "pre-pr: no backend, web or miniprogram changes against ${BASE_REF}; nothing to check."
  exit 0
fi

if [[ "${run_backend}" -eq 1 ]]; then
  echo "==> [backend] verification + golangci-lint"
  VERIFY_GOLANGCI_LINT=1 bash hack/verify-all.sh
  echo "==> [backend] go test"
  go test -count=1 ./cmd/... ./pkg/... ./tests/...
fi

if [[ "${run_web}" -eq 1 ]]; then
  echo "==> [web] eslint --max-warnings=0"
  (cd web && npx --no-install eslint --max-warnings=0 .)
  echo "==> [web] tsc --noEmit"
  (cd web && npx --no-install tsc --noEmit)
fi

if [[ "${run_miniprogram}" -eq 1 ]]; then
  echo "==> [miniprogram] lint + typecheck + tests + project validation"
  pnpm --dir miniprogram check
fi

echo "pre-pr: all affected checks passed."
