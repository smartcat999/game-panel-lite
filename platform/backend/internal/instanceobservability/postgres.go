package instanceobservability

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/messaging"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/persistence"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/providercontract"
)

var ErrInvalidObservation = errors.New("invalid instance observation")

type ProviderRegistry interface {
	Verified(context.Context, string) (providercontract.Manifest, error)
}

type Postgres struct {
	database  *sql.DB
	providers ProviderRegistry
}

func NewPostgres(database *sql.DB, providers ProviderRegistry) *Postgres {
	return &Postgres{database: database, providers: providers}
}

func (p *Postgres) Handle(ctx context.Context, observation Observation) (bool, error) {
	if observation.MessageID == "" || observation.WorkspaceID == "" || observation.LogicalInstanceID == "" || observation.RegionID == "" || observation.RuntimeAttemptID == "" || observation.FencingToken < 1 || observation.Sequence < 1 || observation.ObservedAt.IsZero() || len(observation.Logs) > 500 || len(observation.Metrics) > 1000 {
		return false, ErrInvalidObservation
	}
	for _, entry := range observation.Logs {
		if entry.ID == "" || len(entry.Message) > 16384 || entry.ObservedAt.IsZero() || entry.Stream != "stdout" && entry.Stream != "stderr" && entry.Stream != "console" && entry.Stream != "system" {
			return false, ErrInvalidObservation
		}
	}
	for _, sample := range observation.Metrics {
		if sample.ID == "" || sample.Metric == "" || sample.Unit == "" || sample.SampledAt.IsZero() || sample.Source != "platform" && sample.Source != "provider-api" && sample.Source != "log-parser" || sample.Confidence != nil && (*sample.Confidence < 0 || *sample.Confidence > 1) {
			return false, ErrInvalidObservation
		}
	}
	return messaging.NewPostgres(p.database).HandleOnce(ctx, contract.EventID(observation.MessageID), "instance.telemetry.observed.v1", observation.ObservedAt, func(query persistence.DBTX) error {
		var workspaceID, regionID string
		var sequence int64
		if err := query.QueryRowContext(ctx, `SELECT workspace_id,region_id,telemetry_sequence FROM managed_instances WHERE id=$1`, observation.LogicalInstanceID).Scan(&workspaceID, &regionID, &sequence); err != nil {
			return err
		}
		if workspaceID != observation.WorkspaceID || regionID != observation.RegionID {
			return ErrInvalidObservation
		}
		if observation.Sequence <= sequence {
			return nil
		}
		logsToStore := observation.Logs
		if logsToStore == nil {
			logsToStore = []LogEntry{}
		}
		logs, err := json.Marshal(logsToStore)
		if err != nil {
			return err
		}
		metricsToStore := observation.Metrics
		if metricsToStore == nil {
			metricsToStore = []MetricSample{}
		}
		metrics, err := json.Marshal(metricsToStore)
		if err != nil {
			return err
		}
		if _, err := query.ExecContext(ctx, `INSERT INTO instance_log_entries (id,workspace_id,logical_instance_id,runtime_attempt_id,stream,message,observed_at) SELECT item.id,$2,$3,$4,item.stream,item.message,item."observedAt" FROM jsonb_to_recordset($1::jsonb) AS item(id text,stream text,message text,"observedAt" timestamptz) ON CONFLICT (id) DO NOTHING`, logs, observation.WorkspaceID, observation.LogicalInstanceID, observation.RuntimeAttemptID); err != nil {
			return err
		}
		if _, err := query.ExecContext(ctx, `INSERT INTO instance_metric_samples (id,workspace_id,logical_instance_id,runtime_attempt_id,metric,value,unit,source,confidence,fresh_until,sampled_at) SELECT item.id,$2,$3,$4,item.metric,item.value,item.unit,item.source,item.confidence,item."freshUntil",item."sampledAt" FROM jsonb_to_recordset($1::jsonb) AS item(id text,metric text,value double precision,unit text,source text,confidence double precision,"freshUntil" timestamptz,"sampledAt" timestamptz) ON CONFLICT (id) DO NOTHING`, metrics, observation.WorkspaceID, observation.LogicalInstanceID, observation.RuntimeAttemptID); err != nil {
			return err
		}
		_, err = query.ExecContext(ctx, `UPDATE managed_instances SET telemetry_sequence=$2 WHERE id=$1 AND telemetry_sequence<$2`, observation.LogicalInstanceID, observation.Sequence)
		return err
	})
}

func (p *Postgres) Logs(ctx context.Context, workspaceID, instanceID string, limit int) ([]LogEntry, error) {
	if limit < 1 || limit > 500 {
		limit = 500
	}
	rows, err := p.database.QueryContext(ctx, `SELECT id,logical_instance_id,COALESCE(runtime_attempt_id,''),stream,message,observed_at FROM instance_log_entries WHERE workspace_id=$1 AND logical_instance_id=$2 ORDER BY observed_at DESC,id LIMIT $3`, workspaceID, instanceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]LogEntry, 0)
	for rows.Next() {
		var entry LogEntry
		if err := rows.Scan(&entry.ID, &entry.LogicalInstanceID, &entry.RuntimeAttemptID, &entry.Stream, &entry.Message, &entry.ObservedAt); err != nil {
			return nil, err
		}
		result = append(result, entry)
	}
	return result, rows.Err()
}

func (p *Postgres) Metrics(ctx context.Context, workspaceID, instanceID string, now time.Time, limit int) ([]MetricSample, error) {
	if limit < 1 || limit > 1000 {
		limit = 1000
	}
	var providerReleaseID string
	if err := p.database.QueryRowContext(ctx, `SELECT provider_release_id FROM managed_instances WHERE id=$1 AND workspace_id=$2`, instanceID, workspaceID).Scan(&providerReleaseID); err != nil {
		return nil, err
	}
	manifest, err := p.providers.Verified(ctx, providerReleaseID)
	if err != nil {
		return nil, err
	}
	declared := make(map[string]providercontract.Metric, len(manifest.Metrics))
	for _, metric := range manifest.Metrics {
		declared[metric.Key] = metric
	}
	rows, err := p.database.QueryContext(ctx, `SELECT id,logical_instance_id,COALESCE(runtime_attempt_id,''),metric,value,unit,source,confidence,fresh_until,sampled_at FROM instance_metric_samples WHERE workspace_id=$1 AND logical_instance_id=$2 ORDER BY sampled_at DESC,id LIMIT $3`, workspaceID, instanceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]MetricSample, 0)
	for rows.Next() {
		var sample MetricSample
		var confidence sql.NullFloat64
		var freshUntil sql.NullTime
		if err := rows.Scan(&sample.ID, &sample.LogicalInstanceID, &sample.RuntimeAttemptID, &sample.Metric, &sample.Value, &sample.Unit, &sample.Source, &confidence, &freshUntil, &sample.SampledAt); err != nil {
			return nil, err
		}
		if confidence.Valid {
			sample.Confidence = &confidence.Float64
		}
		if freshUntil.Valid {
			sample.FreshUntil = &freshUntil.Time
		}
		if sample.Source != "platform" {
			definition, ok := declared[sample.Metric]
			if !ok || sample.Source != definition.Source || sample.Confidence == nil || *sample.Confidence < definition.MinimumConfidence || sample.FreshUntil == nil || !sample.FreshUntil.After(now) {
				continue
			}
		}
		result = append(result, sample)
	}
	return result, rows.Err()
}
