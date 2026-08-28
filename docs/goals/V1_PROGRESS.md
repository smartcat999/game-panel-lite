# V1 Progress

## 2026-08-28

- Replaced imperative remote lifecycle tasks with durable workload assignments and worker observations: the API now writes desired state, the worker reconciles real Docker state, and the controller derives status only from matching assignment UID and generation.
- Added worker-side Docker reconciliation with managed workload labels, generation-aware replacement, traversal-safe instance files, authenticated observation reporting, and rejection of legacy lifecycle tasks.
- Fixed remote Terraria console input to attach to container stdin instead of executing game commands through a shell, and made log snapshots retry by content after failed uploads while suppressing overlapping history.
- Restricted task polling, task acknowledgements, assignment reads, observation writes, and log uploads to the authenticated node that owns the resource.
- Added assignment persistence, controller convergence, worker reconciliation, path-safety, and log-delta tests; full `go test ./...` and `go vet ./...` pass.

## 2026-08-17

- Prepared v0.2.4 to publish every control-plane dependency through GamePanel Lite's Docker Hub and Alibaba Cloud registries, pin upstream digests, and refresh monitoring containers during panel updates so regional deployments never mix registries.
- Prepared v0.2.3 to force-recreate Nginx during panel updates, preventing stale upstream container addresses from taking the public site offline after API/Web replacement.
- Prepared v0.2.2 with scalable mobile Settings navigation that switches from desktop tabs to a full-width section selector on narrow viewports.
- Published the v0.2.1 maintenance release metadata for control-plane operations, deployment-driver capabilities, HTTPS renewal observability, stable resource tables, corrected license labels, and revised operations documentation.
- Integrated fixed-operation control-plane maintenance into Settings with authenticated deployment status, service recovery, confirmed restarts, HTTPS setup and renewal jobs, certificate and automatic-renewal inspection, a daily Updater renewal scheduler that defers to existing systemd timers, persistent job state, bilingual UI, and OpenAPI coverage; deliberately kept full control-plane shutdown command-line only so the panel cannot lock users out of its own recovery action.
- Added authenticated asynchronous GamePanel Lite release checks and self-update orchestration with a fixed-operation updater service, persistent job state, explicit confirmation UI, daily notification-only checks, build metadata, release manifests, and buildx-based control-plane image publishing.
- Reworked the dashboard, monitoring, server, mod, preset, version, and settings surfaces around denser resource-management patterns, consistent filters, stable feedback, and responsive bulk actions.
- Added backend-supported server pagination and batch operations for server, mod-library, mod-pack, and configuration-preset management, including stopped-server deletion safeguards and per-item failure reporting.
- Added editable configuration presets with localized schema details, resource and mod summaries, batch deletion, and consistent game / mode terminology; verified the full Go and frontend test, vet, lint, typecheck, and production-build suites.

## 2026-08-11

- Completed the remaining Issue #61 tModLoader `ModConfigs` management flow in the server Mods tab with list, JSON editor, upload, delete, and explicit restart-required feedback.
- Added provider-scoped backend endpoints restricted to top-level `.json` files in the current server data directory, including traversal and symlink rejection, a 1 MiB limit, JSON-object validation, atomic writes, lifecycle/update locking, activity localization, tests, and OpenAPI coverage.

## 2026-08-10

- Changed every Steam-backed mod-library import path to preview current Workshop metadata before writing data, including direct IDs, recommended Workshop cards, collections, and collection-based mod packs.
- Added a short-lived individual-item preview API with provider validation, DST server-mod filtering, library-state detection, metadata caching, and confirmation-token validation for selected items.
- Unified the bilingual confirmation UI around current titles, images, sizes, availability, dependencies, and import counts; unavailable or client-only entries remain visible but cannot be selected.
- Added backend resolver/handler coverage, frontend API coverage, Playwright flow coverage, and OpenAPI documentation; full Go tests/vet, 102 frontend tests, lint, typecheck, and production build pass. Browser execution remains unavailable locally because the matching Playwright Chromium binary is not installed.

## 2026-07-29

