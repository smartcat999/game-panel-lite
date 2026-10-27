# V1 Progress

## 2026-10-27

- Changed new Seven Days to Die instances to execute shared image programs without copying the full game into each data directory; saves, configuration, mods and small platform caches remain isolated per instance.
- Preserved existing local installations and explicit Steam updates. New image-backed instances select an updated local copy only after build verification publishes an atomic completion record; incomplete legacy installs fail without replacing user data.
- Added bilingual confirmation text explaining the writable program copy and 25 GiB reserve required by manual Steam updates. Normal startup checks a 2 GiB operating reserve, with world and mod growth additional.
- Verified 15 Python unit tests and five isolated Docker integration cases, full Go tests/vet, 127 frontend tests, application-source lint, typecheck and production build. Full frontend lint remains blocked by pre-existing files in apps/web/scripts.
- Tested real native startup on um773 with a read-only root filesystem: the test mod loaded, initial instance data was about 16 MiB, and replacing the container loaded the same saved world without copying game programs. Steam login timed out on the fixture network after world startup; public multiplayer connectivity was not verified.
- Changes are local; no production deployment or formal v0.2.19 release performed in this phase.

## 2026-08-30

- Centralized administrator, operator, and viewer permissions in one backend role matrix and returned effective capabilities with the authenticated account.
- Enforced read and mutation permissions on the backend for server operations, deletion, game assets, activity events, nodes, system maintenance, global settings, and tenant-administration scaffolding.
- Aligned navigation and server-detail surfaces with the same role model: viewers only see dashboards, server lobbies, status, and join information; operators retain game-server maintenance without deletion or system access.
- Added an explicit permission matrix to team settings, protected direct page navigation, stored non-admin language preferences locally, and hardened organization member role, identity, duplicate, owner-removal, and quota validation.
- Added backend authorization and frontend role-matrix tests; frontend lint, typecheck, focused tests, and production build pass.

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

## 2026-10-26 — 7 Days to Die vanilla support (issue #161)

- Added the vanilla `seven-days-to-die` Provider, catalog entry, official Steam
  cover, bilingual configuration and join instructions. Creation skips the mod
  step when the selected Provider has no mod support.
- Added explicit mixed-protocol Provider ports and adjacent host-port
  reservations, including collision and overflow checks on creation/configuration.
- Added a prebuilt amd64 runtime with persistent game files and `userdata` saves.
  Managed XML is merged into the installed game's shipped defaults. Ordinary
  restarts do not run SteamCMD. The runtime base initializes SteamCMD before
  installing its 64-bit Steam client library.
- Used the 3.x `SandboxCode` mechanism instead of obsolete difficulty, blood moon
  and loot XML properties. Users copy a code from the matching game version.
- Kept existing world identity read-only to prevent accidentally selecting a new
  save. Manual backups require a stopped server and contain only `userdata`.
  Restore extracts first, then replaces that subtree, preserving runtime files
  and removing world regions created after backup. Scheduled live backups are
  disabled for this Provider until a save-command path is available.
- Generalized Steam update metadata, install paths, executable permission repair,
  save backups and disk preflight through Provider declarations. Palworld retains
  its existing update behavior; seven-days-to-die requires 25 GiB free disk.
- Verification: related Go Provider/server/runtime/HTTP/backup tests, full Go vet,
  121 frontend tests, TypeScript and application-directory ESLint checks passed.
  Python config-merge and shell syntax checks and the final production frontend
  build passed. The amd64 runtime base built successfully on um773.
- Browser verification used an isolated local API/database and mocked only the
  image-ready response. Chinese/English creation, configuration/review and form
  submission succeeded. This does not verify actual game startup or player joins.
- Full `go test -mod=mod ./...` encountered pre-existing invalid-Docker-host
  state-transition timeouts in `internal/app`; the same failure was reproduced
  from an unmodified `main` snapshot. Full frontend lint also includes unrelated,
  pre-existing untracked screenshot scripts; application directories pass.
- Runtime verification: after correcting the host's persistent Steam routing,
  a complete amd64 image built on um773 (Steam Build ID `25661908`, game
  `V 3.3.0 b18`). The local Buildx client could not reach Docker Hub's token
  endpoint, so the build ran from um773's own client.
- Actual startup exposed the obsolete `SaveGameFolder` property, which 3.3.0
  rejects. Removed it and added a regression assertion; saves use
  `UserDataFolder/Saves`. Related Go tests and Provider vet passed again.
- An isolated, loopback-only 2-CPU/6-GiB instance reached `StartGame done` and
  EOS registration, persisted its save under `userdata/Saves`, and saved/exited
  with code 0 on SIGINT. A replacement container reused that volume and loaded
  the existing save directory without downloading or reseeding game files.
- EOS connectivity remains intermittent: some starts timed out before world
  loading; retrying succeeded. A Steam game-server logon timeout also occurred.
  Client joining, player progress across restart, actual game updates and
  dependable external-service connectivity still require verification before
  release. Both isolated test instances were stopped after verification.

### 2026-10-27: Runtime image uninstall (local implementation)

- Added administrator-only uninstall actions to Game Library and Runtime Images,
  with shared confirmation UI and Chinese/English messages.
