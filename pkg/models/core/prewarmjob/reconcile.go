package prewarmjob

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	appdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/app"
	nodedb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/node"
	prewarmjobdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/prewarmjob"
)

// NodeMatchesSpec reports whether a Node matches PrewarmJobSpec's NodeNames / NodeSelector filters.
// When both NodeNames and NodeSelector are empty, all Ready/registered Nodes match.
func NodeMatchesSpec(n *corev1.Node, spec corev1.PrewarmJobSpec) bool {
	if n == nil || n.Metadata.DeletionTimestamp != nil {
		return false
	}
	if len(spec.NodeNames) > 0 {
		return slices.Contains(spec.NodeNames, n.Metadata.Name)
	}
	if len(spec.NodeSelector) > 0 {
		for k, v := range spec.NodeSelector {
			if n.Metadata.Labels[k] != v {
				return false
			}
		}
	}
	return true
}

// BuildRepoToAppMap maps each App's server and addon repository names to the App's canonical name.
func BuildRepoToAppMap(apps []*corev1.App) map[string]string {
	out := make(map[string]string, len(apps)*2)
	for _, a := range apps {
		if a == nil || a.Metadata.DeletionTimestamp != nil {
			continue
		}
		if a.ServerBinding != nil {
			if repoName := strings.TrimSpace(a.ServerBinding.Repository); repoName != "" {
				out[repoName] = a.Metadata.Name
			}
		}
		if a.AddonBinding != nil {
			if repoName := strings.TrimSpace(a.AddonBinding.Repository); repoName != "" {
				if _, exists := out[repoName]; !exists {
					out[repoName] = a.Metadata.Name
				}
			}
		}
	}
	return out
}

// NodeMatchesJob reports whether a Node matches a PrewarmJob's node filters and spec.allowedApps allowlist.
func NodeMatchesJob(n *corev1.Node, job *corev1.PrewarmJob, repoToApp map[string]string) bool {
	if job == nil || !NodeMatchesSpec(n, job.Spec) {
		return false
	}
	if !n.AllowsApp(job.Metadata.Labels[corev1.LabelInstanceApp]) {
		return false
	}
	if len(n.Spec.AllowedApps) > 0 && len(repoToApp) > 0 {
		for _, art := range job.Spec.Artifacts {
			if appName := repoToApp[strings.TrimSpace(art.Repository)]; appName != "" && !n.AllowsApp(appName) {
				return false
			}
		}
	}
	return true
}

// Reconcile resolves target Nodes into status.nodes and updates aggregate PrewarmJob status.
func Reconcile(ctx context.Context, reg *repo.Registry, bus mq.Interface, name string) error {
	jobDB := prewarmjobdb.NewDB(reg.Pool())
	cur, err := jobDB.Get(ctx, name)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil
		}
		return err
	}
	if cur.Metadata.DeletionTimestamp != nil {
		return nil
	}
	nodesList, err := nodedb.NewDB(reg.Pool()).List(ctx, db.ListOptions{SkipTotalCount: true})
	if err != nil {
		return err
	}
	var repoToApp map[string]string
	if appsList, aErr := appdb.NewDB(reg.Pool()).List(ctx, db.ListOptions{SkipTotalCount: true}); aErr == nil && appsList != nil {
		repoToApp = BuildRepoToAppMap(appsList.Items)
	}
	nextStatus := buildDesiredJobStatus(cur, nodesList.Items, repoToApp)
	if reflect.DeepEqual(cur.Status, nextStatus) {
		return nil
	}
	cur.Status = nextStatus
	saved, err := jobDB.UpdateStatus(ctx, cur)
	if err != nil {
		if errors.Is(err, db.ErrConflict) {
			return nil
		}
		return err
	}
	if bus != nil {
		ev := mq.NewEvent(mq.EventStateChanged, corev1.GroupVersion, corev1.KindPrewarmJob, "", saved.Metadata.Name, saved.Metadata.Generation, saved.Metadata.ResourceVersion)
		ev.Topic = mq.TopicPrewarmJob
		_ = bus.Publish(ctx, mq.TopicPrewarmJob, ev)
	}
	return nil
}

func buildDesiredJobStatus(cur *corev1.PrewarmJob, allNodes []*corev1.Node, repoToApp map[string]string) corev1.PrewarmJobStatus {
	genChanged := cur.Status.ObservedGeneration != 0 && cur.Status.ObservedGeneration != cur.Metadata.Generation
	prevByNode := make(map[string]corev1.PrewarmNodeStatus, len(cur.Status.Nodes))
	if !genChanged {
		for _, ns := range cur.Status.Nodes {
			prevByNode[ns.NodeName] = ns
		}
	}
	var targetNames []string
	for _, n := range allNodes {
		if NodeMatchesJob(n, cur, repoToApp) {
			targetNames = append(targetNames, n.Metadata.Name)
		}
	}
	slices.Sort(targetNames)
	nodes := make([]corev1.PrewarmNodeStatus, 0, len(targetNames))
	for _, name := range targetNames {
		if prev, ok := prevByNode[name]; ok {
			nodes = append(nodes, prev)
		} else {
			nodes = append(nodes, corev1.PrewarmNodeStatus{
				NodeName: name,
				Phase:    corev1.PrewarmJobPhasePending,
			})
		}
	}
	next := cur.Status
	next.ObservedGeneration = cur.Metadata.Generation
	next.Nodes = nodes
	prewarmjobdb.RecomputeSummary(&next)
	setJobReconciledCondition(&next, cur.Metadata.Generation)
	return next
}

func setJobReconciledCondition(st *corev1.PrewarmJobStatus, gen int64) {
	status := metav1.ConditionFalse
	reason := corev1.ReasonPrewarmPending
	msg := fmt.Sprintf("prewarm job targeting %d node(s)", st.TargetNodes)
	switch st.Phase {
	case corev1.PrewarmJobPhaseSucceeded:
		status = metav1.ConditionTrue
		reason = corev1.ReasonPrewarmSucceeded
		msg = fmt.Sprintf("all %d target node(s) warmed successfully", st.ReadyNodes)
	case corev1.PrewarmJobPhaseFailed:
		reason = corev1.ReasonPrewarmFailed
		msg = fmt.Sprintf("%d/%d target node(s) failed prewarm", st.FailedNodes, st.TargetNodes)
	case corev1.PrewarmJobPhaseRunning:
		reason = corev1.ReasonPrewarmRunning
		msg = fmt.Sprintf("%d/%d target node(s) ready", st.ReadyNodes, st.TargetNodes)
	}
	metav1.SetCondition(&st.Conditions, metav1.Condition{
		Type:               metav1.ConditionTypeReconciled,
		Status:             status,
		Reason:             reason,
		Message:            msg,
		ObservedGeneration: gen,
	})
}
