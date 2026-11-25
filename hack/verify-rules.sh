#!/usr/bin/env bash
# Copyright 2026 The GamePanel Cloud Authors.
# Enforces AGENTS.md Section 2 Rule 8 (<= 60 lines per Go function across cmd/ and pkg/),
# Rule 9 (canonical constants), Rule 10 (driver/providers/register.go modular layout),
# and Resource-Control Atomicity Rule 1 (no explicit FOR UPDATE in pkg/repo).

set -o errexit
set -o nounset
set -o pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "${ROOT_DIR}"

failed=0

# 1. Strict Function Length Limit (<= 60 lines across cmd/ and pkg/)
long_funcs=$(git ls-files -- 'cmd/*.go' 'cmd/**/*.go' 'pkg/*.go' 'pkg/**/*.go' | xargs awk '
  FNR == 1 { start = 0 }
  /^(func |var [A-Za-z0-9_]+ = func\()/ {
    if ($0 ~ /\{.*\}[[:space:]]*$/) { start = 0; next }
    start = FNR; sig = $0; next
  }
  /^}/ {
    if (start > 0) {
      len = FNR - start + 1
      if (len > 60) {
        printf "%s:%d: function length %d exceeds 60 lines (%s)\n", FILENAME, start, len, sig
      }
      start = 0
    }
  }
')

if [[ -n "${long_funcs}" ]]; then
  echo "[verify-rules] Functions exceeding the 60-line limit (AGENTS.md Section 2 Rule 8):" >&2
  printf '%s\n' "${long_funcs}" >&2
  failed=1
fi

# 2. No explicit FOR UPDATE in pkg/repo (AGENTS.md Resource-Control Atomicity Rule 1)
for_update_hits=$(git grep -n -E 'clause\.Locking\{Strength:\s*"UPDATE"\}|FOR UPDATE' -- 'pkg/repo/*.go' 'pkg/repo/**/*.go' | grep -v '_test\.go:' | grep -v '//' || true)
if [[ -n "${for_update_hits}" ]]; then
  echo "[verify-rules] Explicit FOR UPDATE is forbidden in pkg/repo (use optimistic CAS instead):" >&2
  printf '%s\n' "${for_update_hits}" >&2
  failed=1
fi

# 3. Canonical Constant Management (no raw string literals for Type/Status/Reason/Phase/Stage or inline label/annotation keys in pkg/, AGENTS.md Section 2 Rule 9)
raw_const_hits=$(git grep -n -E '(^|[^[:alnum:]_])(Type|Status|Reason|Phase|Stage):[[:space:]]*"[^"]+"' -- 'pkg/*.go' 'pkg/**/*.go' | grep -v '_test\.go:' | grep -v '//' || true)
if [[ -n "${raw_const_hits}" ]]; then
  echo "[verify-rules] Raw string literals for Type/Status/Reason/Phase/Stage are forbidden in pkg/ (AGENTS.md Section 2 Rule 9):" >&2
  printf '%s\n' "${raw_const_hits}" >&2
  failed=1
fi

raw_label_hits=$(git grep -n -E '"([a-z0-9-]+\.)?gamepanel\.io/[^v]' -- 'pkg/*.go' 'pkg/**/*.go' | grep -v '_test\.go:' | grep -v '^pkg/apis/' || true)
if [[ -n "${raw_label_hits}" ]]; then
  echo "[verify-rules] Inline label/annotation string literals are forbidden in pkg/ (use canonical constants in pkg/apis/*):" >&2
  printf '%s\n' "${raw_label_hits}" >&2
  failed=1
fi

# 4. Modular Extension-Point Layout (AGENTS.md Section 2 Rule 10)
MODULE="github.com/smartcat999/game-panel-cloud"
modular_dirs=$(find pkg -type d \( -name "driver" -o -name "providers" \) | sed -E 's#/(driver|providers)$##' | sort -u)
for mod_dir in ${modular_dirs}; do
  if [[ ! -d "${mod_dir}/driver" ]]; then
    echo "[verify-rules] Modular package ${mod_dir} is missing required driver/ subpackage" >&2
    failed=1
  fi
  if [[ ! -d "${mod_dir}/providers" ]]; then
    echo "[verify-rules] Modular package ${mod_dir} is missing required providers/ directory" >&2
    failed=1
  fi
  if [[ ! -f "${mod_dir}/register.go" ]]; then
    echo "[verify-rules] Modular package ${mod_dir} is missing required register.go" >&2
    failed=1
  fi

  flat_hits=$(find "${mod_dir}" -maxdepth 1 -type f \( -name 'driver_*.go' -o -name 'provider_*.go' -o -name '*_driver.go' -o -name '*_provider.go' \) | sort || true)
  if [[ -n "${flat_hits}" ]]; then
    echo "[verify-rules] Flat driver/provider files are forbidden in ${mod_dir} (use providers/<name>/ instead):" >&2
    printf '%s\n' "${flat_hits}" >&2
    failed=1
  fi

  if [[ -d "${mod_dir}/providers" ]]; then
    while IFS= read -r prov_dir; do
      [[ -z "${prov_dir}" ]] && continue
      init_hits=$(find "${prov_dir}" -maxdepth 1 -type f -name '*.go' ! -name '*_test.go' -exec grep -l 'func init(' {} + 2>/dev/null || true)
      if [[ -z "${init_hits}" ]]; then
        echo "[verify-rules] Provider subpackage ${prov_dir} must self-register via func init()" >&2
        failed=1
      fi
      if [[ -f "${mod_dir}/register.go" ]] && ! grep -qF "${MODULE}/${prov_dir}\"" "${mod_dir}/register.go"; then
        echo "[verify-rules] ${mod_dir}/register.go must import provider ${prov_dir}" >&2
        failed=1
      fi
    done < <(find "${mod_dir}/providers" -mindepth 1 -maxdepth 1 -type d | sort)
  fi

  direct_prov_hits=$(find "${mod_dir}" -maxdepth 1 -type f -name '*.go' ! -name 'register.go' ! -name '*_test.go' -exec grep -Hn "${MODULE}/${mod_dir}/providers/" {} + 2>/dev/null || true)
  if [[ -n "${direct_prov_hits}" ]]; then
    echo "[verify-rules] Non-register.go files in ${mod_dir} must not import concrete providers directly (dispatch via driver registry):" >&2
    printf '%s\n' "${direct_prov_hits}" >&2
    failed=1
  fi
done

# 5. GORM persistence records must live in pkg/apis/*/v1/records.go (AGENTS.md Section 2 Rule 1)
gorm_outside_records=$(git grep -n 'gorm:"' -- 'pkg/apis/*.go' 'pkg/apis/**/*.go' | grep -v '/records\.go:' || true)
if [[ -n "${gorm_outside_records}" ]]; then
  echo "[verify-rules] GORM table models must live in pkg/apis/*/v1/records.go (AGENTS.md Section 2 Rule 1):" >&2
  printf '%s\n' "${gorm_outside_records}" >&2
  failed=1
fi

if [[ "${failed}" -ne 0 ]]; then
  exit 1
fi
