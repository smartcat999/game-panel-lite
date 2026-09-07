# Agent runtime deployment

The Agent constructs one Docker adapter at startup. `DOCKER_HOST` defaults to `unix:///var/run/docker.sock`. `AGENT_INSTANCE_ROOT` defaults to `/var/lib/gamepanel/instances` and must resolve to a persistent directory accessible at the same absolute path to the Agent and Docker daemon. For a containerized Agent, mount that directory at the same path inside and outside the container. Remote daemon deployments must provision this shared path explicitly.

Assignments supply relative files and mount targets, not arbitrary host roots. Remote `Spec.DataDir` is ignored. The adapter applies CPU cores, memory MiB and explicit TCP/UDP port bindings on bridge networking. Existing workloads are checked for managed/server/node ownership before lifecycle operations.

Build independently with `CGO_ENABLED=0 GOOS=linux go build -o /tmp/gamepanel-agent ./apps/agent`. Docker build contexts must include the root `internal` directory; the repository Dockerfiles copy it.

Run `go test ./internal/... ./apps/agent` for local contract tests. To exercise an available Docker daemon, run `GAMEPANEL_TEST_DOCKER_HOST=unix:///var/run/docker.sock go test -count=1 -run TestDockerIntegration -v ./internal/runtime/docker`. This pulls Alpine 3.21, creates a uniquely named disposable container, verifies resource limits, port bindings, literal stdin and lifecycle convergence, then removes it.

This test does not establish fleet capacity, multi-process fencing, game compatibility or hostile-mod isolation. Historical writable file permissions remain for image compatibility; stronger OS isolation and volume ownership are separate SaaS requirements.

## Artifact delivery

The production Agent now initializes an authenticated HTTP artifact source and advertises `artifacts-v1` after runtime initialization succeeds. `MASTER_URL` (or `PANEL_URL`) must be an HTTP(S) URL without embedded credentials, query or fragment. Use HTTPS in production; HTTP remains supported for private/local deployments. The node token comes from the existing Agent token configuration. Downloads use a separate HTTP client, never follow redirects, and accept only status 200, the declared Content-Length, the matching SHA-256 ETag and identity encoding. The runtime independently hashes all downloaded bytes before replacing an old container.

Deployment limits (positive decimal integers; invalid values fail startup):

| Environment variable | Default | Scope |
| --- | --- | --- |
| `AGENT_ARTIFACT_MAX_FILES` | 128 | Files per assignment |
| `AGENT_ARTIFACT_MAX_FILE_BYTES` | 268435456 | Bytes per file (256 MiB) |
| `AGENT_ARTIFACT_MAX_TOTAL_BYTES` | 1073741824 | Total bytes per assignment (1 GiB) |

The existing 90-second reconciliation deadline bounds downloads and runtime operations together. A slow transfer fails that attempt and leaves the old container intact if preparation has not completed; a later reconciliation retries. Heartbeats run independently. These are transfer/preparation limits, not persistent storage quotas. Temporary preparation and final files can coexist, so provision disk headroom beyond the transfer total. Crash orphan cleanup, resumable transfers and fleet bandwidth limiting are not implemented.

The control-plane workload builder now resolves selected uploads and transitive dependencies from the same workspace and provider, emitting deterministic artifact references and optional provider manifests without creating local instance files or installed records. Missing or ambiguous dependencies, unsupported sources and invalid descriptors reject planning. Stop/delete do not need mod sources. Manifest upgrade behavior, full game-package compatibility and cross-host failure recovery remain pending.

Artifact descriptors also pin the library metadata `revision` (omission means zero for legacy descriptors). Publication locks the workspace before validating each uploaded source's revision, size and digest, sharing the lock used by library writers. Persisted manifests prevent in-place updates and deletion of referenced sources, including transitive dependencies; upload a new source ID for a replacement. A new desired generation alone does not release the old manifest. Replacing/removing the persisted manifest releases that reference, but does not prove an old worker has stopped: execution leases and lost-node fencing remain separate work. Reference checks use the composite workspace/artifact index in `workload_artifact_references`. Publication replaces references in the same transaction as the manifest, and assignment deletion removes them atomically. PostgreSQL migration 006 and a one-time SQLite migration backfill existing manifests from source ownership, including manifests whose instance is no longer present. Repeated artifact IDs are deduplicated. Invalid historical JSON fails migration rather than silently dropping protection. PostgreSQL deployments must run migrations before starting the new API binary. Direct instance, preset and pack references still use their existing checks; their normalization remains separate work.

For this schema transition, stop old control-plane writers before migration and resume with the matching new binary. Old binaries do not maintain the reference index; mixing old/new writers during a rolling upgrade would make it incomplete. Zero-downtime compatibility across this transition has not been implemented.

## Two-Agent delivery integration

Run `GAMEPANEL_TEST_DOCKER_HOST=unix:///var/run/docker.sock go test ./apps/api/internal/http -run TestAgentArtifactDeliveryIntegration -count=1 -timeout 4m` to build and start two actual Agent processes against the real registration, assignment, observation and artifact-download handlers. The test uses separate workspaces/data roots, the production upload planner and publisher, and uniquely named Alpine containers that read the provider-defined artifact path. It verifies each container's bytes and ownership, rejects cross-node artifact downloads, then stops the Agents and removes the containers.

Both workers use one Docker daemon: this verifies process separation and protocol wiring, not cross-host networking, game compatibility, fault recovery or fleet capacity. The probe provider preserves Terraria upload/manifest behavior while replacing the game executable with a small file reader. It requires Go on PATH and a daemon that can mount the test's temporary directories.

The integration uncovered a tight tunnel-poll retry loop on immediate empty/error responses. The Agent now drains small response bodies, closes them and waits two seconds when no stream was dispatched; cancellation interrupts the wait. Unit/race tests cover 200 without a stream, 204, 401, 404 and 503. Successful stream dispatch remains immediate; stream concurrency limits and adaptive fleet retry jitter are separate work.

## Customer remote installation requests

Agent registration and heartbeats now include `workloadCapabilities`, derived from the successfully initialized runtime. The API persists recognized capabilities; omitted capabilities from older or downgraded Agents clear previous support. PostgreSQL migration 007 adds this field (SQLite initialization adds it automatically).

Workspace writers can save uploaded-mod installation requests for stopped remote instances when the node is online, its heartbeat is at most 45 seconds old, and it advertises `artifacts-v1`. The write transaction locks and rechecks the node in addition to workspace membership, source revision and instance generation/status. The UI permits remote targets and explains the submission-time check. A 202 response still means desired configuration was saved; the instance remains stopped and files are prepared on its next start. Missing dependencies, incompatible game packages, later node failure and execution leases remain distinct from request acceptance.
