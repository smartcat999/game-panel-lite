package agent

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/client"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/log"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/runtime"
)

const prewarmPollInterval = 15 * time.Second

func (a *Agent) runPrewarmLoop(ctx context.Context) {
	if strings.TrimSpace(a.cfg.APIServerURL) == "" {
		return
	}
	_ = a.RunPrewarmOnce(ctx)
	ticker := time.NewTicker(prewarmPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := a.RunPrewarmOnce(ctx); err != nil && client.IsUnauthorized(err) {
				a.resetAPIClient()
			}
		}
	}
}

// RunPrewarmOnce executes any pending PrewarmJobs targeting this node and warms published App runtime images.
func (a *Agent) RunPrewarmOnce(ctx context.Context) error {
	cli, err := a.apiClient()
	if err != nil {
		return err
	}
	node := a.fetchLocalNode(ctx, cli)
	apps := a.fetchAppsList(ctx, cli)
	a.warmPublishedAppImages(ctx, node, apps)

	didWork := a.syncAllowedNodeCache(ctx, cli, node, apps)

	listCtx, cancel := a.operationContext(ctx)
	jobs, err := cli.CoreV1().PrewarmJobs().List(listCtx, client.ListOptions{})
	cancel()
	if err != nil {
		if didWork {
			_ = a.reportHeartbeat(ctx)
		}
		return err
	}
	if a.executeMatchingPrewarmJobs(ctx, cli, node, jobs, buildAgentRepoToAppMap(apps)) {
		didWork = true
	}
	if didWork {
		_ = a.reportHeartbeat(ctx)
	}
	return nil
}

func (a *Agent) executeMatchingPrewarmJobs(
	ctx context.Context,
	cli client.Interface,
	node *corev1.Node,
	jobs *metav1.List[corev1.PrewarmJob],
	repoToApp map[string]string,
) bool {
	if jobs == nil {
		return false
	}
	cachedList, _ := CollectNodeBlobCacheInventory(a.cfg.WorkDir)
	cachedArtifacts := make(map[string]struct{}, len(cachedList))
	for _, coord := range cachedList {
		cachedArtifacts[coord] = struct{}{}
	}
	didWork := false
	for i := range jobs.Items {
		job := &jobs.Items[i]
		if !nodeShouldRunPrewarmJob(a.cfg.NodeName, node, job, repoToApp, cachedArtifacts) {
			continue
		}
		didWork = true
		a.executeNodePrewarmJob(ctx, cli, job)
	}
	return didWork
}

func (a *Agent) fetchLocalNode(ctx context.Context, cli client.Interface) *corev1.Node {
	if cli == nil || strings.TrimSpace(a.cfg.NodeName) == "" {
		return nil
	}
	opCtx, cancel := a.operationContext(ctx)
	defer cancel()
	node, err := cli.CoreV1().Nodes().Get(opCtx, a.cfg.NodeName)
	if err != nil {
		return nil
	}
	return node
}

func (a *Agent) fetchAppsList(ctx context.Context, cli client.Interface) []corev1.App {
	if cli == nil {
		return nil
	}
	listCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	list, err := cli.CoreV1().Apps().List(listCtx, client.ListOptions{})
	if err != nil || list == nil {
		return nil
	}
	return list.Items
}

func (a *Agent) syncAllowedNodeCache(ctx context.Context, cli client.Interface, node *corev1.Node, apps []corev1.App) bool {
	if node == nil || len(node.Spec.AllowedApps) == 0 || len(apps) == 0 {
		return false
	}
	allowedRepos := buildAllowedReposMap(node, apps)
	if len(allowedRepos) == 0 {
		return false
	}
	activeCoords, ok := a.activeInstanceArtifactCoords(ctx, cli)
	if !ok {
		return false
	}
	return PruneDisallowedNodeBlobCache(a.cfg.WorkDir, allowedRepos, activeCoords)
}

func (a *Agent) activeInstanceArtifactCoords(ctx context.Context, cli client.Interface) (map[string]struct{}, bool) {
	if cli == nil || strings.TrimSpace(a.cfg.NodeName) == "" {
		return nil, false
	}
	listCtx, cancel := a.operationContext(ctx)
	defer cancel()
	list, err := cli.CoreV1().Instances("").List(listCtx, client.ListOptions{NodeName: a.cfg.NodeName})
	if err != nil || list == nil {
		return nil, false
	}
	coords := make(map[string]struct{})
	for i := range list.Items {
		inst := &list.Items[i]
		if inst.Spec.ServerArtifact != nil {
			if c := inst.Spec.ServerArtifact.Coordinate(); c != "" {
				coords[c] = struct{}{}
			}
		}
		for _, addon := range inst.Spec.Addons {
			if c := addon.Coordinate(); c != "" {
				coords[c] = struct{}{}
			}
		}
	}
	return coords, true
}

