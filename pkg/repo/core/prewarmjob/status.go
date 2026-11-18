package prewarmjob

import (
	"context"
	"errors"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/repo/pgutil"
)

// UpdateStatus updates only prewarm_job_states without touching prewarm_jobs or resource_version.
func (s *DB) UpdateStatus(ctx context.Context, in *corev1.PrewarmJob) (*corev1.PrewarmJob, error) {
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
	oldSV, _ := pgutil.ParseVersion(existing.Metadata.StatusVersion)
	return s.commitStatusUpdate(ctx, existing.Metadata.UID, existing.Metadata.Name, &cp, oldSV, oldSV+1)
}

func (s *DB) commitStatusUpdate(ctx context.Context, uid, name string, cp *corev1.PrewarmJob, oldSV, newSV int64) (*corev1.PrewarmJob, error) {
	_, st := toJobRecords(cp, 1, newSV)
	nodesJSON, _ := pgutil.MarshalJSONCol(st.Nodes)
	condsJSON, _ := pgutil.MarshalJSONCol(st.Conditions)
	tx := s.orm.WithContext(ctx).Model(&corev1.PrewarmJobStateRecord{}).
		Where("job_uid = ? AND version = ?", uid, oldSV)
	res := tx.Updates(map[string]any{
		"version":             newSV,
		"observed_generation": st.ObservedGeneration,
		"phase":               st.Phase,
		"target_nodes":        st.TargetNodes,
		"ready_nodes":         st.ReadyNodes,
		"failed_nodes":        st.FailedNodes,
		"nodes":               nodesJSON,
		"conditions":          condsJSON,
		"updated_at":          time.Now().UTC(),
	})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, db.ErrConflict
	}
	return s.Get(ctx, name)
}

// ReportNodeStatus atomically merges a single node's prewarm status using optimistic StatusVersion CAS.
func (s *DB) ReportNodeStatus(ctx context.Context, jobName string, entry corev1.PrewarmNodeStatus) (*corev1.PrewarmJob, error) {
	if s == nil || s.orm == nil {
		return nil, errors.New("db is nil")
	}
	for range 8 {
		cur, err := s.Get(ctx, jobName)
		if err != nil {
			return nil, err
		}
		mergeSingleNodeStatus(&cur.Status, entry)
		saved, err := s.UpdateStatus(ctx, cur)
		if err == nil {
			return saved, nil
		}
		if !errors.Is(err, db.ErrConflict) {
			return nil, err
		}
	}
	return nil, db.ErrConflict
}

func mergeSingleNodeStatus(st *corev1.PrewarmJobStatus, entry corev1.PrewarmNodeStatus) {
	found := false
	for i := range st.Nodes {
		if st.Nodes[i].NodeName == entry.NodeName {
			st.Nodes[i] = entry
			found = true
			break
		}
	}
	if !found {
		st.Nodes = append(st.Nodes, entry)
	}
	recomputeJobSummary(st)
}

// RecomputeSummary recalculates TargetNodes, ReadyNodes, FailedNodes, and Phase from Status.Nodes.
func RecomputeSummary(st *corev1.PrewarmJobStatus) {
	recomputeJobSummary(st)
}

func recomputeJobSummary(st *corev1.PrewarmJobStatus) {
	st.TargetNodes = int32(len(st.Nodes))
	var ready, failed, running int32
	for _, n := range st.Nodes {
		switch n.Phase {
		case corev1.PrewarmJobPhaseSucceeded:
			ready++
		case corev1.PrewarmJobPhaseFailed:
			failed++
		case corev1.PrewarmJobPhaseRunning:
			running++
		}
	}
	st.ReadyNodes = ready
	st.FailedNodes = failed
	switch {
	case st.TargetNodes == 0:
		st.Phase = corev1.PrewarmJobPhasePending
	case ready == st.TargetNodes:
		st.Phase = corev1.PrewarmJobPhaseSucceeded
	case ready+failed == st.TargetNodes && failed > 0:
		st.Phase = corev1.PrewarmJobPhaseFailed
	case running > 0 || ready > 0 || failed > 0:
		st.Phase = corev1.PrewarmJobPhaseRunning
	default:
		st.Phase = corev1.PrewarmJobPhasePending
	}
}
