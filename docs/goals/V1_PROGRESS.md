# V1 Progress

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
