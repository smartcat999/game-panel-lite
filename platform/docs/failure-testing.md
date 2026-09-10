# Failure testing matrix

Run these checks before a production release. Each scenario has a deterministic automated assertion and does not require cross-Region synchronous coordination.

| Failure | Automated check | Expected invariant |
| --- | --- | --- |
| Broker redelivery | `go test ./internal/messaging -run 'TestInboxRunsSuccessfulHandlerOnce|TestInboxRetriesFailedHandler'` | one committed inbox effect per message ID, with failed attempts retryable |
| Crash at delivery boundaries | PostgreSQL-backed `TestDurableDeliveryRecoversEveryStepAndComposesEndpointTruth` | scheduling, Endpoint allocation, assignment, readiness, and observation resume idempotently |
| Failed delivery cleanup | PostgreSQL-backed `TestFailedEndpointStepAutomaticallyCleansResidualCapacity` | capacity and Endpoint residuals are released after terminal failure |
| Node loss or stale authority | `TestAgentRejectsTaskAfterNodeOrFenceChanges` and fencing tests | an expired Node lease or stale fencing token cannot commit a mutation |
| Runtime not ready | `TestAgentKeepsAssignmentPendingUntilRuntimeIsReady`, `TestTCPListenerControlsRuntimeReadiness`, and `TestMissingWorkloadNetworkAddressIsNotReady` | an Operation cannot report success before the declared listener and ready marker are present |
| Uninstalled or forged Provider release | `TestAgentFailsClosedWhenVerifiedReleaseIsNotInstalled` and Provider registry tests | only signed, installed Provider Releases can materialize workloads |
| Unsafe game configuration or mod input | Terraria and tModLoader Provider tests | unsupported versions, unknown mods, traversal, NUL, and multiline injection fail closed |
| Malicious archive or mount | `TestBackupRestoreIsAtomicAndRejectsArchiveTraversal` and Docker scoped-mount tests | no path escape or symlink traversal; failed restore cannot replace current data |
| Runtime replacement and telemetry replay | `TestRuntimeReplacementPreservesInstanceHistoryAndInternetEgress` and instance-observability integration tests | logs remain keyed to Logical Instance and sequence never regresses across Runtime Attempts |
| Backup redelivery | PostgreSQL-backed `TestDurableBackupAndSameRegionRestoreFlow` | one terminal result, immutable digest, and same-Region restore authority |
| Schema replay | PostgreSQL-backed `TestMigrateFreshSchemaAndReplay` | ordered checksummed migrations create a fresh schema and replay without drift |

For database tests, set `GAMEPANEL_GLOBAL_TEST_DSN` and `GAMEPANEL_REGION_TEST_DSN` to disposable databases. Never point integration tests at a deployed Region or Global database. The Docker integration check requires an explicit `GAMEPANEL_DOCKER_INTEGRATION=1` and `GAMEPANEL_DOCKER_HOST`; it creates only namespaced disposable test workloads.