- Uninstalls the provider runtime and its historical images, installation archives,
  temporary archives and markers without force. Does not remove instance data,
  volumes, mods or backups. Installs and new servers offer the latest runtime only.
- Running, starting and unconfirmed servers block uninstall. Confirmed stopped
  local instances may have their old containers removed without force or volume
  deletion. All dependencies and persistent mounts are checked before any removal;
  unknown storage or foreign containers block cleanup. Instance records are kept.
  Installation/game-update locks prevent concurrent maintenance.
- Corrected missing-container state translation so the reconciler recreates the
  saved workload after reinstall. Restoring a stopped instance switches its runtime
  reference to the latest installed image while keeping its data directory, world,
  configuration and mods. No historical-runtime selector is offered.
- Providers with several game-file versions sharing one runtime use one canonical
  latest image archive. Old archives are cleaned after installation where safe;
  running dependencies are retained rather than forcibly stopped.
- Regression coverage verifies saved world and backup bytes remain intact, failed
  removal preserves installation files, unknown providers cannot escape cache
  directories, running shared-image dependencies are protected, and stopped instances
  retain their world, configuration, mods, install sentinel and database records.
- An opt-in real-Docker regression passed on um773 with a tiny isolated fixture:
  stop, remove old container/image, verify original files, install a new latest
  image, recreate using the same mount and resume the existing day-49 save.
- Full Go tests/vet, 122 frontend tests, typecheck, application-source lint and
  production build passed. Full frontend lint remains blocked by pre-existing
  untracked scripts under apps/web/scripts; these were not changed.
- Local Playwright API-fixture checks passed for confirmation, referencing-server
  errors, successful uninstall state refresh and the English Runtime Images entry.
  The latest-only flow also passed: provider-level uninstall has no version
  selector, and reinstall requests the recommended latest runtime version.
- Not deployed or released. v0.2.19 remains a draft pending user validation.


### 2026-10-27: Measured runtime installation progress (local implementation)

- Replaced weighted overall installation percentages with explicit resolving,
  downloading, extracting, saving and finalizing phases. Download and extraction
  counters stay independent when late layers arrive or cached layers are reused.
- Docker pull events do not establish a reliable fixed whole-image size. These
  events now report measured bytes and sampled download speed without a guessed
  percentage; supported fixed totals can drive a current-phase percentage.
- Archive saving reports actual successful bytes written rather than jumping to
  95%. Installation is ready only after the archive and marker are saved. Errors
  retain the last measured phase; isolated layer completion cannot finalize a pull.
- A shared compact progress component serves Game Library and Runtime Images.
  Removed repeated preparing badges, disabled installing buttons and duplicate
  single-provider descriptions. Stage, relevant counters and a thin progress bar
  occupy one area; stale transfer speeds are hidden. Chinese/English supported.
- Regression coverage includes replayed mixed pull events, late/cached layers,
  measured speed, archive writes/cancellation, saving-phase success/failure and
  omission of legacy guessed percentages. Full Go tests/vet, 126 frontend tests,
  typecheck, application-source lint and production build passed. Follow-up Go
  tests cover delaying finalization until the pull stream finishes.
- Local Playwright API-fixture checks passed for all five phases, one status label,
  no guessed percentage, desktop Runtime Images and Chinese/English 390px layouts.
  No production image download, deployment or release was performed for this change.


### 2026-10-27: Install Docker runtimes without mandatory offline archives (local implementation)

- Normal installation now reuses or pulls a Docker image, verifies it exists, and
  atomically writes a Docker-backed installation record. It never exports a tar
  archive or waits for a duplicate copy. Only the active job's exact abandoned
  `.tar.tmp` file is removed; existing offline archives and instance data remain.
- Library availability requires both valid registration and an actual Docker
  image. Missing or empty optional archives do not hide an installed image; an
  archive alone does not claim the Docker image is installed.
- Legacy offline archives remain usable for installation and startup recovery.
  If no usable archive exists, registered images can be pulled again and verified.
  Removed registrations do not auto-reinstall. Shared-image game versions can use
  the latest matching registration. Old runtime caches remain until the replacement
  image is available, preserving recovery options if registry access fails.
- Disk-full errors map to one Chinese/English message without host paths. Game
  Library and Runtime Images show a single error, without repeated install hints.
- Added a seven-days-to-die entrypoint storage check on the mounted instance
  filesystem. Initial preparation estimates program copy bytes plus 2 GiB reserve;
  already prepared instances only require the operating reserve. It exits before
  copying when space is insufficient and leaves existing files intact. This check
  requires rebuilding the game runtime image; it is not active in existing images.
- Validation: full Go tests/vet, 127 frontend tests, typecheck, application-source
  lint, production build and four Python storage-check tests passed. API fixtures
  verified one localized error on both library pages and ready-state actions without
  an archive. An isolated real Docker test on um773 with 1 MiB tmpfs stopped before
  any game-file copying; current seed estimated 19,848,638,198 bytes including reserve.
- Scope: first installation/recovery phase plus startup space protection. Reusing
  read-only game binaries, daemon-filesystem pull capacity estimation and a manual
  offline-export UI remain separate follow-up work. No deployment or release was
  performed, and v0.2.19 remains draft.
