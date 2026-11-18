package node

import (
	"context"
	"errors"
	"reflect"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/repo/pgutil"
)

func (s *DB) Update(ctx context.Context, in *corev1.Node) (*corev1.Node, error) {
	return s.updateNodeSpec(ctx, in, false)
}

func (s *DB) UpdateObject(ctx context.Context, in *corev1.Node) (*corev1.Node, error) {
	return s.updateNodeSpec(ctx, in, true)
}

func (s *DB) updateNodeSpec(ctx context.Context, in *corev1.Node, requireCAS bool) (*corev1.Node, error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	if requireCAS && (in.Metadata.UID == "" || in.Metadata.ResourceVersion == "") {
		return nil, db.ErrConflict
	}
	cp := *in
	cp.DefaultMeta()
	existing, err := s.Get(ctx, cp.Metadata.Name)
	if err != nil {
		return nil, err
	}
	cp.Status = existing.Status
	if err := cp.Validate(); err != nil {
		return nil, err
	}
	if (cp.Metadata.UID != "" && cp.Metadata.UID != existing.Metadata.UID) ||
		(cp.Metadata.ResourceVersion != "" && cp.Metadata.ResourceVersion != existing.Metadata.ResourceVersion) {
		return nil, db.ErrConflict
	}
	oldRV, _ := pgutil.ParseVersion(existing.Metadata.ResourceVersion)
	newRV := oldRV + 1
	cp.Metadata.UID = existing.Metadata.UID
	cp.Metadata.CreationTimestamp = existing.Metadata.CreationTimestamp
	cp.Metadata.Generation = existing.Metadata.Generation
	if !reflect.DeepEqual(existing.Spec, cp.Spec) {
		cp.Metadata.Generation++
	}
	return s.commitNodeSpecUpdate(ctx, &cp, oldRV, newRV, requireCAS || in.Metadata.ResourceVersion != "")
}

func (s *DB) commitNodeSpecUpdate(ctx context.Context, cp *corev1.Node, oldRV, newRV int64, checkRV bool) (*corev1.Node, error) {
	rec, _ := toNodeRecords(cp, newRV, 1)
	finJSON, _ := pgutil.MarshalJSONCol(rec.Finalizers)
	lblJSON, _ := pgutil.MarshalJSONCol(rec.Labels)
	annJSON, _ := pgutil.MarshalJSONCol(rec.Annotations)
	appsJSON, _ := pgutil.MarshalJSONCol(rec.AllowedApps)
	tx := s.orm.WithContext(ctx).Model(&corev1.NodeRecord{}).Where("name = ? AND uid = ?", rec.Name, rec.UID)
	if checkRV {
		tx = tx.Where("resource_version = ?", oldRV)
	}
	res := tx.Updates(map[string]any{
		"unschedulable": rec.Unschedulable, "external_ip": rec.ExternalIP, "allowed_apps": appsJSON,
		"resource_version": newRV, "generation": rec.Generation, "deleted_at": rec.DeletedAt,
		"finalizers": finJSON, "labels": lblJSON, "annotations": annJSON,
	})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, db.ErrConflict
	}
	return s.Get(ctx, rec.Name)
}

// UpdateStatus updates only node_heartbeats without touching nodes or resource_version.
func (s *DB) UpdateStatus(ctx context.Context, in *corev1.Node) (*corev1.Node, error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	existing, err := s.Get(ctx, in.Metadata.Name)
	if err != nil {
		return nil, err
	}
	if (in.Metadata.UID != "" && in.Metadata.UID != existing.Metadata.UID) ||
		(in.Metadata.StatusVersion != "" && in.Metadata.StatusVersion != existing.Metadata.StatusVersion) {
		return nil, db.ErrConflict
	}
	cp := *existing
	cp.Status = in.Status
	cp.DefaultMeta()
	if err := cp.Validate(); err != nil {
		return nil, err
	}
	oldSV, _ := pgutil.ParseVersion(existing.Metadata.StatusVersion)
	return s.commitHeartbeatUpdate(ctx, existing.Metadata.UID, existing.Metadata.Name, &cp, oldSV, oldSV+1, in.Metadata.StatusVersion != "")
}

func (s *DB) commitHeartbeatUpdate(ctx context.Context, uid, name string, cp *corev1.Node, oldSV, newSV int64, checkSV bool) (*corev1.Node, error) {
	_, hb := toNodeRecords(cp, 1, newSV)
	addrJSON, _ := pgutil.MarshalJSONCol(hb.Addresses)
	rtJSON, _ := pgutil.MarshalJSONCol(hb.Runtimes)
	capJSON, _ := pgutil.MarshalJSONCol(hb.Capacity)
	allocJSON, _ := pgutil.MarshalJSONCol(hb.Allocatable)
	var usageCol any
	if hb.Usage != nil {
		uJSON, _ := pgutil.MarshalJSONCol(hb.Usage)
		usageCol = uJSON
	}
	cachedArtJSON, _ := pgutil.MarshalJSONCol(hb.CachedArtifacts)
	cachedBlobJSON, _ := pgutil.MarshalJSONCol(hb.CachedBlobs)
	warmImgJSON, _ := pgutil.MarshalJSONCol(hb.WarmImages)
	condsJSON, _ := pgutil.MarshalJSONCol(hb.Conditions)
	tx := s.orm.WithContext(ctx).Model(&corev1.NodeHeartbeatRecord{}).Where("node_uid = ?", uid)
	if checkSV {
		tx = tx.Where("version = ?", oldSV)
	}
	res := tx.Updates(map[string]any{
		"node_name": name, "version": newSV, "observed_generation": hb.ObservedGeneration,
		"phase": hb.Phase, "addresses": addrJSON, "runtimes": rtJSON,
		"capacity": capJSON, "allocatable": allocJSON, "usage": usageCol,
		"cached_artifacts": cachedArtJSON, "cached_blobs": cachedBlobJSON, "warm_images": warmImgJSON,
		"last_heartbeat_time": hb.LastHeartbeatTime, "conditions": condsJSON, "updated_at": time.Now().UTC(),
	})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, db.ErrConflict
	}
	return s.Get(ctx, name)
}
