# V1 Progress

## 2026-09-07

- Public registration now atomically creates an account, personal organization, owner membership and existing starter quota. Failure during quota persistence rolls all four back. Session issuance remains after commit; a session failure can be retried through login.
- Added authenticated user-scoped organization list/detail endpoints and OpenAPI contracts. Membership is joined in the SQL query, client user IDs cannot select the principal, platform roles do not bypass membership on these endpoints, and removed memberships lose access on the next request. Existing platform organization administration remains separately guarded.
- SQLite and real PostgreSQL tests cover separate users, membership revocation and transaction rollback; HTTP tests cover registration cookies, cross-user IDs, forged principal input, empty lists, pre-setup denial and platform-role separation. This does not yet isolate server/files/SSE or migrate existing users. The starter quota is the existing product default, not paid entitlement enforcement. Validation passed: gofmt, full Go tests, vet, real PostgreSQL integration under race, targeted HTTP race, tracked-source frontend lint, typecheck and production build. OpenAPI YAML parsed successfully. The temporary PostgreSQL container was removed. Root lint still includes unrelated untracked recording scripts, so tracked sources were checked; no remote CI or capacity test ran.

- Separated PostgreSQL migration execution from API startup. Added gamepanel-migrate with cancellation/deadline support and included it in the API Docker image. API connections now only read/verify migration versions and checksums; they never initialize a PostgreSQL schema.
- Real PostgreSQL tests verify a missing schema stays empty on API startup failure, concurrent explicit migrations remain safe, and a dedicated runtime role can start/read/write application data without CREATE or migration-ledger write privileges. Temporary test roles/schemas are cleaned up.
- Production role provisioning, tenant RLS, SQLite data migration and rolling deployment compatibility remain pending. Startup now requires a prior migration job; see architecture/postgresql.md. Validation passed: gofmt, full Go tests, go vet, live PostgreSQL role/migration integration under race, Linux migration binary build and repeat execution of the CLI on a disposable database. CI YAML parsed successfully. No frontend code changed; remote CI was not run.

- Replaced PostgreSQL AutoMigrate with a frozen, embedded SQL baseline and checksum/version ledger. Startup serializes migrations per schema using a transaction advisory lock; DDL and ledger changes commit atomically. SQLite startup remains unchanged.
- Real PostgreSQL integration now exercises four concurrent initializers, idempotent reopen, failed DDL rollback/retry, tampered checksum rejection and refusal of a database newer than the binary. Populated unversioned schemas require explicit adoption and are not automatically modified.
- Existing experimental PostgreSQL databases need an adoption/data-transfer procedure; it is not implemented yet. Dedicated migration jobs, SQLite migration, runtime least-privilege roles and deployment downgrade remain pending. No destructive down script was added. Validation passed: gofmt, full Go tests, vet and real PostgreSQL integration under race, including unversioned-schema rejection. Temporary baseline generator removed and disposable PostgreSQL container deleted. No frontend source or API contract changed.

- Added optional PostgreSQL persistence selected by GAMEPANEL_DATABASE_URL, retaining default SQLite. Added a bounded per-process pool, redacted connection errors and database cleanup on failed initialization/application close. Isolated SQLite rowid ordering behind the persistence dialect; PostgreSQL uses deterministic ID ties.
- Added the official GORM PostgreSQL driver and synchronized vendored dependencies. A real disposable PostgreSQL 16 instance passed schema initialization, JSON/config-version round trip, transaction rollback, activity/job ordering and not-found tests in a unique schema that was removed afterward. Added equivalent CI service/test coverage; remote CI has not run.
- PostgreSQL schema initialization still uses AutoMigrate. Explicit migrations, RLS, SQLite data transfer and multi-replica coordination remain unfinished. See architecture/postgresql.md for deployment/test configuration and limits. Validation passed: gofmt, full Go tests, vet, Store/App race tests, real PostgreSQL integration, frontend typecheck/build, tracked-source lint and CI YAML parsing. Disposable database container removed. Existing unrelated worktree-wide lint failures remain unchanged.

