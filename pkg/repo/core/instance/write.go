package instance

import (
	"context"
	"errors"
	"reflect"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/repo/pgutil"
)

func desiredSpecWithoutNode(spec corev1.InstanceSpec) corev1.InstanceSpec {
	spec.NodeName = ""
	return spec
}

// Update persists Spec and Metadata changes while preserving server-owned ResourceControl & ControlConditions.
func (s *DB) Update(ctx context.Context, in *corev1.Instance) (*corev1.Instance, error) {
	return s.updateInstanceRow(ctx, in, false)
}

// UpdateObject persists Spec, Metadata, ResourceControl, and ControlConditions with strict UID + ResourceVersion CAS.
func (s *DB) UpdateObject(ctx context.Context, in *corev1.Instance) (*corev1.Instance, error) {
	return s.updateInstanceRow(ctx, in, true)
}

func (s *DB) updateInstanceRow(ctx context.Context, in *corev1.Instance, includeControl bool) (*corev1.Instance, error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	if includeControl && (in.Metadata.UID == "" || in.Metadata.ResourceVersion == "") {
		return nil, db.ErrConflict
	}
	cp := *in
	cp.DefaultMeta()
	if err := cp.Validate(); err != nil {
		return nil, err
	}
	existing, err := s.Get(ctx, cp.Metadata.Tenant, cp.Metadata.Name)
	if err != nil {
		return nil, err
	}
	if (cp.Metadata.UID != "" && cp.Metadata.UID != existing.Metadata.UID) ||
		(cp.Metadata.ResourceVersion != "" && cp.Metadata.ResourceVersion != existing.Metadata.ResourceVersion) {
		return nil, db.ErrConflict
	}
	if !includeControl {
		cp.Status = existing.Status
	}
	oldRV, _ := pgutil.ParseVersion(existing.Metadata.ResourceVersion)
	cp.Metadata.UID = existing.Metadata.UID
	cp.Metadata.CreationTimestamp = existing.Metadata.CreationTimestamp
	cp.Metadata.Generation = existing.Metadata.Generation
	if !reflect.DeepEqual(desiredSpecWithoutNode(existing.Spec), desiredSpecWithoutNode(cp.Spec)) {
		cp.Metadata.Generation++
	}
	return s.commitInstanceRowUpdate(ctx, &cp, oldRV, oldRV+1, includeControl || in.Metadata.ResourceVersion != "")
}

func marshalOptionalJSON(v any, nonNil bool) any {
	if !nonNil {
		return nil
	}
	raw, _ := pgutil.MarshalJSONCol(v)
	return raw
}

func instanceRowUpdateColumns(rec corev1.InstanceRecord, newRV int64) map[string]any {
	nodeSelJSON, _ := pgutil.MarshalJSONCol(rec.NodeSelector)
	cmdJSON, _ := pgutil.MarshalJSONCol(rec.Command)
	argsJSON, _ := pgutil.MarshalJSONCol(rec.Args)
	addonsJSON, _ := pgutil.MarshalJSONCol(rec.Addons)
	resJSON, _ := pgutil.MarshalJSONCol(rec.Resources)
	portsJSON, _ := pgutil.MarshalJSONCol(rec.Ports)
	envJSON, _ := pgutil.MarshalJSONCol(rec.Env)
	filesJSON, _ := pgutil.MarshalJSONCol(rec.Files)
	schedJSON, _ := pgutil.MarshalJSONCol(rec.Schedules)
	metricsJSON, _ := pgutil.MarshalJSONCol(rec.Metrics)
	ctrlCondsJSON, _ := pgutil.MarshalJSONCol(rec.ControlConditions)
	finJSON, _ := pgutil.MarshalJSONCol(rec.Finalizers)
	lblJSON, _ := pgutil.MarshalJSONCol(rec.Labels)
	annJSON, _ := pgutil.MarshalJSONCol(rec.Annotations)
	return map[string]any{
		"runtime_class_name": rec.RuntimeClassName, "state": rec.State, "node_name": rec.NodeName,
		"node_selector": nodeSelJSON, "image": rec.Image, "command": cmdJSON, "args": argsJSON,
		"working_dir":     rec.WorkingDir,
		"server_artifact": marshalOptionalJSON(rec.ServerArtifact, rec.ServerArtifact != nil),
		"addons":          addonsJSON,
		"resources":       resJSON, "ports": portsJSON, "env": envJSON,
		"files": filesJSON, "backup_policy": marshalOptionalJSON(rec.BackupPolicy, rec.BackupPolicy != nil),
		"schedules": schedJSON, "metrics": metricsJSON, "control_stage": rec.ControlStage,
		"resource_control":   marshalOptionalJSON(rec.ResourceControl, rec.ResourceControl != nil),
		"control_conditions": ctrlCondsJSON, "resource_version": newRV, "generation": rec.Generation,
		"deleted_at": rec.DeletedAt, "finalizers": finJSON, "labels": lblJSON, "annotations": annJSON,
	}
}

func (s *DB) commitInstanceRowUpdate(ctx context.Context, cp *corev1.Instance, oldRV, newRV int64, checkRV bool) (*corev1.Instance, error) {
	rec := toInstanceRecord(cp, newRV)
	tx := s.orm.WithContext(ctx).Model(&corev1.InstanceRecord{}).
		Where("tenant = ? AND name = ? AND uid = ?", rec.Tenant, rec.Name, rec.UID)
	if checkRV {
		tx = tx.Where("resource_version = ?", oldRV)
	}
	res := tx.Updates(instanceRowUpdateColumns(rec, newRV))
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, db.ErrConflict
	}
	return s.Get(ctx, rec.Tenant, rec.Name)
}
