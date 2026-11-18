#!/usr/bin/env bash
# Copyright 2026 The GamePanel Cloud Authors.
# Inspired by k8s.io/kubernetes/hack/verify-import-boss.sh.
# Enforces AGENTS.md Section 1 & Section 2 package layering and dependency boundaries.

set -o errexit
set -o nounset
set -o pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "${ROOT_DIR}"

MODULE="github.com/smartcat999/game-panel-cloud"
failed=0

# Rule 7: No internal packages anywhere in the repository.
internal_dirs=$(find cmd pkg tests -type d -name "internal" 2>/dev/null || true)
if [[ -n "${internal_dirs}" ]]; then
  echo "[import-boss] prohibited 'internal' package directories found:" >&2
  printf '%s\n' "${internal_dirs}" >&2
  failed=1
fi

while IFS= read -r line; do
  pkg="${line%% -> *}"
  imports="${line#* -> }"
  rel_pkg="${pkg#${MODULE}/}"

  # Section 1: Zero log/slog usage across the codebase (unified zap logger in pkg/infra/log).
  if [[ " ${imports} " == *" log/slog "* ]]; then
    echo "[import-boss] ${rel_pkg} imports forbidden 'log/slog' (use pkg/infra/log instead)" >&2
    failed=1
  fi

  case "${rel_pkg}" in
    pkg/config|pkg/config/*|pkg/apis/*)
      # Layer 1: Pure configuration and API types — zero runtime/infra/repo/model/kapis/controller/agent/client dependencies.
      for imp in ${imports}; do
        case "${imp}" in
          "${MODULE}/pkg/infra"*|"${MODULE}/pkg/repo"*|"${MODULE}/pkg/models"*|"${MODULE}/pkg/kapis"*|"${MODULE}/pkg/apiserver"*|"${MODULE}/pkg/controller"*|"${MODULE}/pkg/agent"*|"${MODULE}/pkg/client"*)
            echo "[import-boss] Layer 1 package ${rel_pkg} must not import upper layer ${imp#${MODULE}/}" >&2
            failed=1
            ;;
          "${MODULE}/pkg/apis/core"*|"${MODULE}/pkg/apis/iam"*)
            if [[ "${rel_pkg}" == pkg/apis/registry* ]]; then
              echo "[import-boss] Standalone registry API ${rel_pkg} must not import ${imp#${MODULE}/}" >&2
              failed=1
            fi
            ;;
        esac
      done
      ;;
    pkg/infra/*)
      # Layer 2: Low-level drivers — must not import repo, models, kapis, apiserver, controller, or agent.
      for imp in ${imports}; do
        case "${imp}" in
          "${MODULE}/pkg/repo"*|"${MODULE}/pkg/models"*|"${MODULE}/pkg/kapis"*|"${MODULE}/pkg/apiserver"*|"${MODULE}/pkg/controller"*|"${MODULE}/pkg/agent"*)
            echo "[import-boss] Layer 2 package ${rel_pkg} must not import upper layer ${imp#${MODULE}/}" >&2
            failed=1
            ;;
        esac
      done
      ;;
    pkg/repo|pkg/repo/*)
      # Layer 3: Single-domain PostgreSQL repositories — zero cross-domain repo imports and zero upper-layer imports.
      for imp in ${imports}; do
        case "${imp}" in
          "${MODULE}/pkg/models"*|"${MODULE}/pkg/kapis"*|"${MODULE}/pkg/apiserver"*|"${MODULE}/pkg/controller"*|"${MODULE}/pkg/agent"*|"${MODULE}/pkg/client"*)
            echo "[import-boss] Layer 3 package ${rel_pkg} must not import upper layer ${imp#${MODULE}/}" >&2
            failed=1
            ;;
          "${MODULE}/pkg/repo/core/"*|"${MODULE}/pkg/repo/iam/"*|"${MODULE}/pkg/repo/registry/"*)
            if [[ "${rel_pkg}" != "pkg/repo" && "${imp#${MODULE}/}" != "${rel_pkg}" ]]; then
              echo "[import-boss] Layer 3 domain repo ${rel_pkg} must not cross-import ${imp#${MODULE}/}" >&2
              failed=1
            fi
            ;;
          "${MODULE}/pkg/apis/core"*|"${MODULE}/pkg/apis/iam"*)
            if [[ "${rel_pkg}" == pkg/repo/registry/* ]]; then
              echo "[import-boss] Registry domain repo ${rel_pkg} must not import ${imp#${MODULE}/}" >&2
              failed=1
            fi
            ;;
        esac
      done
      ;;
    pkg/models|pkg/models/*)
      # Layer 4: Business operators — must not import HTTP transport, controllers, the node agent, or the typed SDK.
      for imp in ${imports}; do
        case "${imp}" in
          "${MODULE}/pkg/kapis"*|"${MODULE}/pkg/apiserver"*|"${MODULE}/pkg/controller"*|"${MODULE}/pkg/agent"*|"${MODULE}/pkg/client"*)
            echo "[import-boss] Layer 4 package ${rel_pkg} must not import upper layer ${imp#${MODULE}/}" >&2
            failed=1
            ;;
          "${MODULE}/pkg/repo/registry"*|"${MODULE}/pkg/models/registry"*)
            if [[ "${rel_pkg}" == pkg/models/core* ]]; then
              echo "[import-boss] Core model ${rel_pkg} must not cross-import registry slice ${imp#${MODULE}/} (use client.RegistryV1())" >&2
              failed=1
            fi
            ;;
          "${MODULE}/pkg/apis/core"*|"${MODULE}/pkg/repo/core"*|"${MODULE}/pkg/models/core"*)
            if [[ "${rel_pkg}" == pkg/models/registry* ]]; then
              echo "[import-boss] Registry model ${rel_pkg} must not cross-import core slice ${imp#${MODULE}/}" >&2
              failed=1
            fi
            ;;
        esac
      done
      ;;
    pkg/kapis|pkg/kapis/*|pkg/apiserver|pkg/apiserver/*)
      # Layer 5: HTTP handlers and filter chain — must not import controllers or the node agent.
      for imp in ${imports}; do
        case "${imp}" in
          "${MODULE}/pkg/controller"*|"${MODULE}/pkg/agent"*)
            echo "[import-boss] Layer 5 package ${rel_pkg} must not import Layer 6 package ${imp#${MODULE}/}" >&2
            failed=1
            ;;
          "${MODULE}/pkg/repo/registry"*|"${MODULE}/pkg/models/registry"*|"${MODULE}/pkg/kapis/registry"*)
            if [[ "${rel_pkg}" == pkg/kapis/core* ]]; then
              echo "[import-boss] Core KApis ${rel_pkg} must not cross-import registry slice ${imp#${MODULE}/}" >&2
              failed=1
            fi
            ;;
          "${MODULE}/pkg/apis/core"*|"${MODULE}/pkg/repo/core"*|"${MODULE}/pkg/models/core"*|"${MODULE}/pkg/kapis/core"*)
            if [[ "${rel_pkg}" == pkg/kapis/registry* ]]; then
              echo "[import-boss] Registry KApis ${rel_pkg} must not cross-import core slice ${imp#${MODULE}/}" >&2
              failed=1
            fi
            ;;
        esac
      done
      ;;
    pkg/controller/core/*|pkg/controller/registry/*)
      for imp in ${imports}; do
        case "${imp}" in
          "${MODULE}/pkg/repo/registry"*|"${MODULE}/pkg/models/registry"*|"${MODULE}/pkg/controller/registry"*)
            if [[ "${rel_pkg}" == pkg/controller/core/* ]]; then
              echo "[import-boss] Core controller ${rel_pkg} must not cross-import registry slice ${imp#${MODULE}/}" >&2
              failed=1
            fi
            ;;
          "${MODULE}/pkg/apis/core"*|"${MODULE}/pkg/repo/core"*|"${MODULE}/pkg/models/core"*|"${MODULE}/pkg/controller/core"*)
            if [[ "${rel_pkg}" == pkg/controller/registry/* ]]; then
              echo "[import-boss] Registry controller ${rel_pkg} must not cross-import core slice ${imp#${MODULE}/}" >&2
              failed=1
            fi
            ;;
        esac
      done
      ;;
    pkg/agent|pkg/agent/*)
      # Layer 6 (node-agent): Must talk to apiserver via pkg/client, never importing control-plane repo/models/kapis/controller.
      for imp in ${imports}; do
        case "${imp}" in
          "${MODULE}/pkg/repo"*|"${MODULE}/pkg/models"*|"${MODULE}/pkg/kapis"*|"${MODULE}/pkg/apiserver"*|"${MODULE}/pkg/controller"*|"${MODULE}/pkg/registryworker"*)
            echo "[import-boss] Layer 6 agent package ${rel_pkg} must not import control-plane package ${imp#${MODULE}/}" >&2
            failed=1
            ;;
        esac
      done
      ;;
    pkg/registryworker|pkg/registryworker/*)
      # Layer 6 (stateless Harbor-style registry-worker): Zero host/core/iam/control-plane coupling; talks exclusively via client.RegistryV1().
      for imp in ${imports}; do
        case "${imp}" in
          "${MODULE}/pkg/apis/core"*|"${MODULE}/pkg/apis/iam"*|"${MODULE}/pkg/repo"*|"${MODULE}/pkg/models"*|"${MODULE}/pkg/kapis"*|"${MODULE}/pkg/apiserver"*|"${MODULE}/pkg/controller"*|"${MODULE}/pkg/agent"*)
            echo "[import-boss] Standalone registryworker package ${rel_pkg} must not import ${imp#${MODULE}/}" >&2
            failed=1
            ;;
        esac
      done
      ;;
  esac

  # Section 2 Rule 10 (AGENTS.md): Enforce driver/ and providers/<name>/ isolation.
  case "${rel_pkg}" in
    */driver|*/driver/*)
      if [[ "${rel_pkg}" == */driver ]]; then
        driver_parent="${rel_pkg%/driver}"
      else
        driver_parent="${rel_pkg%%/driver/*}"
      fi
      for imp in ${imports}; do
        [[ "${imp}" == "${MODULE}/"* ]] || continue
        rel_imp="${imp#${MODULE}/}"
        if [[ "${rel_imp}" == "${driver_parent}" ]] || { [[ "${rel_imp}" == "${driver_parent}/"* ]] && [[ "${rel_imp}" != "${driver_parent}/driver" ]] && [[ "${rel_imp}" != "${driver_parent}/driver/"* ]]; }; then
          echo "[import-boss] Driver contract package ${rel_pkg} must not import parent or sibling package ${rel_imp}" >&2
          failed=1
        fi
      done
      ;;
    */providers/*)
      prov_parent="${rel_pkg%%/providers/*}"
      prov_rest="${rel_pkg#${prov_parent}/providers/}"
      self_prov="${prov_parent}/providers/${prov_rest%%/*}"
      for imp in ${imports}; do
        [[ "${imp}" == "${MODULE}/"* ]] || continue
        rel_imp="${imp#${MODULE}/}"
        if [[ "${rel_imp}" == "${prov_parent}" ]] || { [[ "${rel_imp}" == "${prov_parent}/"* ]] && [[ "${rel_imp}" != "${prov_parent}/driver" ]] && [[ "${rel_imp}" != "${prov_parent}/driver/"* ]] && [[ "${rel_imp}" != "${self_prov}" ]] && [[ "${rel_imp}" != "${self_prov}/"* ]]; }; then
          echo "[import-boss] Provider package ${rel_pkg} must not import parent or sibling package ${rel_imp} (share contracts via ${prov_parent}/driver)" >&2
          failed=1
        fi
      done
      ;;
  esac

  for imp in ${imports}; do
    [[ "${imp}" == "${MODULE}/"* ]] || continue
    rel_imp="${imp#${MODULE}/}"
    case "${rel_imp}" in
      */providers/*)
        imp_parent="${rel_imp%%/providers/*}"
        imp_rest="${rel_imp#${imp_parent}/providers/}"
        imp_prov="${imp_parent}/providers/${imp_rest%%/*}"
        if [[ "${rel_pkg}" == "${imp_parent}/driver" || "${rel_pkg}" == "${imp_parent}/driver/"* || "${rel_pkg}" == "${imp_parent}/providers/"* ]]; then
          continue
        fi
        if [[ "${rel_pkg}" != "${imp_parent}" && "${rel_pkg}" != "${imp_prov}" && "${rel_pkg}" != "${imp_prov}/"* ]]; then
          echo "[import-boss] ${rel_pkg} must not directly import provider implementation ${rel_imp} (go through ${imp_parent} or ${imp_parent}/driver)" >&2
          failed=1
        fi
        ;;
    esac
  done
done < <(go list -f '{{.ImportPath}} -> {{join .Imports " "}}' ./cmd/... ./pkg/...)

if [[ "${failed}" -ne 0 ]]; then
  exit 1
fi
