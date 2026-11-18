package prewarmjob

import (
	"context"
	"errors"
	"maps"
	"reflect"
	"strings"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/config"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	appmodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/app"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	appdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/app"
	prewarmjobdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/prewarmjob"
	tenantdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/tenant"
)

// SyncDefaultAppPrewarmJobs reconciles managed PrewarmJobs for published Apps' default versions
// when core.gamepanel.io/auto-prewarm-defaults is enabled on the built-in admin tenant, and
// cleans up any legacy app-versions/app-addons PrewarmJobs.
func SyncDefaultAppPrewarmJobs(ctx context.Context, reg *repo.Registry, bus mq.Interface) error {
	if reg == nil || reg.Pool() == nil {
		return nil
	}
	jobDB := prewarmjobdb.NewDB(reg.Pool())
	jobs, err := jobDB.List(ctx, db.ListOptions{SkipTotalCount: true})
	if err != nil {
		return err
	}
	deleteLegacyManagedJobs(ctx, jobDB, bus, jobs.Items)
	enabled := isAutoPrewarmEnabled(ctx, tenantdb.NewDB(reg.Pool()))
	if !enabled {
		deleteAutoDefaultJobs(ctx, jobDB, bus, jobs.Items, nil)
		return nil
	}
	desired, err := buildDesiredDefaultJobs(ctx, appdb.NewDB(reg.Pool()))
	if err != nil {
		return err
	}
	deleteAutoDefaultJobs(ctx, jobDB, bus, jobs.Items, desired)
	return applyDesiredDefaultJobs(ctx, reg, bus, jobDB, jobs.Items, desired)
}

func isAutoPrewarmEnabled(ctx context.Context, tdb *tenantdb.DB) bool {
	if tdb == nil {
		return false
	}
	adminName := strings.TrimSpace(config.Get().Bootstrap.AdminUser)
	if adminName == "" {
		adminName = "admin"
	}
	t, err := tdb.Get(ctx, adminName)
	if err != nil || t == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(t.Metadata.Annotations[corev1.AnnotationAutoPrewarmDefaults]), "true")
}

func deleteLegacyManagedJobs(ctx context.Context, jobDB *prewarmjobdb.DB, bus mq.Interface, items []*corev1.PrewarmJob) {
	for _, job := range items {
		if job == nil {
			continue
		}
		mb := job.Metadata.Labels[corev1.LabelManagedBy]
		if mb != corev1.ManagedByLegacyAppVersions && mb != corev1.ManagedByLegacyAppAddons {
			continue
		}
		deleteManagedJob(ctx, jobDB, bus, job)
	}
}

func deleteAutoDefaultJobs(
	ctx context.Context,
	jobDB *prewarmjobdb.DB,
	bus mq.Interface,
	items []*corev1.PrewarmJob,
	desired map[string]*corev1.PrewarmJob,
) {
	for _, job := range items {
		if job == nil || job.Metadata.Labels[corev1.LabelManagedBy] != corev1.ManagedByAutoDefault {
			continue
		}
		if desired != nil && desired[job.Metadata.Name] != nil {
			continue
		}
		deleteManagedJob(ctx, jobDB, bus, job)
	}
}

func deleteManagedJob(ctx context.Context, jobDB *prewarmjobdb.DB, bus mq.Interface, job *corev1.PrewarmJob) {
	if err := jobDB.Delete(ctx, job.Metadata.Name); err != nil && !errors.Is(err, db.ErrNotFound) {
		return
	}
	publishJobEvent(ctx, bus, mq.EventDeleted, job)
}

func buildDesiredDefaultJobs(ctx context.Context, adb *appdb.DB) (map[string]*corev1.PrewarmJob, error) {
	apps, err := adb.List(ctx, db.ListOptions{SkipTotalCount: true})
	if err != nil {
		return nil, err
	}
	desired := make(map[string]*corev1.PrewarmJob)
	for _, a := range apps.Items {
		if a == nil || !a.Published || a.Metadata.DeletionTimestamp != nil {
			continue
		}
		if job := desiredJobForApp(a); job != nil {
			desired[job.Metadata.Name] = job
		}
	}
	return desired, nil
}

func desiredJobForApp(a *corev1.App) *corev1.PrewarmJob {
	ver := defaultAppVersion(a)
	var images []string
	if img := resolveDefaultAppImage(a, ver); img != "" {
		images = []string{img}
	}
	var artifacts []corev1.ArtifactRef
	if sb := a.ServerBinding; sb != nil && ver != "" &&
		strings.TrimSpace(sb.Repository) != "" && strings.TrimSpace(sb.Package) != "" {
		artifacts = []corev1.ArtifactRef{{
			Repository: strings.TrimSpace(sb.Repository),
			Package:    strings.TrimSpace(sb.Package),
			Version:    ver,
		}}
	}
	if len(images) == 0 && len(artifacts) == 0 {
		return nil
	}
	job := &corev1.PrewarmJob{
		Metadata: metav1.ObjectMeta{
			Name: autoDefaultJobName(a.Metadata.Name),
			Labels: map[string]string{
				corev1.LabelInstanceApp: a.Metadata.Name,
				corev1.LabelManagedBy:   corev1.ManagedByAutoDefault,
			},
		},
		Spec: corev1.PrewarmJobSpec{
			Images:    images,
			Artifacts: artifacts,
		},
	}
	job.DefaultMeta()
	return job
}

