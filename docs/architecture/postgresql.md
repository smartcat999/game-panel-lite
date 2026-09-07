# PostgreSQL persistence foundation

Set `GAMEPANEL_DATABASE_URL` to a PostgreSQL URL/DSN supplied by deployment secrets. When unset, `GAMEPANEL_DB_PATH` continues to select SQLite. `GAMEPANEL_DB_MAX_CONNECTIONS` defaults to 20 per API process. The PostgreSQL pool caps open/idle connections at this number and rotates connections after 30 minutes; size the total across replicas against database capacity. Invalid/nonpositive environment values currently fall back to 20.

Use TLS settings appropriate to the deployment. No production credentials belong in repository files. The test environment uses an ephemeral local database; its trust authentication/disabled TLS must not be copied into a public deployment.

The adapter follows the [official GORM PostgreSQL and connection pool documentation](https://gorm.io/docs/connecting_to_the_database.html). Driver parsing/connection errors are redacted to avoid disclosing DSNs. PostgreSQL SQL logging is disabled in this baseline adapter. Application shutdown closes the database pool.

Run `GAMEPANEL_TEST_POSTGRES_URL=<test database URL> go test -count=1 -run TestPostgresIntegration -v ./apps/api/internal/store`. The test creates a uniquely named schema, exercises JSON round trips, transaction rollback and chronological queries, then drops only that schema. The account requires schema creation rights. The CI PostgreSQL service is ephemeral.

This is not yet a production SaaS database rollout. PostgreSQL now uses immutable embedded SQL migrations and a checksum ledger. SQLite still uses existing GORM AutoMigrate. SQLite data transfer, tenant ownership/RLS, application replica concurrency and backup/recovery validation are unfinished. Do not start multiple production replicas assuming this work supplies those guarantees. SQLite's historical rowid tie break is retained; PostgreSQL orders equal creation timestamps by record ID, which is deterministic but does not imply insertion order.

## PostgreSQL migration protocol

The initial schema is fixed in `apps/api/internal/store/migrations/001_postgres_baseline.sql`, captured from the previous PostgreSQL Store schema and verified by real persistence tests. Future schema changes must append numbered scripts; editing an applied script causes startup to reject the checksum mismatch. An older binary refuses a ledger containing versions it does not know.

Initialization acquires a schema-scoped PostgreSQL transaction advisory lock, applies pending DDL and ledger rows in the same transaction, then commits. The initializer has a one-minute context deadline. The lock follows [PostgreSQL's transaction-level advisory lock behavior](https://www.postgresql.org/docs/current/explicit-locking.html#ADVISORY-LOCKS). Four concurrent initializers and intentionally failing DDL have been exercised against PostgreSQL 16. This serializes migrations only; it does not serialize application controllers.

Use an empty application schema for first initialization. A populated schema without the migration ledger is deliberately rejected. Databases created by the earlier AutoMigrate-based PostgreSQL experiment require an explicit, separately validated adoption/data-transfer procedure that is not yet implemented. No tables are automatically dropped or adopted. The current baseline has no destructive down migration; reported migration errors roll back their transaction, while deployment downgrade/data restoration requires a separate plan. Dedicated migration-job deployment and least-privilege runtime roles remain pending.