- Added server-side Steam Workshop collection resolution for tModLoader and Don't Starve Together using fixed official API endpoints, strict Steam URL parsing, provider AppID validation, bounded nested collection expansion, response size limits, and request timeouts without reading Steam cookies or credentials.
- Added a short-lived Workshop collection preview flow that reports new, existing, installed, and unavailable items, retains server-fetched metadata for confirmed imports, and prevents importing IDs that were not present in the preview.
- Made global Workshop batch imports idempotent and transactional so existing items refresh safely and failures cannot leave a partially-created batch.
- Reworked the Mods import dialog into Steam Collection and Workshop ID paths with live metadata, selectable differences, ARM preview-only guidance, and bilingual copy.
- Added Go coverage for Steam URL validation, nested collection resolution, game filtering, unavailable collections, preview caching, and idempotent imports; extended the OpenAPI contract with the preview endpoint and response schema.

## 2026-07-19

- Removed deprecated DST `-console` and `offline_server` usage while retaining console control through the supported cluster setting.
- Fixed cave shard creation by publishing cave-specific defaults for biome and spawn-area generation settings and transparently repairing legacy payloads that stored their forest-only `default` values.
- Separated DST mod synchronization by explicit lifecycle action: Start reuses the verified persistent Workshop cache and downloads only missing entries, while Restart refreshes every configured server mod through a validated staging directory and atomically replaces the live cache only after a complete download.
- Blocked DST client-only and unclassified Workshop mods from entering the server mod library or server installer in the frontend, while preserving raw Workshop tags in discovery and separating server-only from all-clients-required labeling without changing backend behavior.
- Prevented configuration and mod edits from implicitly restarting or starting game servers; saved revisions now wait for an explicit restart/start while preserving the current lifecycle state.
- Aligned resource slider value badges exactly over their thumbs and fixed endpoint column widths so CPU and memory tracks share identical horizontal geometry.
- Changed GamePanel Lite's Palworld new-server death-penalty default from the official `All` setting to the friendlier `None` setting across schema defaults, normalization, and runtime environment generation; existing saved servers remain unchanged.
- Stacked CPU and memory resource controls vertically in both server creation and server details, and removed redundant per-step tick labels and memory recommendations, keeping only endpoints and the thumb-aligned current value.
- Restored the delete-server action to the mobile server-detail overflow menu while retaining the existing destructive confirmation flow.
- Unified creation and server-detail resource limits around compact sliders with thumb-aligned values, every whole-core CPU marker on common hosts, readable memory capacity markers, and a detected recommended memory ceiling.
- Collapsed provider boolean settings into single-row label-and-switch controls and replaced per-field reset icons with one advanced-settings-wide restore-defaults action.
- Distinguished DST world settings from world-generation options in the server editor with explicit restart/regeneration applicability, a direct route into the existing high-risk regeneration flow, and no misleading restart prompt after world-generation-only saves.
- Stabilized provider field header geometry so the modified-field reset action can appear or disappear without shifting Palworld slider controls.
- Merged Palworld slider bounds into the control row as `minimum | track | maximum`, removing the separate range-caption row while retaining the thumb-anchored current value.
- Anchored each Palworld slider's current-value readout above its thumb so the value moves with the control instead of occupying a disconnected title-row column.
- Replaced the advanced-settings entry/return flow with a stable basic/advanced tab switch, flattening the editor hierarchy and keeping search, filters, and categories inside the advanced view without a nested panel shell.
- Fixed DST mod status rendering so client-only mods no longer claim to await a server restart, server-managed mods without runtime inspection show the honest configured state, and pending restart remains reserved for actual synchronization flows.
- Removed noisy per-field reset actions from basic provider settings, where generated names and credentials are valid initial values, and reduced modified advanced-field resets to accessible icon actions.
- Simplified Palworld number sliders to a compact current-value readout, one full-width track, and range endpoints, removing the redundant number input and default-value caption while retaining keyboard precision.
- Replaced the inline advanced-settings expansion with an in-place basic/advanced view switch, explicit back navigation, and a directional entry affordance on both creation and server-detail forms.
- Fixed the shared provider boolean switch thumb positioning so enabled, disabled, and keyboard-focused states stay aligned inside the track on creation and server-detail forms.
- Unified creation and server-detail provider configuration with a progressive advanced editor: basic fields stay visible, while DST and Palworld rules use category navigation, global search, modified-only filtering, per-field reset, and compact review summaries.
- Added bounded Palworld number sliders with synchronized precise inputs, visible minimum/default/maximum values, and retained selects for non-continuous DST frequency settings.
- Replaced the partial DST world configuration list with a generated manifest of all 222 options exposed by dedicated-server build 740477, including official Chinese labels, valid values, defaults, forest/cave applicability, and grass gecko mutation controls.
- Fixed DST Workshop installation by publishing the generated setup file into the server installation's `mods` directory, persisting the UGC download directory, excluding known client-only mods from server registration, and rendering per-mod configuration options for both shards.
- Added a reproducible DST option generator plus provider and frontend tests for schema completeness, Chinese rendering, client/server mod classification, and `configuration_options` output.
- Shared provider option localization across creation, review, and server-detail forms so DST enum values render in Chinese while retaining their backend values.
- Fixed DST detail configuration initialization to deep-merge stored nested values with provider schema defaults, preventing empty world override groups from rendering as `never`.
- Added an accessible show/hide control for saved provider secrets so existing Klei server tokens and other password fields can be verified without changing their stored values; secrets remain masked by default and are hidden again while saving.

