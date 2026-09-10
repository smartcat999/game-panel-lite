# Failure testing matrix

Run these checks before a production release. Each scenario has a deterministic automated assertion and does not require cross-Region synchronous coordination.

| Failure | Automated check | Expected invariant |
| --- | --- | --- |
| Broker redelivery | `go test ./internal/messaging -run 'TestInboxRunsSuccessfulHandlerOnce|TestInboxRetriesFailedHandler'` | one committed inbox effect per message ID, with failed attempts retryable |
| Global database restart | PostgreSQL-backed `TestPostgresCheckoutAndActivationTransactions` | idempotent command and payment results survive a new module instance |
| Region database or controller restart | PostgreSQL-backed `TestPostgresAssignmentRestartAndBackupCompletionAreDurable` | unexpired claims are retained and expired claims resume once |
| Region isolation from Global | `TestRegionReconcilesExistingDeploymentWithoutGlobalPlane` | existing regional desired state continues reconciling |
| Node loss | `TestStaleNodeOrFencingTokenCannotCompleteAssignment` | an expired Node lease cannot commit a workload result |
| Duplicate payment notification | `TestVerifiedPaymentActivationIsIdempotentAndTransactional` | one Payment, Entitlement, and authority event set |
| Stale deployment observation | `TestDuplicateAndOutOfOrderEventsCannotRegressDeployment` and projection tests | regional and global sequences never regress |
| Backup redelivery | Node executor and Region PostgreSQL durability tests | one terminal result and stable object key |
| Malicious archive or mount | scoped-root and Docker adapter security tests | no path escape or symlink traversal |

For database tests, set `GAMEPANEL_GLOBAL_TEST_DSN` and `GAMEPANEL_REGION_TEST_DSN` to disposable databases. For the read-only Docker daemon compatibility check, set `GAMEPANEL_DOCKER_INTEGRATION=1` and an explicit `GAMEPANEL_DOCKER_HOST`; it only pings the daemon and does not create a container.