func defaultAppVersion(a *corev1.App) string {
	if rec := strings.TrimSpace(a.RecommendedVersion); rec != "" {
		return rec
	}
	for _, v := range a.Versions {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func resolveDefaultAppImage(a *corev1.App, ver string) string {
	if tpl := strings.TrimSpace(a.ImageTemplate); tpl != "" && ver != "" {
		if rendered, err := appmodel.RenderTemplate(tpl, appSchemaDefaults(a.ConfigSchema), ver); err == nil {
			if trimmed := strings.TrimSpace(rendered); trimmed != "" {
				return trimmed
			}
		}
	}
	return strings.TrimSpace(a.Image)
}

func appSchemaDefaults(fields []corev1.AppConfigField) map[string]string {
	out := make(map[string]string, len(fields))
	for _, f := range fields {
		out[f.Name] = f.Default
	}
	return out
}

func autoDefaultJobName(appName string) string {
	const prefix = "auto-default-"
	const maxLen = 63
	slug := sanitizeDNSLabel(appName)
	if slug == "" {
		slug = "app"
	}
	if len(prefix)+len(slug) > maxLen {
		slug = strings.TrimRight(slug[:maxLen-len(prefix)], "-")
	}
	if slug == "" {
		slug = "app"
	}
	return prefix + slug
}

func sanitizeDNSLabel(raw string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(raw)) {
		isAlnum := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if isAlnum {
			b.WriteRune(r)
			prevDash = false
		} else if !prevDash && b.Len() > 0 {
			b.WriteByte('-')
			prevDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func applyDesiredDefaultJobs(
	ctx context.Context,
	reg *repo.Registry,
	bus mq.Interface,
	jobDB *prewarmjobdb.DB,
	existingItems []*corev1.PrewarmJob,
	desired map[string]*corev1.PrewarmJob,
) error {
	existingByName := make(map[string]*corev1.PrewarmJob, len(existingItems))
	for _, item := range existingItems {
		if item != nil {
			existingByName[item.Metadata.Name] = item
		}
	}
	for name, want := range desired {
		cur := existingByName[name]
		if cur == nil {
			created, err := jobDB.Create(ctx, want)
			if err != nil {
				if errors.Is(err, db.ErrAlreadyExists) {
					continue
				}
				return err
			}
			publishJobEvent(ctx, bus, mq.EventCreated, created)
			_ = Reconcile(ctx, reg, bus, created.Metadata.Name)
			continue
		}
		if reflect.DeepEqual(cur.Spec, want.Spec) && maps.Equal(cur.Metadata.Labels, want.Metadata.Labels) {
			continue
		}
		if err := updateManagedJob(ctx, reg, bus, jobDB, cur, want); err != nil {
			return err
		}
	}
	return nil
}

func updateManagedJob(
	ctx context.Context,
	reg *repo.Registry,
	bus mq.Interface,
	jobDB *prewarmjobdb.DB,
	cur, want *corev1.PrewarmJob,
) error {
	next := *cur
	next.Spec = want.Spec
	nextLabels := maps.Clone(cur.Metadata.Labels)
	if nextLabels == nil {
		nextLabels = make(map[string]string, len(want.Metadata.Labels))
	}
	maps.Copy(nextLabels, want.Metadata.Labels)
	next.Metadata.Labels = nextLabels
	updated, err := jobDB.Update(ctx, &next)
	if err != nil {
		if errors.Is(err, db.ErrConflict) {
			return nil
		}
		return err
	}
	publishJobEvent(ctx, bus, mq.EventUpdated, updated)
	_ = Reconcile(ctx, reg, bus, updated.Metadata.Name)
	return nil
}

func publishJobEvent(ctx context.Context, bus mq.Interface, evType mq.EventType, job *corev1.PrewarmJob) {
	if bus == nil || job == nil {
		return
	}
	ev := mq.NewEvent(evType, corev1.GroupVersion, corev1.KindPrewarmJob, "", job.Metadata.Name, job.Metadata.Generation, job.Metadata.ResourceVersion)
	ev.Topic = mq.TopicPrewarmJob
	_ = bus.Publish(ctx, mq.TopicPrewarmJob, ev)
}