func buildAllowedReposMap(node *corev1.Node, apps []corev1.App) map[string]struct{} {
	allowed := make(map[string]struct{}, len(apps)*2)
	for i := range apps {
		app := &apps[i]
		if !node.AllowsApp(app.Metadata.Name) {
			continue
		}
		if app.ServerBinding != nil {
			if repo := strings.TrimSpace(app.ServerBinding.Repository); repo != "" {
				allowed[repo] = struct{}{}
			}
		}
		if app.AddonBinding != nil {
			if repo := strings.TrimSpace(app.AddonBinding.Repository); repo != "" {
				allowed[repo] = struct{}{}
			}
		}
	}
	return allowed
}

func buildAgentRepoToAppMap(apps []corev1.App) map[string]string {
	out := make(map[string]string, len(apps)*2)
	for i := range apps {
		app := &apps[i]
		if app.ServerBinding != nil {
			if repo := strings.TrimSpace(app.ServerBinding.Repository); repo != "" {
				out[repo] = app.Metadata.Name
			}
		}
		if app.AddonBinding != nil {
			if repo := strings.TrimSpace(app.AddonBinding.Repository); repo != "" {
				if _, exists := out[repo]; !exists {
					out[repo] = app.Metadata.Name
				}
			}
		}
	}
	return out
}

func jobAllowedOnNode(node *corev1.Node, job *corev1.PrewarmJob, repoToApp map[string]string) bool {
	if job == nil || node == nil || len(node.Spec.AllowedApps) == 0 {
		return true
	}
	if !node.AllowsApp(job.Metadata.Labels[corev1.LabelInstanceApp]) {
		return false
	}
	if len(repoToApp) > 0 {
		for _, art := range job.Spec.Artifacts {
			if appName := repoToApp[strings.TrimSpace(art.Repository)]; appName != "" && !node.AllowsApp(appName) {
				return false
			}
		}
	}
	return true
}

func nodeShouldRunPrewarmJob(
	nodeName string,
	node *corev1.Node,
	job *corev1.PrewarmJob,
	repoToApp map[string]string,
	cachedArtifacts map[string]struct{},
) bool {
	return jobAllowedOnNode(node, job, repoToApp) && nodeNeedsPrewarmExecution(nodeName, job, cachedArtifacts)
}

func nodeNeedsPrewarmExecution(nodeName string, job *corev1.PrewarmJob, cachedArtifacts map[string]struct{}) bool {
	if job == nil || nodeName == "" {
		return false
	}
	for _, entry := range job.Status.Nodes {
		if entry.NodeName != nodeName {
			continue
		}
		if entry.Phase == "" || entry.Phase == corev1.PrewarmJobPhasePending || entry.Phase == corev1.PrewarmJobPhaseRunning {
			return true
		}
		if entry.Phase == corev1.PrewarmJobPhaseSucceeded {
			return missingPrewarmArtifact(job.Spec.Artifacts, cachedArtifacts)
		}
		return false
	}
	return false
}

func missingPrewarmArtifact(artifacts []corev1.ArtifactRef, cached map[string]struct{}) bool {
	for i := range artifacts {
		coord := artifacts[i].Coordinate()
		if coord == "" {
			continue
		}
		if _, ok := cached[coord]; !ok {
			return true
		}
	}
	return false
}