## 2026-07-18

- Added provider-scoped world recreation as a compact server overflow action with an on-demand confirmation dialog, without exposing the hidden global world library, adding a separate navigation tab, or occupying the configuration page with a persistent card.
- Added persistent asynchronous world recreation jobs with stop, required full backup, provider-declared save isolation, optional restart, health checking, and automatic rollback stages.
- Added the first provider implementation for Don't Starve Together, safely recreating the Master and enabled Caves shard saves while preserving cluster identity, token, ports, mods, and configuration.
- Added path traversal and symbolic-link protection for save isolation, startup recovery for interrupted jobs, maintenance-task mutation locking, bilingual progress and confirmation UI, and OpenAPI coverage.
- Verified full Go tests and vet, 80 frontend tests, frontend lint/typecheck, and the production frontend build.

## 2026-07-16

- Fixed the Palworld runtime logger FIFO ownership race by atomically publishing
  a private FIFO owned by the configured `PUID` and `PGID`, with a strict
  build-time upstream compatibility check and focused patch tests.
- Isolated Certbot behind an opt-in Compose profile so normal control-plane operations do not create a misleading exited container.
- Added a reusable systemd installer for persistent daily HTTPS renewal checks, using a root-owned runner/config with randomized scheduling and journald logs.
- Integrated automatic renewal setup into the HTTPS bootstrap flow and documented installation, status, manual checks, and logs in English and Chinese.

## 2026-07-15

- Added persistent asynchronous Palworld update checks and installs with explicit task status, stage, progress, build IDs, and failure details.
- Added pre-update save backups, player-online protection, atomic per-server mutation locking, optional restart-after-update, and safe restoration of a previously running server when failure occurs before SteamCMD modifies game files.
- Added global exclusion for heavy update/image tasks, 512 MiB check and 1536 MiB install helper limits, disk and managed-memory headroom checks, and a longer API shutdown grace period.
- Added restart-safe recovery: updater containers are labeled by task, canceled tasks remain active, interrupted installs are cleaned up and revalidated before any optional restart, and stale manifests plus `PalServer.sh` permissions are repaired on retry/failure paths.
- Added the server-detail update card for checking availability, confirming an install, and following background progress without blocking the page.
- Documented the game update state, check, and apply endpoints in the OpenAPI contract.
- Verified the OpenAPI YAML and production Compose merge, full Go tests and vet, race tests for the update-critical backend packages, 79 frontend tests, frontend lint/typecheck, and the production frontend build.

## 2026-07-14

- Refined the public-host settings form with compact, change-aware save and discard actions plus accessible success/error feedback.
- Added constrained Don't Starve Together world and cave settings based on the current official build 739495 option definitions.
- Grouped common world generation, seasons, resources, creatures, threats, cave environment, cave resources, and cave threats using progressive disclosure.
- Added Chinese and English labels, numeric limits, known override validation, and forest/cave Lua rendering tests.
- Verified with Go tests/vet and frontend tests/lint/typecheck/production build.