- Kept backup originals until the caller finishes restored configuration parsing/persistence. RestoreHooks separates metadata validation from the final commit, and both HTTP restore paths use the commit hook. Returned commit failures roll back published files and retain the existing runtime error status mapping.
- Added an integration regression through the real configuration parser and a rejecting store: in-memory spec stays unchanged, original config files return, newly restored files disappear and staging is cleaned up. Successful commit sees the published files and runs once.
- This handles reported errors while the process is alive. Ambiguous database commit outcomes, process/power loss and distributed recovery still require durable coordination; it is not a cross-resource ACID transaction. Validation passed: gofmt, full Go tests, vet and backup/gameconfig/HTTP race regression. No frontend source or API schema changed.

- Changed ZIP restore from streaming overwrite to complete staging before publication. All entry reads/CRC checks finish before replacing existing files; duplicate normalized paths and symlink entries are rejected. Publication retains originals and rolls back modified/new files on returned errors.
- Added tests for a corrupted later ZIP entry preserving original files, rollback after a later target-directory conflict, original permission restoration and staging cleanup. Failed rollback retains recovery files and reports their relative location.
- This is file rollback within a live process, not a durable crash-recovery transaction. Newly created empty directories may remain after failure; later configuration/database errors are not rolled back by this file operation. Disk capacity policy and startup recovery still need implementation. Validation passed: gofmt, full Go tests, vet and backup/HTTP race regression. No frontend code or wire contract changed.

- New production backups embed .gamepanel-backup.json with archive format, source game/provider and configuration versions. All manual/snapshot/pre-update/pre-regeneration creators pass metadata. Restore checks bounded metadata before destination creation, rejects malformed/duplicate/unsupported metadata and skips the metadata entry during extraction.
- Added root-confined restore writes to prevent existing target symlinks escaping the game directory. Tests cover metadata round trips, callback rejection before target creation, malformed/duplicate/oversized metadata, no metadata extraction and symlink escape. HTTP verifies embedded compatibility even when the database record appears compatible.
- Legacy ZIPs without metadata retain format-1 semantics. Archive metadata is not authentication; full extraction rollback, standalone import workflows and signed integrity remain pending. Validation passed: gofmt, full Go tests, vet, and backup/gameconfig/HTTP race regression including the embedded-version endpoint test. No frontend code or response schema changed.

- Persisted source game/provider and configuration format on new manual, save snapshot, pre-update and pre-regeneration backup records. Existing zero/unrecorded metadata retains legacy format-1 compatibility; list hydration no longer overwrites recorded source identity with current instance identity.
- Both restore endpoints check recorded source provider/configuration compatibility before archive access. Added create/persistence, immutable hydration and pre-extraction rejection tests. OpenAPI now describes backup source fields.
- Source metadata currently lives in the database, not inside portable ZIP files. Standalone import verification, explicit database migrations and archive/source integrity remain pending; existing SQLite schema setup uses AutoMigrate. Validation passed: gofmt, full Go tests, vet, gameconfig/store/HTTP race regression and OpenAPI YAML parsing. No frontend source changed.

- Extended configuration version checks to generic config editing, versioned preview, and gameconfig restoration. Both backup restore endpoints check the target configuration before archive extraction or orphan-record pruning.
- Added regressions proving incompatible edits/restores preserve the stored server spec and backup record, and incompatible direct restore does not persist/mutate caller state. Preview accepts optional configVersion (legacy zero remains format 1), documented in OpenAPI.
- Source backup format/version metadata, other config mutation paths and explicit migrations are still pending. Target-instance preflight does not establish backup-source compatibility. Validation passed: gofmt, full Go tests, vet, gameconfig/HTTP race tests and OpenAPI YAML parsing. No frontend source changed.

