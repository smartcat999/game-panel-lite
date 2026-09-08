# PostgreSQL persistence foundation

Set `GAMEPANEL_DATABASE_URL` to a PostgreSQL URL/DSN supplied by deployment secrets. When unset, `GAMEPANEL_DB_PATH` continues to select SQLite. `GAMEPANEL_DB_MAX_CONNECTIONS` defaults to 20 per API process. The PostgreSQL pool caps open/idle connections at this number and rotates connections after 30 minutes; size the total across replicas against database capacity. Invalid/nonpositive environment values currently fall back to 20.

Use TLS settings appropriate to the deployment. No production credentials belong in repository files. The test environment uses an ephemeral local database; its trust authentication/disabled TLS must not be copied into a public deployment.

The adapter follows the [official GORM PostgreSQL and connection pool documentation](https://gorm.io/docs/connecting_to_the_database.html). Driver parsing/connection errors are redacted to avoid disclosing DSNs. PostgreSQL SQL logging is disabled in this baseline adapter. Application shutdown closes the database pool.

Run `GAMEPANEL_TEST_POSTGRES_URL=<test database URL> go test -count=1 -run TestPostgresIntegration -v ./apps/api/internal/store`. The test creates a uniquely named schema, exercises JSON round trips, transaction rollback and chronological queries, then drops only that schema. The integration-test account requires schema and role creation rights; it creates a unique temporary runtime role and removes its own role/schema afterward. The CI PostgreSQL service is ephemeral.

This is not yet a production SaaS database rollout. PostgreSQL now uses immutable embedded SQL migrations and a checksum ledger. SQLite still uses existing GORM AutoMigrate. SQLite data transfer, tenant ownership/RLS, application replica concurrency and backup/recovery validation are unfinished. Do not start multiple production replicas assuming this work supplies those guarantees. SQLite's historical rowid tie break is retained; PostgreSQL orders equal creation timestamps by record ID, which is deterministic but does not imply insertion order.

## PostgreSQL migration protocol

The initial schema is fixed in `apps/api/internal/store/migrations/001_postgres_baseline.sql`, captured from the previous PostgreSQL Store schema and verified by real persistence tests. Future schema changes must append numbered scripts; editing an applied script causes startup to reject the checksum mismatch. An older binary refuses a ledger containing versions it does not know.

The dedicated migration command acquires a schema-scoped PostgreSQL transaction advisory lock, applies pending DDL and ledger rows in the same transaction, then commits. The command defaults to a one-minute deadline and accepts `-timeout`. The lock follows [PostgreSQL's transaction-level advisory lock behavior](https://www.postgresql.org/docs/current/explicit-locking.html#ADVISORY-LOCKS). Four concurrent initializers and intentionally failing DDL have been exercised against PostgreSQL 16. This serializes migrations only; it does not serialize application controllers.

Use an empty application schema for first initialization. A populated schema without the migration ledger is deliberately rejected. Databases created by the earlier AutoMigrate-based PostgreSQL experiment require an explicit, separately validated adoption/data-transfer procedure that is not yet implemented. No tables are automatically dropped or adopted. The current baseline has no destructive down migration; reported migration errors roll back their transaction, while deployment downgrade/data restoration requires a separate plan. A standalone migration command is now included in the API image, and a runtime role without DDL/ledger-write rights has been tested; production role provisioning and deployment orchestration remain environment-specific work.

## Deploy migration separately from the API

Run `go run ./apps/api/cmd/migrate -timeout 1m` with `GAMEPANEL_DATABASE_URL` set to the migration credential. The API image also provides `gamepanel-migrate`; execute it as a one-off deployment job before starting the new API version. The migration command has no SQLite fallback and fails when the PostgreSQL DSN is missing.

Start the API with a separate DSN granting schema usage, SELECT on `gamepanel_schema_migrations`, and required SELECT/INSERT/UPDATE/DELETE rights on application tables. Do not grant runtime CREATE on the schema or writes on the migration ledger. The API now checks version/checksums with SELECTs only and fails on a missing, older, newer or mismatched ledger. Grant permissions on newly added tables as part of future migrations/provisioning.

This verifies separation of DDL privileges, not tenant isolation. RLS, per-tenant transaction context, quota concurrency, migration/rolling-deployment compatibility and production role management still require implementation and validation.

## World ownership migration

Migration 002 adds persistent world organization ownership and copies the current source instance organization for historical attached worlds. Unassigned/unowned legacy records remain platform-only until explicitly adopted. New private unassigned uploads use `tenant-worlds/<organization>/unassigned`; existing assigned and legacy file paths are unchanged. SQLite initialization performs the same backfill for blank ownership, and never replaces an existing owner. Back up before upgrading; this does not implement old-binary rolling compatibility or a downgrade.

Moving an instance between organizations still requires a separate ownership/file-transfer procedure; changing its organization field alone is not a supported tenant migration.

## Activity ownership migration

Migration 003 persists activity organization ownership, backfilling historical events whose source instance still exists. Unowned platform events and events whose source was already deleted require explicit adoption if they should become tenant-visible; no owner is guessed from a message or payload. New instance events capture the source organization, and private world-library import/delete events explicitly carry world ownership. SQLite startup similarly backfills only blank ownership. New owned history remains queryable through membership after instance deletion.

## Scheduling and OAuth schema alignment

Migration 010 introduces credit storage and the original OAuth identity table. Migration 011 adds `compute_nodes.unschedulable` as a non-null boolean defaulting to false. Existing node configuration is preserved; repeated migration does not reset an administrator's drain flag.

Migration 012 renames `oauth_identities` to `o_auth_identities`, matching the current GORM `OAuthIdentity` naming convention and SQLite's existing table. It preserves linked accounts and the unique `(provider, provider_user_id)` index instead of copying or recreating rows. A conflicting pre-existing destination table causes migration to fail rather than merge identities speculatively. Apply migrations before starting the matching API binary; no reverse migration or mixed-version compatibility is supplied.

The PostgreSQL integration suite inserts an identity under migration 010 and verifies lookup/update after 012 using a runtime role without DDL privileges. It also verifies duplicate rejection and that the same remote subject may belong to different providers. This is database compatibility evidence, not a third-party OAuth login, account recovery, payment or credit-accounting acceptance test.

Migration 013 adds `compute_nodes.runtime_architecture` with a non-null empty default. Existing rows stay unknown until an Agent reports the Docker daemon architecture. It is deliberately not backfilled from OSInfo: the Agent process and Docker daemon may have different architectures. Registration and heartbeat normalize recognized aliases; omitted or failed probes clear prior evidence. Repeated migration preserves subsequently reported values.

### 014 节点端口预留

`node_port_reservations` 持久化每个节点、端口、实例的占用，主端口和 Provider 附加端口均参与。数字端口保守地跨协议互斥。迁移回填现有实例与任务清单，保留历史冲突的所有占用者；主键包含实例 ID 是为保留历史证据，当前写入通过节点端口池锁串行检查冲突。

升级必须停止旧写入进程后运行迁移；尚不支持新旧写入协议混跑。SQLite 使用一次性迁移版本 2 回填。替换或删除任务不会隐式释放端口；当前节点的受租约授权删除完成回报才释放该实例预留。迁移源节点和配置更新后的旧端口仍需可靠运行时证据才能回收。
