package repository

import (
	"context"
	"errors"
	"slices"
	"time"

	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
)

func (o *operator) Reconcile(ctx context.Context, name string) error {
	repoObj, err := o.db.Get(ctx, name)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil
		}
		return err
	}
	if repoObj.Metadata.DeletionTimestamp != nil {
		return o.reconcileDelete(ctx, repoObj)
	}
	return o.reconcileActive(ctx, repoObj)
}

func (o *operator) reconcileDelete(ctx context.Context, repoObj *registryv1.Repository) error {
	if err := o.cleanupRepositoryChildren(ctx, repoObj.Metadata.Name); err != nil {
		return err
	}
	nextFin := slices.DeleteFunc(append([]string(nil), repoObj.Metadata.Finalizers...), func(f string) bool {
		return f == registryv1.FinalizerRepositoryCleanup
	})
	if len(nextFin) != len(repoObj.Metadata.Finalizers) {
		repoObj.Metadata.Finalizers = nextFin
		updated, err := o.db.UpdateObject(ctx, repoObj)
		if err != nil {
			return err
		}
		repoObj = updated
	}
	if len(repoObj.Metadata.Finalizers) > 0 {
		return nil
	}
	if err := o.db.DeleteIfUID(ctx, repoObj.Metadata.Name, repoObj.Metadata.UID); err != nil && !errors.Is(err, db.ErrNotFound) {
		return err
	}
	return nil
}

func (o *operator) cleanupRepositoryChildren(ctx context.Context, repoName string) error {
	policies, err := o.policyDB.List(ctx, db.ListOptions{FieldSelector: map[string]string{"repository": repoName}})
	if err != nil {
		return err
	}
	for _, p := range policies.Items {
		if err := o.policyDB.Delete(ctx, p.Metadata.Name); err != nil && !errors.Is(err, db.ErrNotFound) {
			return err
		}
	}
	arts, err := o.artDB.List(ctx, db.ListOptions{FieldSelector: map[string]string{"repository": repoName}})
	if err != nil {
		return err
	}
	for _, a := range arts.Items {
		if bound := a.Metadata.Annotations[registryv1.AnnotationBoundBlobDigest]; bound != "" {
			if _, err := o.blobDB.ReleaseRef(ctx, bound, a.Metadata.UID); err != nil && !errors.Is(err, db.ErrNotFound) {
				return err
			}
		}
		if err := o.artDB.DeleteIfUID(ctx, a.Metadata.Name, a.Metadata.UID); err != nil && !errors.Is(err, db.ErrNotFound) {
			return err
		}
	}
	pkgs, err := o.pkgDB.List(ctx, db.ListOptions{FieldSelector: map[string]string{"repository": repoName}})
	if err != nil {
		return err
	}
	for _, p := range pkgs.Items {
		if err := o.pkgDB.Delete(ctx, p.Metadata.Name); err != nil && !errors.Is(err, db.ErrNotFound) {
			return err
		}
	}
	return nil
}

func (o *operator) reconcileActive(ctx context.Context, repoObj *registryv1.Repository) error {
	pkgCount, err := o.pkgDB.CountByRepository(ctx, repoObj.Metadata.Name)
	if err != nil {
		return err
	}
	artCount, storageBytes, err := o.artDB.SummaryByRepository(ctx, repoObj.Metadata.Name)
	if err != nil {
		return err
	}
	phase := registryv1.RepositoryPhaseActive
	condStatus := metav1.ConditionTrue
	reason := registryv1.ConditionReasonReady
	msg := "Repository is active"
	if repoObj.Spec.Mode == registryv1.RepositoryModeProxy {
		if _, uErr := o.upstreamDB.Get(ctx, repoObj.Spec.UpstreamName); uErr != nil {
			phase = registryv1.RepositoryPhaseDegraded
			condStatus = metav1.ConditionFalse
			reason = registryv1.ConditionReasonUpstreamDown
			msg = "Referenced upstream is unavailable"
		}
	}
	if repoObj.Status.ObservedGeneration == repoObj.Metadata.Generation &&
		repoObj.Status.Phase == phase &&
		repoObj.Status.PackageCount == pkgCount &&
		repoObj.Status.ArtifactCount == artCount &&
		repoObj.Status.StorageBytes == storageBytes {
		return nil
	}
	repoObj.Status.ObservedGeneration = repoObj.Metadata.Generation
	repoObj.Status.Phase = phase
	repoObj.Status.PackageCount = pkgCount
	repoObj.Status.ArtifactCount = artCount
	repoObj.Status.StorageBytes = storageBytes
	repoObj.Status.Conditions = []metav1.Condition{{
		Type:               metav1.ConditionTypeReady,
		Status:             condStatus,
		ObservedGeneration: repoObj.Metadata.Generation,
		LastTransitionTime: time.Now().UTC(),
		Reason:             reason,
		Message:            msg,
	}}
	updated, err := o.db.UpdateStatus(ctx, repoObj)
	if err != nil {
		return err
	}
	o.publish(ctx, mq.EventStateChanged, updated)
	return nil
}