- Introduced independent plugin release and configuration format declarations for every registered provider. Registry rejects malformed numeric major.minor.patch release identifiers and nonpositive configuration versions. Current providers declare release 1.0.0 / configuration format 1 as the initial contract baseline.
- New instances persist spec.configVersion; omitted/zero legacy values mean format 1, not the newest format. Workload construction rejects incompatible formats before directory creation/mod planning. Added OpenAPI documentation, registry compatibility tests, persistence/create assertions and a no-filesystem-mutation regression.
- Configuration editing/restoration guards, explicit migrations and compatibility across actual future provider releases are not yet implemented. Plugin version metadata does not imply hot loading or a versioned RPC protocol. Validation passed: gofmt, full Go tests/vet, provider/server/store/HTTP race tests, frontend typecheck/build, tracked-source lint and OpenAPI YAML parsing. Existing worktree-wide lint failures in unrelated local recording scripts remain unchanged.

- Moved tMod binary metadata parsing and its tests out of the generic mod cache package into Terraria Provider. Added optional ModInspector and generic ModMetadata with loader version; HTTP delegates inspection through modruntime with request cancellation and no concrete format parser import.
- Removed provider-specific conditions around applying returned metadata. Existing database/wire fields remain compatible. Added malformed/truncated/oversized-string/UTF-8 header tests and a synthetic inspector test covering registration, missing files, unknown providers, optional capability and cancellation.
- Metadata inspection remains best-effort at current upload endpoints; this does not validate complete archives or reject all unsafe packages. Strict upload validation and installation transactions remain pending. Validation passed: gofmt, full Go tests, vet and Terraria/modruntime/HTTP/server race tests. No frontend code or wire schema changed.

- Removed provider switches from mod cache filename validation and per-instance upload extension checks. Providers declare accepted upload extensions and exact auxiliary cache names; modruntime exposes separate user-upload and internal-cache validation, injected into the cache service.
- Added a synthetic .addon provider regression proving upload/cache extensibility without handler or storage changes. Auxiliary manifests and unknown providers are rejected at the user-upload boundary. Legacy Terraria global routes remain compatible.
- Fixed ignored cache path errors in single/batch deletion: normalize legacy record metadata, validate the path and retain the database record when validation or removal fails. Binary mod parsing and complete installation transactions remain pending. Validation passed: gofmt, full go test ./..., go vet ./..., and final mod/modruntime/HTTP/server race tests. No frontend code or response schema changed.

- Centralized mod runtime file installation/removal in modruntime. HTTP and startup planning now pass context and source readers to one implementation; deleted the duplicate server file copier. Provider paths are validated before staging rather than silently discarded.
- Confined destination filesystem operations with os.Root, staged all copies before publishing and used per-file atomic rename. Tests cover symlink/traversal escapes, unchanged existing files on read failure/cancellation, staging cleanup, repeated replacement/removal and runtime-compatible permissions. Multi-file publication and database changes remain non-transactional; this is not strong tenant isolation. Validation passed: gofmt, full Go tests, vet, and final modruntime/HTTP/server race regression after retaining runtime permissions. No frontend code or wire contract changed.

- Consolidated duplicate HTTP/lifecycle mod identity and dependency metadata rules into modcatalog. Consolidated dependency graph traversal into modruntime, with sequential resolution, cycle/diamond deduplication and context cancellation. Existing persistence operations remain caller-owned; errors do not imply rollback of prior assignments.
- Added shared-interface tests for metadata precedence, provider-scoped fallback, independent result slices, cyclic/diamond graphs, existing dependencies, failure and cancellation. Import checks prevent these shared modules from reaching back into HTTP, server or concrete store/runtime packages. Remaining upload/metadata write/install transaction work remains in the SaaS checklist. Validation passed: gofmt, full go test ./..., go vet ./..., modcatalog/modruntime/HTTP/server race tests and expanded architecture tests. No frontend code or wire contract changed.

