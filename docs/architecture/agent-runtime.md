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

Control-plane generation of complete remote manifests and remote installation eligibility remain pending. Receiving a hand-authored persisted manifest in tests does not prove a complete remote installation workflow.
