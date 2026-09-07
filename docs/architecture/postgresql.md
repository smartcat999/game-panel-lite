# PostgreSQL persistence foundation

Set `GAMEPANEL_DATABASE_URL` to a PostgreSQL URL/DSN supplied by deployment secrets. When unset, `GAMEPANEL_DB_PATH` continues to select SQLite. `GAMEPANEL_DB_MAX_CONNECTIONS` defaults to 20 per API process. The PostgreSQL pool caps open/idle connections at this number and rotates connections after 30 minutes; size the total across replicas against database capacity. Invalid/nonpositive environment values currently fall back to 20.

Use TLS settings appropriate to the deployment. No production credentials belong in repository files. The test environment uses an ephemeral local database; its trust authentication/disabled TLS must not be copied into a public deployment.

The adapter follows the [official GORM PostgreSQL and connection pool documentation](https://gorm.io/docs/connecting_to_the_database.html). Driver parsing/connection errors are redacted to avoid disclosing DSNs. PostgreSQL SQL logging is disabled in this baseline adapter. Application shutdown closes the database pool.

Run `GAMEPANEL_TEST_POSTGRES_URL=<test database URL> go test -count=1 -run TestPostgresIntegration -v ./apps/api/internal/store`. The test creates a uniquely named schema, exercises JSON round trips, transaction rollback and chronological queries, then drops only that schema. The account requires schema creation rights. The CI PostgreSQL service is ephemeral.

This is not yet a production SaaS database rollout. Schema initialization still uses existing GORM AutoMigrate; explicit versioned migrations, serialized migration deployment, SQLite data transfer, tenant ownership/RLS, replica concurrency and backup/recovery validation are unfinished. Do not start multiple production replicas assuming this work supplies those guarantees. SQLite's historical rowid tie break is retained; PostgreSQL orders equal creation timestamps by record ID, which is deterministic but does not imply insertion order.
