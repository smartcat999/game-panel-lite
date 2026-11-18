package prewarmjob

import (
	"context"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	ctrlruntime "github.com/smartcat999/game-panel-cloud/pkg/controller/runtime"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	prewarmjobmodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/prewarmjob"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	appdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/app"
	nodedb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/node"
	prewarmjobdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/prewarmjob"
)

func init() {
	ctrlruntime.Register(&Controller{})
}

type Controller struct{}

func (c *Controller) Name() string  { return "07-prewarmjob-lifecycle" }
func (c *Controller) Topic() string { return mq.TopicPrewarmJob }

func (c *Controller) Resync(ctx context.Context, reg *repo.Registry, _ *ctrlruntime.Config) ([]mq.Event, error) {
	_ = prewarmjobmodel.SyncDefaultAppPrewarmJobs(ctx, reg, nil)
	list, err := prewarmjobdb.NewDB(reg.Pool()).List(ctx, db.ListOptions{SkipTotalCount: true})
	if err != nil {
		return nil, err
	}
	nodesList, _ := nodedb.NewDB(reg.Pool()).List(ctx, db.ListOptions{SkipTotalCount: true})
	var allNodes []*corev1.Node
	if nodesList != nil {
		allNodes = nodesList.Items
	}
	var repoToApp map[string]string
	if appsList, aErr := appdb.NewDB(reg.Pool()).List(ctx, db.ListOptions{SkipTotalCount: true}); aErr == nil && appsList != nil {
		repoToApp = prewarmjobmodel.BuildRepoToAppMap(appsList.Items)
	}
	var events []mq.Event
	for _, job := range list.Items {
		if jobNeedsResync(job, allNodes, repoToApp) {
			ev := mq.NewEvent(mq.EventReconcile, corev1.GroupVersion, corev1.KindPrewarmJob, "", job.Metadata.Name, job.Metadata.Generation, job.Metadata.ResourceVersion)
			ev.Topic = mq.TopicPrewarmJob
			events = append(events, ev)
		}
	}
	return events, nil
}

func jobNeedsResync(job *corev1.PrewarmJob, allNodes []*corev1.Node, repoToApp ...map[string]string) bool {
	if job == nil || job.Metadata.DeletionTimestamp != nil {
		return false
	}
	if job.Status.ObservedGeneration != job.Metadata.Generation ||
		job.Status.Phase == corev1.PrewarmJobPhasePending ||
		job.Status.Phase == corev1.PrewarmJobPhaseRunning {
		return true
	}
	var r2a map[string]string
	if len(repoToApp) > 0 {
		r2a = repoToApp[0]
	}
	tracked := make(map[string]bool, len(job.Status.Nodes))
	for _, ns := range job.Status.Nodes {
		tracked[ns.NodeName] = true
	}
	matchedCount := 0
	for _, n := range allNodes {
		if prewarmjobmodel.NodeMatchesJob(n, job, r2a) {
			matchedCount++
			if !tracked[n.Metadata.Name] {
				return true
			}
		}
	}
	return matchedCount != len(job.Status.Nodes)
}

func (c *Controller) Reconcile(ctx context.Context, reg *repo.Registry, bus mq.Interface, _ *ctrlruntime.Config, event mq.Event) error {
	if event.Name == corev1.PrewarmSyncDefaultsName {
		return prewarmjobmodel.SyncDefaultAppPrewarmJobs(ctx, reg, bus)
	}
	if event.Name == "" || event.Type == mq.EventDeleted {
		return nil
	}
	return prewarmjobmodel.Reconcile(ctx, reg, bus, event.Name)
}