- Corrected mod dependency resolution in HTTP and lifecycle planning: match the target provider as well as the mod name, and resolve catalog dependencies within that provider. Unknown providers no longer silently use tModLoader recommendations.
- Reject direct cross-provider mod assignment before filesystem/database mutation. Existing pre-provider records retain the established legacy hydration behavior. Regression tests cover same-name collisions in instance/library records, cross-provider assignment rejection and catalog fallback rejection. This fixes a concrete migration prerequisite; complete mod application-module consolidation remains pending. Validation passed: gofmt, go test ./..., go vet ./..., and race tests for modcatalog/server/http. No frontend code or response shape changed in this batch.

- Extracted shared workload wire types and runtime-independent worker reconciliation. Agent Docker SDK, file preparation, logs and console now live in a dedicated adapter; all recorded import exceptions are removed. CPU/memory and TCP/UDP mappings now survive the shared assignment protocol and reach Docker.
- Added cancellation and goroutine ownership to Agent loops, managed workload ownership checks, and local-root file mounts. Removed legacy lifecycle execution and the tunnel game-port fallback. Distributed fencing and stronger tenant isolation remain pending.
- Validated full Go tests/vet, shared packages and Agent race tests, frontend typecheck/build, tracked-source lint and standalone Linux Agent build. Worktree-wide lint still has previously recorded unrelated local-script failures. A real disposable Alpine workload passed create/reconcile/resource/port/stdin/stop/delete checks. Added formatting, Linux Agent build and opt-in Docker integration to CI; remote CI has not run. Formatting the existing DST provider was necessary for the new formatting gate.

- Continued M2: moved legacy configuration preview/presets and restored-config parsing behind provider capabilities and a gameconfig application module. Restore reads are confined with os.Root, bounded to 1 MiB, and update caller state only after persistence succeeds.
- Moved world-file candidates, mod file layouts and tModLoader enabled/workshop manifests into providers. HTTP and lifecycle callers share modruntime for manifests; mod source support is declared by providers, and Registry rejects selected capability/implementation mismatches.
- Removed all seven direct concrete-provider import exceptions from HTTP/server code; two Agent Docker imports remain. This does not finish M2: upload parsing, mod metadata/dependencies, world storage and further HTTP orchestration still require migration. Track the full remaining scope in SAAS_REFACTOR_PROGRESS.md.
- M2 batch checks pass: go test ./..., go vet ./..., race tests for gameconfig/modruntime/providers/server, frontend typecheck/build and tracked-source lint. The worktree-wide lint issue from unrelated local scripts remains; no live Docker, payment or scale claims are made.

- Implemented the first backend modularity batch for the SaaS roadmap: game catalog metadata and recommendation priorities now come from registered providers, and duplicate provider IDs fail application startup instead of overwriting an implementation.
- Removed the legacy ConfigText side channel from provider/runtime contracts. Terraria renders serverconfig.txt into the same file collection as other provider assets; generic workload conversion and Docker creation no longer identify or create that filename specially. Unregistered games no longer appear as hardcoded planned catalog entries.
- Added AST import-boundary tests for domain, concrete providers, Docker SDK and GORM access, with nine exact legacy import exceptions across eight files assigned to M2/M3. Added a backend GitHub Actions workflow for Go tests, vet and focused race checks; remote CI has not run yet.
- Added regression coverage for duplicate registration, plugin-owned catalog metadata/order, a new game flowing through the workload builder, file round trips, nested/empty files and lexical path traversal rejection.
- Validation: changed Go files formatted; go test ./..., go vet ./..., focused provider/runtime race tests, pnpm typecheck and pnpm build pass. Full pnpm lint reports 123 pre-existing errors in untracked recording scripts and ignored tmp scripts; tracked JS/TS sources are checked separately without modifying those user files. No live Docker game deployment or scale test was performed.
- Next: migrate game-specific mod/world integrations behind provider capabilities and application use cases, then address the Agent runtime import exceptions alongside the shared execution protocol.

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
