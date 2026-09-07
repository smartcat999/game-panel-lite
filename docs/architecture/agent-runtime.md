# Agent runtime deployment

The Agent constructs one Docker adapter at startup. `DOCKER_HOST` defaults to `unix:///var/run/docker.sock`. `AGENT_INSTANCE_ROOT` defaults to `/var/lib/gamepanel/instances` and must resolve to a persistent directory accessible at the same absolute path to the Agent and Docker daemon. For a containerized Agent, mount that directory at the same path inside and outside the container. Remote daemon deployments must provision this shared path explicitly.

Assignments supply relative files and mount targets, not arbitrary host roots. Remote `Spec.DataDir` is ignored. The adapter applies CPU cores, memory MiB and explicit TCP/UDP port bindings on bridge networking. Existing workloads are checked for managed/server/node ownership before lifecycle operations.

Build independently with `CGO_ENABLED=0 GOOS=linux go build -o /tmp/gamepanel-agent ./apps/agent`. Docker build contexts must include the root `internal` directory; the repository Dockerfiles copy it.

Run `go test ./internal/... ./apps/agent` for local contract tests. To exercise an available Docker daemon, run `GAMEPANEL_TEST_DOCKER_HOST=unix:///var/run/docker.sock go test -count=1 -run TestDockerIntegration -v ./internal/runtime/docker`. This pulls Alpine 3.21, creates a uniquely named disposable container, verifies resource limits, port bindings, literal stdin and lifecycle convergence, then removes it.

This test does not establish fleet capacity, multi-process fencing, game compatibility or hostile-mod isolation. Historical writable file permissions remain for image compatibility; stronger OS isolation and volume ownership are separate SaaS requirements.
