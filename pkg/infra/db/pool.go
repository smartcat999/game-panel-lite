package db

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/config"
)

var gormPoolCache sync.Map

// Config is an alias for config.DatabaseConfig managed centrally in pkg/config.
type Config = config.DatabaseConfig

func NewConfig() *Config {
	return config.NewDatabaseConfig()
}

// OpenPool initializes and migrates the PostgreSQL connection pool from config.DatabaseConfig.
func OpenPool(ctx context.Context, cfg *Config) (*pgxpool.Pool, func(), error) {
	if cfg == nil {
		cfg = &config.Get().Database
	}
	if strings.TrimSpace(cfg.DSN) == "" {
		return nil, nil, errors.New("database.dsn is required")
	}
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, nil, err
	}
	if cfg.MaxOpenConns > 0 {
		poolCfg.MaxConns = int32(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		poolCfg.MinConns = int32(cfg.MaxIdleConns)
	}
	if cfg.ConnMaxLifetime > 0 {
		poolCfg.MaxConnLifetime = cfg.ConnMaxLifetime
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, nil, err
	}
	if err := MigrateSchema(ctx, pool); err != nil {
		pool.Close()
		return nil, nil, err
	}
	return pool, pool.Close, nil
}

// MigrateSchema ensures all strongly-typed domain tables, PostgreSQL CHECK constraints, and GIN/partial indexes exist.
func MigrateSchema(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return nil
	}
	if _, err := pool.Exec(ctx, schemaBootstrapDDL); err != nil {
		return err
	}
	return autoMigrateTables(OpenGORM(pool))
}

func autoMigrateTables(orm *gorm.DB) error {
	if orm == nil {
		return nil
	}
	return orm.AutoMigrate(
		&iamv1.UserRecord{},
		&iamv1.UserStateRecord{},
		&iamv1.GlobalRoleRecord{},
		&iamv1.GlobalRoleBindingRecord{},
		&iamv1.TenantRoleRecord{},
		&iamv1.TenantRoleBindingRecord{},
		&corev1.TenantRecord{},
		&corev1.TenantStateRecord{},
		&corev1.RuntimeClassRecord{},
		&corev1.AppRecord{},
		&corev1.PrewarmJobRecord{},
		&corev1.PrewarmJobStateRecord{},
		&corev1.NodeRecord{},
		&corev1.NodeHeartbeatRecord{},
		&corev1.InstanceRecord{},
		&corev1.InstanceObservationRecord{},
		&corev1.InstanceExecutionFactRecord{},
		&corev1.AuditEventRecord{},
		&corev1.BackupGateRecord{},
		&corev1.BackupCommandRecord{},
		&corev1.QuotaAccountRecord{},
		&corev1.QuotaPolicyChangeRecord{},
		&corev1.ResourcePlanRecord{},
		&corev1.QuotaOrderRecord{},
		&corev1.QuotaRedeemCodeRecord{},
		&corev1.QuotaAllocationRecord{},
		&corev1.QuotaOperationRecord{},
		&corev1.QuotaChangeRecord{},
		&corev1.NodeAccountRecord{},
		&corev1.NodeReservationRecord{},
		&corev1.NodeOperationRecord{},
		&corev1.NodeChangeRecord{},
		&corev1.NodePortRecord{},
		&registryv1.UpstreamRecord{},
		&registryv1.RepositoryRecord{},
		&registryv1.RepositoryStateRecord{},
		&registryv1.PackageRecord{},
		&registryv1.ArtifactRecord{},
		&registryv1.ArtifactStateRecord{},
		&registryv1.BlobRecord{},
		&registryv1.BlobRefRecord{},
		&registryv1.BlobUploadRecord{},
		&registryv1.ReplicationPolicyRecord{},
	)
}

// OpenGORM returns a shared *gorm.DB instance backed by the given pgxpool.Pool.
func OpenGORM(pool *pgxpool.Pool) *gorm.DB {
	if pool == nil {
		return nil
	}
	if cached, ok := gormPoolCache.Load(pool); ok {
		return cached.(*gorm.DB)
	}
	sqlDB := stdlib.OpenDBFromPool(pool)
	orm, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{
		SkipDefaultTransaction: true,
		Logger:                 logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		panic(err)
	}
	actual, _ := gormPoolCache.LoadOrStore(pool, orm)
	return actual.(*gorm.DB)
}

