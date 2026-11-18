package prewarmjob

import (
	"context"
	"fmt"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	"github.com/smartcat999/game-panel-cloud/pkg/models"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	appdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/app"
	nodedb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/node"
	prewarmjobdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/prewarmjob"
)

// Operator defines business operations on cluster-scoped PrewarmJob resources.
type Operator interface {
	Create(ctx context.Context, in *corev1.PrewarmJob) (*corev1.PrewarmJob, error)
	Get(ctx context.Context, name string) (*corev1.PrewarmJob, error)
	List(ctx context.Context, opts db.ListOptions) (*metav1.List[*corev1.PrewarmJob], error)
	Update(ctx context.Context, name string, in *corev1.PrewarmJob) (*corev1.PrewarmJob, error)
	UpdateStatus(ctx context.Context, name string, in *corev1.PrewarmJob) (*corev1.PrewarmJob, error)
	ReportNodeStatus(ctx context.Context, name, nodeName string, entry corev1.PrewarmNodeStatus) (*corev1.PrewarmJob, error)
	Delete(ctx context.Context, name string) error
}

type operator struct {
	jobDB  *prewarmjobdb.DB
	nodeDB *nodedb.DB
	appDB  *appdb.DB
	mq     mq.Interface
}

func NewOperator(reg *repo.Registry, bus mq.Interface) Operator {
	return &operator{
		jobDB:  prewarmjobdb.NewDB(reg.Pool()),
		nodeDB: nodedb.NewDB(reg.Pool()),
		appDB:  appdb.NewDB(reg.Pool()),
		mq:     bus,
	}
}

func (o *operator) Create(ctx context.Context, in *corev1.PrewarmJob) (*corev1.PrewarmJob, error) {
	in.DefaultMeta()
	if err := in.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", models.ErrBadRequest, err)
	}
	created, err := o.jobDB.Create(ctx, in)
	if err != nil {
		return nil, err
	}
	o.publish(ctx, mq.EventCreated, created)
	return created, nil
}

func (o *operator) Get(ctx context.Context, name string) (*corev1.PrewarmJob, error) {
	out, err := o.jobDB.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (o *operator) List(ctx context.Context, opts db.ListOptions) (*metav1.List[*corev1.PrewarmJob], error) {
	out, err := o.jobDB.List(ctx, opts)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (o *operator) Update(ctx context.Context, name string, in *corev1.PrewarmJob) (*corev1.PrewarmJob, error) {
	in.Metadata.Name = name
	in.DefaultMeta()
	if err := in.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", models.ErrBadRequest, err)
	}
	updated, err := o.jobDB.Update(ctx, in)
	if err != nil {
		return nil, err
	}
	o.publish(ctx, mq.EventUpdated, updated)
	return updated, nil
}

func (o *operator) UpdateStatus(ctx context.Context, name string, in *corev1.PrewarmJob) (*corev1.PrewarmJob, error) {
	in.Metadata.Name = name
	prewarmjobdb.RecomputeSummary(&in.Status)
	updated, err := o.jobDB.UpdateStatus(ctx, in)
	if err != nil {
		return nil, err
	}
	o.publish(ctx, mq.EventStateChanged, updated)
	return updated, nil
}

func (o *operator) ReportNodeStatus(ctx context.Context, name, nodeName string, entry corev1.PrewarmNodeStatus) (*corev1.PrewarmJob, error) {
	cur, err := o.jobDB.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	allowed, err := o.nodeTargeted(ctx, cur, nodeName)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, fmt.Errorf("%w: node %s is not targeted by prewarmjob %s", models.ErrForbidden, nodeName, name)
	}
	entry.NodeName = nodeName
	updated, err := o.jobDB.ReportNodeStatus(ctx, name, entry)
	if err != nil {
		return nil, err
	}
	o.publish(ctx, mq.EventStateChanged, updated)
	return updated, nil
}

func (o *operator) nodeTargeted(ctx context.Context, job *corev1.PrewarmJob, nodeName string) (bool, error) {
	for _, n := range job.Status.Nodes {
		if n.NodeName == nodeName {
			return true, nil
		}
	}
	nodeObj, err := o.nodeDB.Get(ctx, nodeName)
	if err != nil {
		return false, err
	}
	var repoToApp map[string]string
	if o.appDB != nil {
		if apps, aErr := o.appDB.List(ctx, db.ListOptions{SkipTotalCount: true}); aErr == nil && apps != nil {
			repoToApp = BuildRepoToAppMap(apps.Items)
		}
	}
	return NodeMatchesJob(nodeObj, job, repoToApp), nil
}

func (o *operator) Delete(ctx context.Context, name string) error {
	existing, err := o.jobDB.Get(ctx, name)
	if err != nil {
		return err
	}
	if err := o.jobDB.Delete(ctx, name); err != nil {
		return err
	}
	o.publish(ctx, mq.EventDeleted, existing)
	return nil
}

func (o *operator) publish(ctx context.Context, evType mq.EventType, obj *corev1.PrewarmJob) {
	if o.mq == nil || obj == nil {
		return
	}
	ev := mq.NewEvent(evType, corev1.GroupVersion, corev1.KindPrewarmJob, "", obj.Metadata.Name, obj.Metadata.Generation, obj.Metadata.ResourceVersion)
	ev.Topic = mq.TopicPrewarmJob
	_ = o.mq.Publish(ctx, mq.TopicPrewarmJob, ev)
}