func (a *Agent) executeNodePrewarmJob(ctx context.Context, cli client.Interface, job *corev1.PrewarmJob) {
	a.reportPrewarmNodeStatus(ctx, cli, job.Metadata.Name, corev1.PrewarmNodeStatus{
		NodeName: a.cfg.NodeName,
		Phase:    corev1.PrewarmJobPhaseRunning,
		Message:  "正在预热镜像与制品",
	})

	for _, img := range job.Spec.Images {
		img = strings.TrimSpace(img)
		if img == "" {
			continue
		}
		if err := a.prePullSingleImage(ctx, img); err != nil {
			now := time.Now().UTC()
			a.reportPrewarmNodeStatus(ctx, cli, job.Metadata.Name, corev1.PrewarmNodeStatus{
				NodeName:    a.cfg.NodeName,
				Phase:       corev1.PrewarmJobPhaseFailed,
				Message:     fmt.Sprintf("pull image %s: %v", img, err),
				CompletedAt: &now,
			})
			return
		}
	}

	for _, ref := range job.Spec.Artifacts {
		coord := ref.Coordinate()
		if coord == "" {
			continue
		}
		syncMsg, _, err := EnsureArtifactCached(ctx, a.cfg.WorkDir, cli, coord)
		if err != nil {
			now := time.Now().UTC()
			a.reportPrewarmNodeStatus(ctx, cli, job.Metadata.Name, corev1.PrewarmNodeStatus{
				NodeName:    a.cfg.NodeName,
				Phase:       corev1.PrewarmJobPhaseFailed,
				Message:     err.Error(),
				CompletedAt: &now,
			})
			return
		}
		if syncMsg != "" {
			a.reportPrewarmNodeStatus(ctx, cli, job.Metadata.Name, corev1.PrewarmNodeStatus{
				NodeName: a.cfg.NodeName,
				Phase:    corev1.PrewarmJobPhaseRunning,
				Message:  syncMsg,
			})
			return
		}
	}

	now := time.Now().UTC()
	a.reportPrewarmNodeStatus(ctx, cli, job.Metadata.Name, corev1.PrewarmNodeStatus{
		NodeName:    a.cfg.NodeName,
		Phase:       corev1.PrewarmJobPhaseSucceeded,
		Message:     "预热完成",
		CompletedAt: &now,
	})
}

func (a *Agent) reportPrewarmNodeStatus(ctx context.Context, cli client.Interface, jobName string, entry corev1.PrewarmNodeStatus) {
	opCtx, cancel := a.operationContext(ctx)
	defer cancel()
	payload := &corev1.PrewarmJob{
		Metadata: metav1.ObjectMeta{Name: jobName},
		Status:   corev1.PrewarmJobStatus{Nodes: []corev1.PrewarmNodeStatus{entry}},
	}
	if _, err := cli.CoreV1().PrewarmJobs().UpdateStatus(opCtx, payload); err != nil {
		log.S().Warnw("failed to report prewarm node status", "job", jobName, "node", a.cfg.NodeName, "error", err)
	}
}

func (a *Agent) prePullSingleImage(ctx context.Context, img string) error {
	if a.drivers == nil {
		return nil
	}
	drv, ok := a.drivers.Get(runtime.HandlerDocker)
	if !ok {
		return nil
	}
	prewarmer, ok := drv.(runtime.ImagePrewarmer)
	if !ok {
		return nil
	}
	pullCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	ready, err := prewarmer.PrePullImage(pullCtx, img)
	if err != nil {
		return err
	}
	if !ready {
		return fmt.Errorf("pull image %s not ready", img)
	}
	a.recordWarmImage(img)
	return nil
}

func (a *Agent) warmPublishedAppImages(ctx context.Context, node *corev1.Node, apps []corev1.App) {
	if a.drivers == nil || len(apps) == 0 {
		return
	}
	drv, ok := a.drivers.Get(runtime.HandlerDocker)
	if !ok {
		return
	}
	if _, ok := drv.(runtime.ImagePrewarmer); !ok {
		return
	}
	seen := make(map[string]bool)
	for i := range apps {
		app := &apps[i]
		if !app.Published || (node != nil && !node.AllowsApp(app.Metadata.Name)) {
			continue
		}
		img := strings.TrimSpace(app.Image)
		if img == "" || seen[img] {
			continue
		}
		seen[img] = true
		_ = a.prePullSingleImage(ctx, img)
	}
}

func (a *Agent) recordWarmImage(img string) {
	img = strings.TrimSpace(img)
	if img == "" {
		return
	}
	a.warmImagesMu.Lock()
	if a.warmImages == nil {
		a.warmImages = make(map[string]bool)
	}
	a.warmImages[img] = true
	a.warmImagesMu.Unlock()
}

func (a *Agent) snapshotWarmImages() []string {
	a.warmImagesMu.RLock()
	defer a.warmImagesMu.RUnlock()
	if len(a.warmImages) == 0 {
		return nil
	}
	out := make([]string, 0, len(a.warmImages))
	for img := range a.warmImages {
		out = append(out, img)
	}
	sort.Strings(out)
	return out
}