const schemaBootstrapDDL = `
CREATE TABLE IF NOT EXISTS users (
    name             TEXT        PRIMARY KEY,
    uid              TEXT        NOT NULL CONSTRAINT uni_users_uid UNIQUE,
    email            TEXT        NOT NULL,
    display_name     TEXT        NOT NULL DEFAULT '',
    password_hash    TEXT        NOT NULL DEFAULT '',
    state            TEXT        NOT NULL DEFAULT 'Active',
    resource_version BIGINT      NOT NULL DEFAULT 1 CHECK (resource_version > 0),
    generation       BIGINT      NOT NULL DEFAULT 1 CHECK (generation > 0),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at       TIMESTAMPTZ NULL,
    finalizers       JSONB       NOT NULL DEFAULT '[]'::jsonb,
    labels           JSONB       NOT NULL DEFAULT '{}'::jsonb,
    annotations      JSONB       NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX IF NOT EXISTS idx_users_email ON users (LOWER(email));
CREATE INDEX IF NOT EXISTS idx_users_labels ON users USING GIN (labels);

CREATE TABLE IF NOT EXISTS user_states (
    user_uid            TEXT        PRIMARY KEY REFERENCES users(uid) ON DELETE CASCADE,
    version             BIGINT      NOT NULL DEFAULT 1 CHECK (version > 0),
    observed_generation BIGINT      NOT NULL DEFAULT 0,
    phase               TEXT        NOT NULL DEFAULT 'Active',
    default_tenant      TEXT        NOT NULL DEFAULT '',
    last_login_time     TIMESTAMPTZ NULL,
    conditions          JSONB       NOT NULL DEFAULT '[]'::jsonb,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_user_states_phase ON user_states (phase);

CREATE TABLE IF NOT EXISTS global_roles (
    name             TEXT        PRIMARY KEY,
    uid              TEXT        NOT NULL CONSTRAINT uni_global_roles_uid UNIQUE,
    description      TEXT        NOT NULL DEFAULT '',
    rules            JSONB       NOT NULL DEFAULT '[]'::jsonb,
    resource_version BIGINT      NOT NULL DEFAULT 1 CHECK (resource_version > 0),
    generation       BIGINT      NOT NULL DEFAULT 1 CHECK (generation > 0),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at       TIMESTAMPTZ NULL,
    finalizers       JSONB       NOT NULL DEFAULT '[]'::jsonb,
    labels           JSONB       NOT NULL DEFAULT '{}'::jsonb,
    annotations      JSONB       NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX IF NOT EXISTS idx_global_roles_labels ON global_roles USING GIN (labels);

CREATE TABLE IF NOT EXISTS global_role_bindings (
    name             TEXT        PRIMARY KEY,
    uid              TEXT        NOT NULL CONSTRAINT uni_global_role_bindings_uid UNIQUE,
    role_name        TEXT        NOT NULL,
    subjects         JSONB       NOT NULL DEFAULT '[]'::jsonb,
    resource_version BIGINT      NOT NULL DEFAULT 1 CHECK (resource_version > 0),
    generation       BIGINT      NOT NULL DEFAULT 1 CHECK (generation > 0),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at       TIMESTAMPTZ NULL,
    finalizers       JSONB       NOT NULL DEFAULT '[]'::jsonb,
    labels           JSONB       NOT NULL DEFAULT '{}'::jsonb,
    annotations      JSONB       NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX IF NOT EXISTS idx_global_role_bindings_subjects ON global_role_bindings USING GIN (subjects);
CREATE INDEX IF NOT EXISTS idx_global_role_bindings_labels ON global_role_bindings USING GIN (labels);

CREATE TABLE IF NOT EXISTS tenant_roles (
    tenant           TEXT        NOT NULL,
    name             TEXT        NOT NULL,
    uid              TEXT        NOT NULL CONSTRAINT uni_tenant_roles_uid UNIQUE,
    description      TEXT        NOT NULL DEFAULT '',
    rules            JSONB       NOT NULL DEFAULT '[]'::jsonb,
    resource_version BIGINT      NOT NULL DEFAULT 1 CHECK (resource_version > 0),
    generation       BIGINT      NOT NULL DEFAULT 1 CHECK (generation > 0),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at       TIMESTAMPTZ NULL,
    finalizers       JSONB       NOT NULL DEFAULT '[]'::jsonb,
    labels           JSONB       NOT NULL DEFAULT '{}'::jsonb,
    annotations      JSONB       NOT NULL DEFAULT '{}'::jsonb,
    PRIMARY KEY (tenant, name)
);
CREATE INDEX IF NOT EXISTS idx_tenant_roles_labels ON tenant_roles USING GIN (labels);

CREATE TABLE IF NOT EXISTS tenant_role_bindings (
    tenant           TEXT        NOT NULL,
    name             TEXT        NOT NULL,
    uid              TEXT        NOT NULL CONSTRAINT uni_tenant_role_bindings_uid UNIQUE,
    role_name        TEXT        NOT NULL,
    subjects         JSONB       NOT NULL DEFAULT '[]'::jsonb,
    resource_version BIGINT      NOT NULL DEFAULT 1 CHECK (resource_version > 0),
    generation       BIGINT      NOT NULL DEFAULT 1 CHECK (generation > 0),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at       TIMESTAMPTZ NULL,
    finalizers       JSONB       NOT NULL DEFAULT '[]'::jsonb,
    labels           JSONB       NOT NULL DEFAULT '{}'::jsonb,
    annotations      JSONB       NOT NULL DEFAULT '{}'::jsonb,
    PRIMARY KEY (tenant, name)
);
CREATE INDEX IF NOT EXISTS idx_tenant_role_bindings_subjects ON tenant_role_bindings USING GIN (subjects);
CREATE INDEX IF NOT EXISTS idx_tenant_role_bindings_labels ON tenant_role_bindings USING GIN (labels);

CREATE TABLE IF NOT EXISTS tenants (
    name                    TEXT        PRIMARY KEY,
    uid                     TEXT        NOT NULL CONSTRAINT uni_tenants_uid UNIQUE,
    display_name            TEXT        NOT NULL DEFAULT '',
    type                    TEXT        NOT NULL DEFAULT 'Organization',
    owner                   TEXT        NOT NULL,
    billing_mode            TEXT        NOT NULL DEFAULT 'Manual',
    quota                   JSONB       NOT NULL DEFAULT '{}'::jsonb,
    unlimited_quota         JSONB       NOT NULL DEFAULT '[]'::jsonb,
    alert_rules             JSONB       NOT NULL DEFAULT '[]'::jsonb,
    alert_channels          JSONB       NOT NULL DEFAULT '[]'::jsonb,
    platform_alert_channels JSONB       NOT NULL DEFAULT '[]'::jsonb,
    alert_silences          JSONB       NOT NULL DEFAULT '[]'::jsonb,
    platform_alert_silences JSONB       NOT NULL DEFAULT '[]'::jsonb,
    resource_version        BIGINT      NOT NULL DEFAULT 1 CHECK (resource_version > 0),
    generation              BIGINT      NOT NULL DEFAULT 1 CHECK (generation > 0),
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at              TIMESTAMPTZ NULL,
    finalizers              JSONB       NOT NULL DEFAULT '[]'::jsonb,
    labels                  JSONB       NOT NULL DEFAULT '{}'::jsonb,
    annotations             JSONB       NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX IF NOT EXISTS idx_tenants_owner ON tenants (owner);
CREATE INDEX IF NOT EXISTS idx_tenants_labels ON tenants USING GIN (labels);

CREATE TABLE IF NOT EXISTS tenant_states (
    tenant_uid          TEXT        PRIMARY KEY REFERENCES tenants(uid) ON DELETE CASCADE,
    version             BIGINT      NOT NULL DEFAULT 1 CHECK (version > 0),
    observed_generation BIGINT      NOT NULL DEFAULT 0,
    phase               TEXT        NOT NULL DEFAULT 'Active',
    usage               JSONB       NOT NULL DEFAULT '{}'::jsonb,
    subscription        JSONB       NULL,
    conditions          JSONB       NOT NULL DEFAULT '[]'::jsonb,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS billing_mode TEXT NOT NULL DEFAULT 'Manual';
ALTER TABLE tenant_states ADD COLUMN IF NOT EXISTS subscription JSONB NULL;
CREATE INDEX IF NOT EXISTS idx_tenant_states_phase ON tenant_states (phase);

CREATE TABLE IF NOT EXISTS resource_plans (
    name             TEXT        PRIMARY KEY,
    uid              TEXT        NOT NULL CONSTRAINT uni_resource_plans_uid UNIQUE,
    display_name     TEXT        NOT NULL DEFAULT '',
    description      TEXT        NOT NULL DEFAULT '',
    kind             TEXT        NOT NULL CHECK (kind IN ('Base', 'Addon')),
    quota            JSONB       NOT NULL DEFAULT '{}'::jsonb,
    sort_order       INT         NOT NULL DEFAULT 0,
    published        BOOLEAN     NOT NULL DEFAULT TRUE,
    prices           JSONB       NOT NULL DEFAULT '[]'::jsonb,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resource_version BIGINT      NOT NULL DEFAULT 1 CHECK (resource_version > 0),
    labels           JSONB       NOT NULL DEFAULT '{}'::jsonb,
    annotations      JSONB       NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX IF NOT EXISTS idx_resource_plans_published ON resource_plans (published, sort_order, name);

CREATE TABLE IF NOT EXISTS runtime_classes (
    name             TEXT        PRIMARY KEY,
    uid              TEXT        NOT NULL CONSTRAINT uni_runtime_classes_uid UNIQUE,
    handler          TEXT        NOT NULL,
    is_default       BOOLEAN     NOT NULL DEFAULT FALSE,
    parameters       JSONB       NOT NULL DEFAULT '{}'::jsonb,
    scheduling       JSONB       NULL,
    resource_version BIGINT      NOT NULL DEFAULT 1 CHECK (resource_version > 0),
    generation       BIGINT      NOT NULL DEFAULT 1 CHECK (generation > 0),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at       TIMESTAMPTZ NULL,
    finalizers       JSONB       NOT NULL DEFAULT '[]'::jsonb,
    labels           JSONB       NOT NULL DEFAULT '{}'::jsonb,
    annotations      JSONB       NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX IF NOT EXISTS idx_runtime_classes_default ON runtime_classes (is_default);
CREATE INDEX IF NOT EXISTS idx_runtime_classes_labels ON runtime_classes USING GIN (labels);

CREATE TABLE IF NOT EXISTS apps (
    name                TEXT        PRIMARY KEY,
    uid                 TEXT        NOT NULL CONSTRAINT uni_apps_uid UNIQUE,
    display_name        TEXT        NOT NULL,
    tagline             TEXT        NOT NULL DEFAULT '',
    summary             TEXT        NOT NULL DEFAULT '',
    category            TEXT        NOT NULL DEFAULT 'survival',
    cover_image         TEXT        NOT NULL DEFAULT '',
    image               TEXT        NOT NULL,
    image_template      TEXT        NOT NULL DEFAULT '',
    versions            JSONB       NOT NULL DEFAULT '[]'::jsonb,
    recommended_version TEXT        NOT NULL DEFAULT '',
    server_binding      JSONB       NULL,
    addon_binding       JSONB       NULL,
    runtime_class_name  TEXT        NOT NULL DEFAULT '',
    pre_start_command   TEXT        NOT NULL DEFAULT '',
    startup_command     TEXT        NOT NULL DEFAULT '',
    pre_stop_command    TEXT        NOT NULL DEFAULT '',
    command             JSONB       NOT NULL DEFAULT '[]'::jsonb,
    working_dir         TEXT        NOT NULL DEFAULT '',
    ports               JSONB       NOT NULL DEFAULT '[]'::jsonb,
    env                 JSONB       NOT NULL DEFAULT '{}'::jsonb,
    recommended_plan    TEXT        NOT NULL DEFAULT 'standard',
    resources           JSONB       NOT NULL DEFAULT '{}'::jsonb,
    config_schema       JSONB       NOT NULL DEFAULT '[]'::jsonb,
    config_presets      JSONB       NOT NULL DEFAULT '[]'::jsonb,
    file_templates      JSONB       NOT NULL DEFAULT '[]'::jsonb,
    console_delivery    TEXT        NOT NULL DEFAULT '',
    console_targets     JSONB       NOT NULL DEFAULT '[]'::jsonb,
    console_actions     JSONB       NOT NULL DEFAULT '[]'::jsonb,
    storage_policy      JSONB       NULL,
    metrics             JSONB       NOT NULL DEFAULT '[]'::jsonb,
    capabilities        JSONB       NOT NULL DEFAULT '[]'::jsonb,
    published           BOOLEAN     NOT NULL DEFAULT FALSE,
    built_in            BOOLEAN     NOT NULL DEFAULT FALSE,
    resource_version    BIGINT      NOT NULL DEFAULT 1 CHECK (resource_version > 0),
    generation          BIGINT      NOT NULL DEFAULT 1 CHECK (generation > 0),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at          TIMESTAMPTZ NULL,
    finalizers          JSONB       NOT NULL DEFAULT '[]'::jsonb,
    labels              JSONB       NOT NULL DEFAULT '{}'::jsonb,
    annotations         JSONB       NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX IF NOT EXISTS idx_apps_category ON apps (category);
CREATE INDEX IF NOT EXISTS idx_apps_labels ON apps USING GIN (labels);

CREATE TABLE IF NOT EXISTS prewarm_jobs (
    name             TEXT        PRIMARY KEY,
    uid              TEXT        NOT NULL CONSTRAINT uni_prewarm_jobs_uid UNIQUE,
    node_names       JSONB       NOT NULL DEFAULT '[]'::jsonb,
    node_selector    JSONB       NOT NULL DEFAULT '{}'::jsonb,
    artifacts        JSONB       NOT NULL DEFAULT '[]'::jsonb,
    images           JSONB       NOT NULL DEFAULT '[]'::jsonb,
    resource_version BIGINT      NOT NULL DEFAULT 1 CHECK (resource_version > 0),
    generation       BIGINT      NOT NULL DEFAULT 1 CHECK (generation > 0),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at       TIMESTAMPTZ NULL,
    finalizers       JSONB       NOT NULL DEFAULT '[]'::jsonb,
    labels           JSONB       NOT NULL DEFAULT '{}'::jsonb,
    annotations      JSONB       NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX IF NOT EXISTS idx_prewarm_jobs_labels ON prewarm_jobs USING GIN (labels);

CREATE TABLE IF NOT EXISTS prewarm_job_states (
    job_uid             TEXT        PRIMARY KEY REFERENCES prewarm_jobs(uid) ON DELETE CASCADE,
    version             BIGINT      NOT NULL DEFAULT 1 CHECK (version > 0),
    observed_generation BIGINT      NOT NULL DEFAULT 0,
    phase               TEXT        NOT NULL DEFAULT 'Pending',
    target_nodes        INTEGER     NOT NULL DEFAULT 0,
    ready_nodes         INTEGER     NOT NULL DEFAULT 0,
    failed_nodes        INTEGER     NOT NULL DEFAULT 0,
    nodes               JSONB       NOT NULL DEFAULT '[]'::jsonb,
    conditions          JSONB       NOT NULL DEFAULT '[]'::jsonb,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_prewarm_job_states_phase ON prewarm_job_states (phase);

CREATE TABLE IF NOT EXISTS nodes (
    name             TEXT        PRIMARY KEY,
    uid              TEXT        NOT NULL CONSTRAINT uni_nodes_uid UNIQUE,
    unschedulable    BOOLEAN     NOT NULL DEFAULT FALSE,
    external_ip      TEXT        NOT NULL DEFAULT '',
    allowed_apps     JSONB       NOT NULL DEFAULT '[]'::jsonb,
    resource_version BIGINT      NOT NULL DEFAULT 1 CHECK (resource_version > 0),
    generation       BIGINT      NOT NULL DEFAULT 1 CHECK (generation > 0),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at       TIMESTAMPTZ NULL,
    finalizers       JSONB       NOT NULL DEFAULT '[]'::jsonb,
    labels           JSONB       NOT NULL DEFAULT '{}'::jsonb,
    annotations      JSONB       NOT NULL DEFAULT '{}'::jsonb
);
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS allowed_apps JSONB NOT NULL DEFAULT '[]'::jsonb;
CREATE INDEX IF NOT EXISTS idx_nodes_labels ON nodes USING GIN (labels);

CREATE TABLE IF NOT EXISTS node_heartbeats (
    node_uid            TEXT        PRIMARY KEY REFERENCES nodes(uid) ON DELETE CASCADE,
    node_name           TEXT        NOT NULL,
    version             BIGINT      NOT NULL DEFAULT 1 CHECK (version > 0),
    observed_generation BIGINT      NOT NULL DEFAULT 0,
    phase               TEXT        NOT NULL DEFAULT 'Ready',
    addresses           JSONB       NOT NULL DEFAULT '[]'::jsonb,
    runtimes            JSONB       NOT NULL DEFAULT '[]'::jsonb,
    capacity            JSONB       NOT NULL DEFAULT '{}'::jsonb,
    allocatable         JSONB       NOT NULL DEFAULT '{}'::jsonb,
    usage               JSONB       NULL,
    cached_artifacts    JSONB       NOT NULL DEFAULT '[]'::jsonb,
    cached_blobs        JSONB       NOT NULL DEFAULT '[]'::jsonb,
    warm_images         JSONB       NOT NULL DEFAULT '[]'::jsonb,
    last_heartbeat_time TIMESTAMPTZ NULL,
    conditions          JSONB       NOT NULL DEFAULT '[]'::jsonb,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_node_heartbeats_phase ON node_heartbeats (phase);

CREATE TABLE IF NOT EXISTS instances (
    tenant             TEXT        NOT NULL,
    name               TEXT        NOT NULL,
    uid                TEXT        NOT NULL CONSTRAINT uni_instances_uid UNIQUE,
    runtime_class_name TEXT        NOT NULL DEFAULT '',
    state              TEXT        NOT NULL DEFAULT 'Running',
    node_name          TEXT        NOT NULL DEFAULT '',
    node_selector      JSONB       NOT NULL DEFAULT '{}'::jsonb,
    image              TEXT        NOT NULL DEFAULT '',
    command            JSONB       NOT NULL DEFAULT '[]'::jsonb,
    args               JSONB       NOT NULL DEFAULT '[]'::jsonb,
    working_dir        TEXT        NOT NULL DEFAULT '',
    server_artifact    JSONB       NULL,
    addons             JSONB       NOT NULL DEFAULT '[]'::jsonb,
    resources          JSONB       NOT NULL DEFAULT '{}'::jsonb,
    ports              JSONB       NOT NULL DEFAULT '[]'::jsonb,
    env                JSONB       NOT NULL DEFAULT '{}'::jsonb,
    files              JSONB       NOT NULL DEFAULT '[]'::jsonb,
    backup_policy      JSONB       NULL,
    schedules          JSONB       NOT NULL DEFAULT '[]'::jsonb,
    metrics            JSONB       NOT NULL DEFAULT '[]'::jsonb,
    control_stage      TEXT        NOT NULL DEFAULT '',
    resource_control   JSONB       NULL,
    control_conditions JSONB       NOT NULL DEFAULT '[]'::jsonb,
    resource_version   BIGINT      NOT NULL DEFAULT 1 CHECK (resource_version > 0),
    generation         BIGINT      NOT NULL DEFAULT 1 CHECK (generation > 0),
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at         TIMESTAMPTZ NULL,
    finalizers         JSONB       NOT NULL DEFAULT '[]'::jsonb,
    labels             JSONB       NOT NULL DEFAULT '{}'::jsonb,
    annotations        JSONB       NOT NULL DEFAULT '{}'::jsonb,
    PRIMARY KEY (tenant, name)
);
CREATE INDEX IF NOT EXISTS idx_instances_nodename ON instances (node_name);
CREATE INDEX IF NOT EXISTS idx_instances_state ON instances (state);
CREATE INDEX IF NOT EXISTS idx_instances_control_stage ON instances (control_stage);
CREATE INDEX IF NOT EXISTS idx_instances_labels ON instances USING GIN (labels);

CREATE TABLE IF NOT EXISTS instance_observations (
    instance_uid        TEXT        PRIMARY KEY REFERENCES instances(uid) ON DELETE CASCADE,
    tenant              TEXT        NOT NULL,
    instance_name       TEXT        NOT NULL,
    version             BIGINT      NOT NULL DEFAULT 1 CHECK (version > 0),
    observed_generation BIGINT      NOT NULL DEFAULT 0,
    phase               TEXT        NOT NULL DEFAULT 'Pending',
    runtime_id          TEXT        NOT NULL DEFAULT '',
    endpoint            JSONB       NOT NULL DEFAULT '{}'::jsonb,
    started_at          TIMESTAMPTZ NULL,
    observed_at         TIMESTAMPTZ NULL,
    transition_id       TEXT        NOT NULL DEFAULT '',
    observed_revision   BIGINT      NOT NULL DEFAULT 0,
    observed_attempt_id TEXT        NOT NULL DEFAULT '',
    observed_epoch      BIGINT      NOT NULL DEFAULT 0,
    observed_resources  JSONB       NOT NULL DEFAULT '{}'::jsonb,
    observed_ports      JSONB       NOT NULL DEFAULT '[]'::jsonb,
    conditions          JSONB       NOT NULL DEFAULT '[]'::jsonb,
    schedule_statuses   JSONB       NOT NULL DEFAULT '[]'::jsonb,
    backups             JSONB       NOT NULL DEFAULT '[]'::jsonb,
    usage               JSONB       NULL,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_instance_observations_tenant_name ON instance_observations (tenant, instance_name);
CREATE INDEX IF NOT EXISTS idx_instance_observations_phase ON instance_observations (phase);

CREATE TABLE IF NOT EXISTS instance_execution_facts (
    fact_id       TEXT        PRIMARY KEY,
    tenant        TEXT        NOT NULL,
    tenant_uid    TEXT        NOT NULL DEFAULT '',
    instance_uid  TEXT        NOT NULL,
    instance_name TEXT        NOT NULL,
    fact_type     TEXT        NOT NULL,
    occurred_at   TIMESTAMPTZ NOT NULL,
    attributes    JSONB       NOT NULL DEFAULT '{}'::jsonb,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_instance_execution_facts_tenant_window ON instance_execution_facts (tenant, tenant_uid, occurred_at ASC, fact_id ASC);
CREATE INDEX IF NOT EXISTS idx_instance_execution_facts_instance_time ON instance_execution_facts (tenant, tenant_uid, instance_uid, occurred_at DESC, fact_id DESC);

CREATE TABLE IF NOT EXISTS audit_events (
    id            TEXT        PRIMARY KEY,
    tenant        TEXT        NOT NULL DEFAULT '',
    actor         TEXT        NOT NULL,
    verb          TEXT        NOT NULL,
    api_group     TEXT        NOT NULL DEFAULT '',
    resource      TEXT        NOT NULL,
    subresource   TEXT        NOT NULL DEFAULT '',
    resource_name TEXT        NOT NULL DEFAULT '',
    action        TEXT        NOT NULL DEFAULT '',
    target_name   TEXT        NOT NULL DEFAULT '',
    status_code   INT         NOT NULL,
    client_ip     TEXT        NOT NULL DEFAULT '',
    request_id    TEXT        NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    payload       JSONB       NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_audit_events_tenant_time ON audit_events (tenant, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_events_actor_time ON audit_events (actor, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_events_resource_time ON audit_events (resource, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_events_tenant_resource_time ON audit_events (tenant, resource, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_events_webhook_inflight ON audit_events (created_at, id) WHERE resource = 'alerts' AND subresource = 'webhook' AND action IN ('WebhookPending', 'WebhookRetrying', 'WebhookSending');

CREATE TABLE IF NOT EXISTS quota_accounts (
    tenant_uid TEXT PRIMARY KEY,
    policy_version BIGINT NOT NULL DEFAULT 0 CHECK (policy_version >= 0),
    upstream_version BIGINT NOT NULL DEFAULT 0 CHECK (upstream_version >= 0),
    expires_at TIMESTAMPTZ NULL,
    limit_cpu BIGINT DEFAULT 0 CHECK (limit_cpu >= 0),
    limit_mem BIGINT DEFAULT 0 CHECK (limit_mem >= 0),
    limit_storage BIGINT DEFAULT 0 CHECK (limit_storage >= 0),
    limit_instances BIGINT DEFAULT 0 CHECK (limit_instances >= 0),
    limit_backups BIGINT DEFAULT 0 CHECK (limit_backups >= 0),
    limit_backup_storage BIGINT DEFAULT 0 CHECK (limit_backup_storage >= 0),
    allocated_cpu BIGINT NOT NULL DEFAULT 0 CHECK (allocated_cpu >= 0),
    allocated_mem BIGINT NOT NULL DEFAULT 0 CHECK (allocated_mem >= 0),
    allocated_storage BIGINT NOT NULL DEFAULT 0 CHECK (allocated_storage >= 0),
    allocated_instances BIGINT NOT NULL DEFAULT 0 CHECK (allocated_instances >= 0),
    allocated_backups BIGINT NOT NULL DEFAULT 0 CHECK (allocated_backups >= 0),
    allocated_backup_storage BIGINT NOT NULL DEFAULT 0 CHECK (allocated_backup_storage >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS quota_policy_changes (
    change_id TEXT PRIMARY KEY,
    tenant_uid TEXT NOT NULL REFERENCES quota_accounts(tenant_uid),
    request_hash TEXT NOT NULL CHECK (length(request_hash) = 64),
    plan JSONB NOT NULL,
    source_version BIGINT NOT NULL CHECK (source_version >= 0),
    exported_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_quota_policy_changes_tenant ON quota_policy_changes (tenant_uid);
CREATE UNIQUE INDEX IF NOT EXISTS uq_quota_policy_changes_source ON quota_policy_changes (tenant_uid, source_version);
CREATE INDEX IF NOT EXISTS idx_quota_policy_changes_unexported ON quota_policy_changes (created_at) WHERE exported_at IS NULL;

CREATE TABLE IF NOT EXISTS quota_orders (
    id             TEXT        PRIMARY KEY,
    tenant_uid     TEXT        NOT NULL REFERENCES tenants(uid) ON DELETE CASCADE,
    order_kind     TEXT        NOT NULL CHECK (order_kind IN ('BasePlan', 'AddonPack')),
    plan_name      TEXT        NOT NULL,
    source         TEXT        NOT NULL CHECK (source IN ('Trial', 'Redeem', 'AdminGrant', 'Checkout')),
    source_ref     TEXT        NOT NULL DEFAULT '',
    reason         TEXT        NOT NULL DEFAULT '',
    quota_delta    JSONB       NOT NULL DEFAULT '{}'::jsonb,
    duration_days  INT         NOT NULL CHECK (duration_days > 0),
    amount_cents   BIGINT      NOT NULL DEFAULT 0 CHECK (amount_cents >= 0),
    currency       TEXT        NOT NULL DEFAULT '',
    state          TEXT        NOT NULL DEFAULT 'Pending' CHECK (state IN ('Pending', 'Paid', 'Cancelled')),
    paid_at        TIMESTAMPTZ NULL,
    expires_at     TIMESTAMPTZ NULL,
    applied_at     TIMESTAMPTZ NULL,
    exported_at    TIMESTAMPTZ NULL,
    policy_version BIGINT      NOT NULL DEFAULT 0 CHECK (policy_version >= 0),
    created_by     TEXT        NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
ALTER TABLE quota_orders ADD COLUMN IF NOT EXISTS exported_at TIMESTAMPTZ NULL;
CREATE INDEX IF NOT EXISTS idx_quota_orders_tenant_paid ON quota_orders (tenant_uid, paid_at DESC);
CREATE INDEX IF NOT EXISTS idx_quota_orders_pending_apply ON quota_orders (tenant_uid) WHERE state = 'Paid' AND applied_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_quota_orders_active_expires ON quota_orders (tenant_uid, expires_at) WHERE state = 'Paid' AND expires_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_quota_orders_unexported ON quota_orders (applied_at) WHERE state = 'Paid' AND applied_at IS NOT NULL AND exported_at IS NULL;

CREATE TABLE IF NOT EXISTS quota_redeem_codes (
    id                     TEXT        PRIMARY KEY,
    code_hash              TEXT        NOT NULL CONSTRAINT uq_quota_redeem_codes_hash UNIQUE,
    code_hint              TEXT        NOT NULL,
    batch_name             TEXT        NOT NULL DEFAULT '',
    order_kind             TEXT        NOT NULL CHECK (order_kind IN ('BasePlan', 'AddonPack')),
    plan_name              TEXT        NOT NULL,
    duration_days          INT         NOT NULL CHECK (duration_days > 0),
    max_redemptions        INT         NOT NULL DEFAULT 1 CHECK (max_redemptions >= 1),
    redeemed_count         INT         NOT NULL DEFAULT 0 CHECK (redeemed_count >= 0),
    redeemed_by_tenant_uid TEXT        NOT NULL DEFAULT '',
    redeemed_order_id      TEXT        NOT NULL DEFAULT '',
    redeemed_at            TIMESTAMPTZ NULL,
    disabled               BOOLEAN     NOT NULL DEFAULT FALSE,
    expires_at             TIMESTAMPTZ NULL,
    created_by             TEXT        NOT NULL DEFAULT '',
    created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_quota_redeem_codes_batch ON quota_redeem_codes (batch_name, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_quota_redeem_codes_tenant ON quota_redeem_codes (redeemed_by_tenant_uid, redeemed_at DESC) WHERE redeemed_by_tenant_uid <> '';

CREATE TABLE IF NOT EXISTS quota_allocations (
    allocation_id TEXT PRIMARY KEY,
    tenant_uid TEXT NOT NULL REFERENCES quota_accounts(tenant_uid),
    instance_uid TEXT NOT NULL,
    instance_name TEXT NOT NULL,
    initial_intent JSONB,
    intent_hash TEXT NOT NULL DEFAULT '',
    state TEXT NOT NULL CHECK (state IN ('Pending', 'Active', 'Closing', 'Closed')),
    current_cpu BIGINT NOT NULL DEFAULT 0 CHECK (current_cpu >= 0),
    current_mem BIGINT NOT NULL DEFAULT 0 CHECK (current_mem >= 0),
    current_storage BIGINT NOT NULL DEFAULT 0 CHECK (current_storage >= 0),
    current_instances BIGINT NOT NULL DEFAULT 0 CHECK (current_instances >= 0),
    current_backups BIGINT NOT NULL DEFAULT 0 CHECK (current_backups >= 0),
    current_backup_storage BIGINT NOT NULL DEFAULT 0 CHECK (current_backup_storage >= 0),
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    resource_revision BIGINT NOT NULL DEFAULT 0 CHECK (resource_revision >= 0),
    close_intent_id TEXT NOT NULL DEFAULT '',
    change_plan_id TEXT NOT NULL DEFAULT '',
    change_revision BIGINT NOT NULL DEFAULT 0 CHECK (change_revision >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK ((initial_intent IS NULL AND intent_hash = '') OR (initial_intent IS NOT NULL AND length(intent_hash) = 64)),
    CHECK (state NOT IN ('Pending', 'Active') OR length(intent_hash) = 64),
    CHECK ((state IN ('Pending', 'Active') AND close_intent_id = '') OR (state IN ('Closing', 'Closed') AND close_intent_id <> '')),
    CHECK (state NOT IN ('Pending', 'Closed') OR (current_cpu = 0 AND current_mem = 0 AND current_storage = 0 AND current_instances = 0 AND current_backups = 0 AND current_backup_storage = 0)),
    CHECK ((change_plan_id = '' AND change_revision = 0) OR (state IN ('Active', 'Closing') AND change_plan_id <> '' AND change_revision > resource_revision))
);
CREATE INDEX IF NOT EXISTS idx_quota_allocations_owner ON quota_allocations(tenant_uid, instance_uid);
CREATE INDEX IF NOT EXISTS idx_quota_allocations_pending ON quota_allocations(state, updated_at, allocation_id) WHERE state IN ('Pending', 'Closing');

CREATE TABLE IF NOT EXISTS quota_changes (
    change_id TEXT PRIMARY KEY,
    allocation_id TEXT NOT NULL REFERENCES quota_allocations(allocation_id),
    request_hash TEXT NOT NULL CHECK (length(request_hash) = 64),
    plan JSONB NOT NULL,
    source_version BIGINT NOT NULL CHECK (source_version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_quota_changes_allocation ON quota_changes(allocation_id);

CREATE TABLE IF NOT EXISTS quota_operations (
    operation_id TEXT PRIMARY KEY,
    allocation_id TEXT NOT NULL REFERENCES quota_allocations(allocation_id),
    kind TEXT NOT NULL CHECK (kind IN ('Grant', 'Increase', 'SettleDecrease', 'RequestClose', 'FinalizeClose')),
    request_hash TEXT NOT NULL CHECK (length(request_hash) = 64),
    command JSONB NOT NULL,
    change_plan_id TEXT NOT NULL DEFAULT '',
    close_intent_id TEXT NOT NULL DEFAULT '',
    state TEXT NOT NULL DEFAULT 'Accepted' CHECK (state IN ('Accepted', 'Committed', 'Cancelled')),
    result JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_quota_operations_grant ON quota_operations(allocation_id) WHERE kind = 'Grant';
CREATE UNIQUE INDEX IF NOT EXISTS uq_quota_operations_change ON quota_operations(allocation_id, kind, change_plan_id) WHERE kind IN ('Increase', 'SettleDecrease');
CREATE UNIQUE INDEX IF NOT EXISTS uq_quota_operations_close ON quota_operations(allocation_id, kind, close_intent_id) WHERE kind IN ('RequestClose', 'FinalizeClose');
CREATE INDEX IF NOT EXISTS idx_quota_operations_accepted ON quota_operations(state, updated_at, operation_id) WHERE state = 'Accepted';

CREATE TABLE IF NOT EXISTS node_accounts (
    node_uid TEXT PRIMARY KEY,
    policy_version BIGINT NOT NULL DEFAULT 0 CHECK(policy_version>=0),
    limit_cpu BIGINT DEFAULT 0 CHECK (limit_cpu>=0),
    limit_mem BIGINT DEFAULT 0 CHECK (limit_mem>=0),
    limit_storage BIGINT DEFAULT 0 CHECK (limit_storage>=0),
    limit_instances BIGINT DEFAULT 0 CHECK (limit_instances>=0),
    limit_backups BIGINT NOT NULL DEFAULT 0 CHECK (limit_backups=0),
    limit_backup_storage BIGINT NOT NULL DEFAULT 0 CHECK (limit_backup_storage=0),
    allocated_cpu BIGINT NOT NULL DEFAULT 0 CHECK (allocated_cpu>=0),
    allocated_mem BIGINT NOT NULL DEFAULT 0 CHECK (allocated_mem>=0),
    allocated_storage BIGINT NOT NULL DEFAULT 0 CHECK (allocated_storage>=0),
    allocated_instances BIGINT NOT NULL DEFAULT 0 CHECK (allocated_instances>=0),
    allocated_backups BIGINT NOT NULL DEFAULT 0 CHECK (allocated_backups=0),
    allocated_backup_storage BIGINT NOT NULL DEFAULT 0 CHECK (allocated_backup_storage=0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS node_reservations (
    allocation_id TEXT PRIMARY KEY,
    node_uid TEXT NOT NULL REFERENCES node_accounts(node_uid),
    tenant_uid TEXT NOT NULL,
    instance_uid TEXT NOT NULL,
    instance_name TEXT NOT NULL,
    initial_intent JSONB,
    intent_hash TEXT NOT NULL DEFAULT '',
    assigned_ports JSONB NOT NULL DEFAULT '[]'::JSONB CHECK(jsonb_typeof(assigned_ports)='array' AND jsonb_array_length(assigned_ports)<=32),
    state TEXT NOT NULL CHECK (state IN ('Pending','Active','Closing','Closed')),
    current_cpu BIGINT NOT NULL DEFAULT 0 CHECK(current_cpu>=0),
    current_mem BIGINT NOT NULL DEFAULT 0 CHECK(current_mem>=0),
    current_storage BIGINT NOT NULL DEFAULT 0 CHECK(current_storage>=0),
    current_instances BIGINT NOT NULL DEFAULT 0 CHECK(current_instances>=0),
    current_backups BIGINT NOT NULL DEFAULT 0 CHECK(current_backups=0),
    current_backup_storage BIGINT NOT NULL DEFAULT 0 CHECK(current_backup_storage=0),
    version BIGINT NOT NULL DEFAULT 1 CHECK(version>0),
    resource_revision BIGINT NOT NULL DEFAULT 0 CHECK(resource_revision>=0),
    close_intent_id TEXT NOT NULL DEFAULT '',
    change_plan_id TEXT NOT NULL DEFAULT '',
    change_revision BIGINT NOT NULL DEFAULT 0 CHECK(change_revision>=0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(node_uid,allocation_id),
    CHECK ((initial_intent IS NULL AND intent_hash='') OR (initial_intent IS NOT NULL AND length(intent_hash)=64)),
    CHECK (state NOT IN ('Pending','Active') OR length(intent_hash)=64),
    CHECK ((state IN ('Pending','Active') AND close_intent_id='') OR (state IN ('Closing','Closed') AND close_intent_id<>'')),
    CHECK (state NOT IN ('Pending','Closed') OR (current_cpu=0 AND current_mem=0 AND current_storage=0 AND current_instances=0 AND assigned_ports='[]'::JSONB)),
    CHECK ((change_plan_id='' AND change_revision=0) OR (state IN ('Active','Closing') AND change_plan_id<>'' AND change_revision>resource_revision))
);
CREATE INDEX IF NOT EXISTS idx_node_reservations_owner ON node_reservations(tenant_uid,instance_uid);
CREATE INDEX IF NOT EXISTS idx_node_reservations_pending ON node_reservations(state,updated_at,allocation_id) WHERE state IN ('Pending','Closing');

CREATE TABLE IF NOT EXISTS node_ports (
    node_uid TEXT NOT NULL,
    protocol TEXT NOT NULL CHECK(protocol IN ('TCP','UDP')),
    published_port INTEGER NOT NULL CHECK(published_port BETWEEN 1 AND 65535),
    attempt_id TEXT NOT NULL,
    PRIMARY KEY(node_uid,protocol,published_port),
    FOREIGN KEY(node_uid,attempt_id) REFERENCES node_reservations(node_uid,allocation_id)
);
CREATE INDEX IF NOT EXISTS idx_node_ports_attempt ON node_ports(node_uid,attempt_id);

CREATE TABLE IF NOT EXISTS node_changes (
    change_id TEXT PRIMARY KEY,
    allocation_id TEXT NOT NULL REFERENCES node_reservations(allocation_id),
    request_hash TEXT NOT NULL CHECK(length(request_hash)=64),
    plan JSONB NOT NULL,
    source_version BIGINT NOT NULL CHECK(source_version>0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_node_changes_allocation ON node_changes(allocation_id);

CREATE TABLE IF NOT EXISTS node_operations (
    operation_id TEXT PRIMARY KEY,
    allocation_id TEXT NOT NULL REFERENCES node_reservations(allocation_id),
    kind TEXT NOT NULL CHECK(kind IN ('Grant','Increase','SettleDecrease','RequestClose','FinalizeClose')),
    request_hash TEXT NOT NULL CHECK(length(request_hash)=64),
    command JSONB NOT NULL,
    change_plan_id TEXT NOT NULL DEFAULT '',
    close_intent_id TEXT NOT NULL DEFAULT '',
    state TEXT NOT NULL DEFAULT 'Accepted' CHECK(state IN ('Accepted','Committed','Cancelled')),
    result JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_node_operations_grant ON node_operations(allocation_id) WHERE kind='Grant';
CREATE UNIQUE INDEX IF NOT EXISTS uq_node_operations_change ON node_operations(allocation_id,kind,change_plan_id) WHERE kind IN ('Increase','SettleDecrease');
CREATE UNIQUE INDEX IF NOT EXISTS uq_node_operations_close ON node_operations(allocation_id,kind,close_intent_id) WHERE kind IN ('RequestClose','FinalizeClose');
CREATE INDEX IF NOT EXISTS idx_node_operations_accepted ON node_operations(state,updated_at,operation_id) WHERE state='Accepted';

CREATE TABLE IF NOT EXISTS backup_instance_gates (
    instance_uid TEXT PRIMARY KEY,
    closed BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE TABLE IF NOT EXISTS backup_commands (
    operation_id TEXT PRIMARY KEY,
    instance_uid TEXT NOT NULL,
    object_key TEXT NOT NULL,
    request_hash TEXT NOT NULL CHECK (length(request_hash) = 64),
    command JSONB NOT NULL,
    phase TEXT NOT NULL CHECK (phase IN ('PendingQuota','Ready','Uploading','Unknown','Completed','Deleting','Deleted')),
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    result JSONB,
    upload_expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(instance_uid, object_key),
    CHECK (phase <> 'Completed' OR result IS NOT NULL),
    CHECK (phase NOT IN ('Uploading','Unknown') OR upload_expires_at IS NOT NULL)
);
CREATE INDEX IF NOT EXISTS idx_backup_commands_instance ON backup_commands(instance_uid, operation_id);
CREATE INDEX IF NOT EXISTS idx_backup_commands_recovery ON backup_commands(phase, updated_at) WHERE phase NOT IN ('Completed','Deleted');

CREATE TABLE IF NOT EXISTS registry_upstreams (
    name                  TEXT        PRIMARY KEY,
    uid                   TEXT        NOT NULL CONSTRAINT uni_registry_upstreams_uid UNIQUE,
    provider              TEXT        NOT NULL,
    endpoint              TEXT        NOT NULL DEFAULT '',
    proxy_url             TEXT        NOT NULL DEFAULT '',
    steam_app_id          INTEGER     NOT NULL DEFAULT 0,
    modrinth_loader       TEXT        NOT NULL DEFAULT '',
    modrinth_project_type TEXT        NOT NULL DEFAULT '',
    api_key               TEXT        NOT NULL DEFAULT '',
    rate_limit_rps        INTEGER     NOT NULL DEFAULT 0,
    resource_version      BIGINT      NOT NULL DEFAULT 1 CHECK (resource_version > 0),
    generation            BIGINT      NOT NULL DEFAULT 1 CHECK (generation > 0),
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at            TIMESTAMPTZ NULL,
    finalizers            JSONB       NOT NULL DEFAULT '[]'::jsonb,
    labels                JSONB       NOT NULL DEFAULT '{}'::jsonb,
    annotations           JSONB       NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX IF NOT EXISTS idx_registry_upstreams_labels ON registry_upstreams USING GIN (labels);

CREATE TABLE IF NOT EXISTS registry_repositories (
    name              TEXT        PRIMARY KEY,
    uid               TEXT        NOT NULL CONSTRAINT uni_registry_repositories_uid UNIQUE,
    display_name      TEXT        NOT NULL DEFAULT '',
    description       TEXT        NOT NULL DEFAULT '',
    mode              TEXT        NOT NULL DEFAULT 'Hosted',
    format            TEXT        NOT NULL DEFAULT 'generic',
    upstream_name     TEXT        NOT NULL DEFAULT '',
    member_repos      JSONB       NOT NULL DEFAULT '[]'::jsonb,
    public_read       BOOLEAN     NOT NULL DEFAULT FALSE,
    retention_policy  JSONB       NULL,
    resource_version  BIGINT      NOT NULL DEFAULT 1 CHECK (resource_version > 0),
    generation        BIGINT      NOT NULL DEFAULT 1 CHECK (generation > 0),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at        TIMESTAMPTZ NULL,
    finalizers        JSONB       NOT NULL DEFAULT '[]'::jsonb,
    labels            JSONB       NOT NULL DEFAULT '{}'::jsonb,
    annotations       JSONB       NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX IF NOT EXISTS idx_registry_repositories_labels ON registry_repositories USING GIN (labels);

CREATE TABLE IF NOT EXISTS registry_repository_states (
    repository_uid      TEXT        PRIMARY KEY REFERENCES registry_repositories(uid) ON DELETE CASCADE,
    version             BIGINT      NOT NULL DEFAULT 1 CHECK (version > 0),
    observed_generation BIGINT      NOT NULL DEFAULT 0,
    phase               TEXT        NOT NULL DEFAULT 'Active',
    package_count       BIGINT      NOT NULL DEFAULT 0,
    artifact_count      BIGINT      NOT NULL DEFAULT 0,
    storage_bytes       BIGINT      NOT NULL DEFAULT 0,
    last_synced_at      TIMESTAMPTZ NULL,
    conditions          JSONB       NOT NULL DEFAULT '[]'::jsonb,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_registry_repository_states_phase ON registry_repository_states (phase);

CREATE TABLE IF NOT EXISTS registry_packages (
    name              TEXT        PRIMARY KEY,
    uid               TEXT        NOT NULL CONSTRAINT uni_registry_packages_uid UNIQUE,
    repository        TEXT        NOT NULL,
    package           TEXT        NOT NULL,
    title             TEXT        NOT NULL,
    summary           TEXT        NOT NULL DEFAULT '',
    description       TEXT        NOT NULL DEFAULT '',
    author            TEXT        NOT NULL DEFAULT '',
    icon_url          TEXT        NOT NULL DEFAULT '',
    gallery_urls      JSONB       NOT NULL DEFAULT '[]'::jsonb,
    source_url        TEXT        NOT NULL DEFAULT '',
    downloads         BIGINT      NOT NULL DEFAULT 0,
    size_bytes        BIGINT      NOT NULL DEFAULT 0,
    latest_version    TEXT        NOT NULL DEFAULT '',
    updated_at_str    TEXT        NOT NULL DEFAULT '',
    tags              JSONB       NOT NULL DEFAULT '[]'::jsonb,
    mod_ids           JSONB       NOT NULL DEFAULT '[]'::jsonb,
    map_folders       JSONB       NOT NULL DEFAULT '[]'::jsonb,
    dependencies      JSONB       NOT NULL DEFAULT '[]'::jsonb,
    config_schema     JSONB       NOT NULL DEFAULT '[]'::jsonb,
    resource_version  BIGINT      NOT NULL DEFAULT 1 CHECK (resource_version > 0),
    generation        BIGINT      NOT NULL DEFAULT 1 CHECK (generation > 0),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at        TIMESTAMPTZ NULL,
    finalizers        JSONB       NOT NULL DEFAULT '[]'::jsonb,
    labels            JSONB       NOT NULL DEFAULT '{}'::jsonb,
    annotations       JSONB       NOT NULL DEFAULT '{}'::jsonb,
    CONSTRAINT idx_registry_packages_repo_pkg UNIQUE (repository, package)
);
CREATE INDEX IF NOT EXISTS idx_registry_packages_labels ON registry_packages USING GIN (labels);

CREATE TABLE IF NOT EXISTS registry_artifacts (
    name              TEXT        PRIMARY KEY,
    uid               TEXT        NOT NULL CONSTRAINT uni_registry_artifacts_uid UNIQUE,
    repository        TEXT        NOT NULL,
    package           TEXT        NOT NULL,
    version           TEXT        NOT NULL,
    source_url        TEXT        NOT NULL DEFAULT '',
    archive_format    TEXT        NOT NULL DEFAULT 'raw',
    target_path       TEXT        NOT NULL DEFAULT '',
    game_versions     JSONB       NOT NULL DEFAULT '[]'::jsonb,
    loaders           JSONB       NOT NULL DEFAULT '[]'::jsonb,
    spec_descriptor   JSONB       NULL,
    resource_version  BIGINT      NOT NULL DEFAULT 1 CHECK (resource_version > 0),
    generation        BIGINT      NOT NULL DEFAULT 1 CHECK (generation > 0),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at        TIMESTAMPTZ NULL,
    finalizers        JSONB       NOT NULL DEFAULT '[]'::jsonb,
    labels            JSONB       NOT NULL DEFAULT '{}'::jsonb,
    annotations       JSONB       NOT NULL DEFAULT '{}'::jsonb,
    CONSTRAINT idx_registry_artifacts_coord UNIQUE (repository, package, version)
);
CREATE INDEX IF NOT EXISTS idx_registry_artifacts_labels ON registry_artifacts USING GIN (labels);

CREATE TABLE IF NOT EXISTS registry_artifact_states (
    artifact_uid        TEXT        PRIMARY KEY REFERENCES registry_artifacts(uid) ON DELETE CASCADE,
    version             BIGINT      NOT NULL DEFAULT 1 CHECK (version > 0),
    observed_generation BIGINT      NOT NULL DEFAULT 0,
    phase               TEXT        NOT NULL DEFAULT 'Pending',
    stage               TEXT        NOT NULL DEFAULT '',
    progress            INTEGER     NOT NULL DEFAULT 0,
    processed_bytes     BIGINT      NOT NULL DEFAULT 0,
    total_bytes         BIGINT      NOT NULL DEFAULT 0,
    speed_bytes_per_sec BIGINT      NOT NULL DEFAULT 0,
    started_at          TIMESTAMPTZ NULL,
    build               TEXT        NOT NULL DEFAULT '',
    descriptor          JSONB       NULL,
    blob_digest         TEXT        NOT NULL DEFAULT '',
    mod_ids             JSONB       NOT NULL DEFAULT '[]'::jsonb,
    map_folders         JSONB       NOT NULL DEFAULT '[]'::jsonb,
    dependencies        JSONB       NOT NULL DEFAULT '[]'::jsonb,
    config_schema       JSONB       NOT NULL DEFAULT '[]'::jsonb,
    published_at        TIMESTAMPTZ NULL,
    changelog           TEXT        NOT NULL DEFAULT '',
    upstream_url        TEXT        NOT NULL DEFAULT '',
    synced_at           TIMESTAMPTZ NULL,
    message             TEXT        NOT NULL DEFAULT '',
    conditions          JSONB       NOT NULL DEFAULT '[]'::jsonb,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_registry_artifact_states_phase ON registry_artifact_states (phase);
CREATE INDEX IF NOT EXISTS idx_registry_artifact_states_digest ON registry_artifact_states (blob_digest);

CREATE TABLE IF NOT EXISTS registry_blobs (
    name              TEXT        PRIMARY KEY,
    uid               TEXT        NOT NULL CONSTRAINT uni_registry_blobs_uid UNIQUE,
    digest            TEXT        NOT NULL CONSTRAINT uni_registry_blobs_digest UNIQUE,
    size_bytes        BIGINT      NOT NULL DEFAULT 0 CHECK (size_bytes >= 0),
    media_type        TEXT        NOT NULL DEFAULT 'application/octet-stream',
    object_key        TEXT        NOT NULL,
    ref_count         BIGINT      NOT NULL DEFAULT 0 CHECK (ref_count >= 0),
    resource_version  BIGINT      NOT NULL DEFAULT 1 CHECK (resource_version > 0),
    generation        BIGINT      NOT NULL DEFAULT 1 CHECK (generation > 0),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at        TIMESTAMPTZ NULL,
    finalizers        JSONB       NOT NULL DEFAULT '[]'::jsonb,
    labels            JSONB       NOT NULL DEFAULT '{}'::jsonb,
    annotations       JSONB       NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX IF NOT EXISTS idx_registry_blobs_refcount ON registry_blobs (ref_count, created_at);

CREATE TABLE IF NOT EXISTS registry_blob_refs (
    digest            TEXT        NOT NULL REFERENCES registry_blobs(digest) ON DELETE CASCADE,
    holder_uid        TEXT        NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (digest, holder_uid)
);

CREATE TABLE IF NOT EXISTS registry_blob_uploads (
    name              TEXT        PRIMARY KEY,
    uid               TEXT        NOT NULL CONSTRAINT uni_registry_blob_uploads_uid UNIQUE,
    digest            TEXT        NOT NULL DEFAULT '',
    size_bytes        BIGINT      NOT NULL DEFAULT 0 CHECK (size_bytes >= 0),
    media_type        TEXT        NOT NULL DEFAULT 'application/octet-stream',
    file_name         TEXT        NOT NULL DEFAULT '',
    object_key        TEXT        NOT NULL DEFAULT '',
    upload_id         TEXT        NOT NULL DEFAULT '',
    part_size         BIGINT      NOT NULL DEFAULT 0,
    parts             JSONB       NOT NULL DEFAULT '[]'::jsonb,
    presigned_url     TEXT        NOT NULL DEFAULT '',
    expires_at        TIMESTAMPTZ NULL,
    upload_state      TEXT        NOT NULL DEFAULT 'Uploading',
    resource_version  BIGINT      NOT NULL DEFAULT 1 CHECK (resource_version > 0),
    generation        BIGINT      NOT NULL DEFAULT 1 CHECK (generation > 0),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at        TIMESTAMPTZ NULL,
    finalizers        JSONB       NOT NULL DEFAULT '[]'::jsonb,
    labels            JSONB       NOT NULL DEFAULT '{}'::jsonb,
    annotations       JSONB       NOT NULL DEFAULT '{}'::jsonb
);

CREATE TABLE IF NOT EXISTS registry_replication_policies (
    name              TEXT        PRIMARY KEY,
    uid               TEXT        NOT NULL CONSTRAINT uni_registry_rep_policies_uid UNIQUE,
    repository        TEXT        NOT NULL,
    enabled           BOOLEAN     NOT NULL DEFAULT TRUE,
    mode              TEXT        NOT NULL DEFAULT 'TrackReleases',
    interval_minutes  INTEGER     NOT NULL DEFAULT 60,
    top_n             INTEGER     NOT NULL DEFAULT 0,
    packages          JSONB       NOT NULL DEFAULT '[]'::jsonb,
    game_version      TEXT        NOT NULL DEFAULT '',
    last_run_at       TIMESTAMPTZ NULL,
    last_message      TEXT        NOT NULL DEFAULT '',
    resource_version  BIGINT      NOT NULL DEFAULT 1 CHECK (resource_version > 0),
    generation        BIGINT      NOT NULL DEFAULT 1 CHECK (generation > 0),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at        TIMESTAMPTZ NULL,
    finalizers        JSONB       NOT NULL DEFAULT '[]'::jsonb,
    labels            JSONB       NOT NULL DEFAULT '{}'::jsonb,
    annotations       JSONB       NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX IF NOT EXISTS idx_registry_rep_policies_repo ON registry_replication_policies (repository);
`
