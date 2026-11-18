package tests

import (
	"context"
	"strings"
	"testing"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/apiserver"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/models/core/prewarmjob"
	pkgmodel "github.com/smartcat999/game-panel-cloud/pkg/models/registry/pkg"
	appdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/app"
	prewarmjobdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/prewarmjob"
	tenantdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/tenant"
	artifactdb "github.com/smartcat999/game-panel-cloud/pkg/repo/registry/artifact"
	pkgdb "github.com/smartcat999/game-panel-cloud/pkg/repo/registry/pkg"
	"github.com/smartcat999/game-panel-cloud/tests/testpg"
)

func TestSyncDefaultAppPrewarmJobs(t *testing.T) {
	ctx := context.Background()
	reg := testpg.NewRegistry(t)
	if err := apiserver.Bootstrap(ctx, reg, "admin", "admin@example.com", "P@88w0rd"); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	jobDB := prewarmjobdb.NewDB(reg.Pool())
	// Seed legacy app-versions and app-addons jobs that must always be cleaned up.
	legacyVer := &corev1.PrewarmJob{
		Metadata: metav1.ObjectMeta{
			Name:   "auto-ver-terraria-vanilla-1-4-5-8",
			Labels: map[string]string{corev1.LabelManagedBy: corev1.ManagedByLegacyAppVersions},
		},
		Spec: corev1.PrewarmJobSpec{Images: []string{"alpine:latest"}},
	}
	legacyAddon := &corev1.PrewarmJob{
		Metadata: metav1.ObjectMeta{
			Name:   "auto-addons-dst",
			Labels: map[string]string{corev1.LabelManagedBy: corev1.ManagedByLegacyAppAddons},
		},
		Spec: corev1.PrewarmJobSpec{Images: []string{"alpine:latest"}},
	}
	if _, err := jobDB.Create(ctx, legacyVer); err != nil {
		t.Fatalf("create legacy version job: %v", err)
	}
	if _, err := jobDB.Create(ctx, legacyAddon); err != nil {
		t.Fatalf("create legacy addon job: %v", err)
	}

	// Initially auto-prewarm is off -> legacy jobs removed, no auto-default job created.
	if err := prewarmjob.SyncDefaultAppPrewarmJobs(ctx, reg, nil); err != nil {
		t.Fatalf("SyncDefaultAppPrewarmJobs (disabled): %v", err)
	}
	list, err := jobDB.List(ctx, db.ListOptions{})
	if err != nil || len(list.Items) != 0 {
		t.Fatalf("expected 0 prewarm jobs when disabled, got %d (err=%v)", len(list.Items), err)
	}

	// Enable auto-prewarm-defaults on admin tenant.
	tdb := tenantdb.NewDB(reg.Pool())
	adminTenant, err := tdb.Get(ctx, "admin")
	if err != nil {
		t.Fatalf("get admin tenant: %v", err)
	}
	if adminTenant.Metadata.Annotations == nil {
		adminTenant.Metadata.Annotations = make(map[string]string)
	}
	adminTenant.Metadata.Annotations[corev1.AnnotationAutoPrewarmDefaults] = "true"
	if _, err := tdb.Update(ctx, adminTenant); err != nil {
		t.Fatalf("enable auto-prewarm on admin tenant: %v", err)
	}

	if err := prewarmjob.SyncDefaultAppPrewarmJobs(ctx, reg, nil); err != nil {
		t.Fatalf("SyncDefaultAppPrewarmJobs (enabled): %v", err)
	}
	adb := appdb.NewDB(reg.Pool())
	dstApp, err := adb.Get(ctx, "dont-starve-together")
	if err != nil {
		t.Fatalf("get dont-starve-together app: %v", err)
	}
	dstJob, err := jobDB.Get(ctx, "auto-default-dont-starve-together")
	if err != nil {
		t.Fatalf("expected auto-default-dont-starve-together job: %v", err)
	}
	if dstJob.Metadata.Labels[corev1.LabelManagedBy] != corev1.ManagedByAutoDefault {
		t.Fatalf("unexpected managed-by label: %v", dstJob.Metadata.Labels)
	}
	if len(dstJob.Spec.Artifacts) != 1 || dstJob.Spec.Artifacts[0].Version != dstApp.RecommendedVersion {
		t.Fatalf("expected default artifact version %q, got %+v", dstApp.RecommendedVersion, dstJob.Spec.Artifacts)
	}

	// Updating RecommendedVersion updates the managed PrewarmJob spec.
	dstApp.RecommendedVersion = "custom-v2"
	if _, err := adb.Update(ctx, dstApp); err != nil {
		t.Fatalf("update dont-starve-together recommendedVersion: %v", err)
	}
	if err := prewarmjob.SyncDefaultAppPrewarmJobs(ctx, reg, nil); err != nil {
		t.Fatalf("SyncDefaultAppPrewarmJobs (after version update): %v", err)
	}
	dstJob, err = jobDB.Get(ctx, "auto-default-dont-starve-together")
	if err != nil || len(dstJob.Spec.Artifacts) != 1 || dstJob.Spec.Artifacts[0].Version != "custom-v2" {
		t.Fatalf("expected updated artifact version custom-v2, got %+v err=%v", dstJob, err)
	}

	// Disabling auto-prewarm-defaults removes auto-default jobs.
	adminTenant, _ = tdb.Get(ctx, "admin")
	adminTenant.Metadata.Annotations[corev1.AnnotationAutoPrewarmDefaults] = "false"
	if _, err := tdb.Update(ctx, adminTenant); err != nil {
		t.Fatalf("disable auto-prewarm on admin tenant: %v", err)
	}
	if err := prewarmjob.SyncDefaultAppPrewarmJobs(ctx, reg, nil); err != nil {
		t.Fatalf("SyncDefaultAppPrewarmJobs (re-disabled): %v", err)
	}
	list, err = jobDB.List(ctx, db.ListOptions{})
	if err != nil || len(list.Items) != 0 {
		t.Fatalf("expected 0 prewarm jobs after disabling, got %d (err=%v)", len(list.Items), err)
	}
}

func TestClusterCachedPackageIconNormalization(t *testing.T) {
	ctx := context.Background()
	reg := testpg.NewRegistry(t)
	if err := apiserver.Bootstrap(ctx, reg, "admin", "admin@example.com", "P@88w0rd"); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	pdb := pkgdb.NewDB(reg.Pool())
	createdPkg, err := pdb.Create(ctx, &registryv1.Package{
		Repository:    "dst-workshop",
		Package:       "1207269058",
		Title:         "Simple Health Bar DST",
		IconURL:       "https://images.steamusercontent.com/ugc/example/icon.png",
		LatestVersion: "latest",
	})
	if err != nil {
		t.Fatalf("create package: %v", err)
	}
	_ = createdPkg

	adb := artifactdb.NewDB(reg.Pool())
	if _, err := adb.Create(ctx, &registryv1.Artifact{
		Metadata: metav1.ObjectMeta{Name: "dst-workshop.1207269058.latest"},
		Spec: registryv1.ArtifactSpec{
			Repository: "dst-workshop",
			Package:    "1207269058",
			Version:    "latest",
		},
	}); err != nil {
		t.Fatalf("create artifact: %v", err)
	}

	op := pkgmodel.NewOperator(reg, newMemTestCache())
	list, err := op.List(ctx, db.ListOptions{
		FieldSelector: map[string]string{"cached": "true"},
		Limit:         100,
	})
	if err != nil {
		t.Fatalf("cluster List(cached=true): %v", err)
	}
	var found *registryv1.Package
	for _, p := range list.Items {
		if p != nil && p.Repository == "dst-workshop" && p.Package == "1207269058" {
			found = p
			break
		}
	}
	if found == nil {
		t.Fatalf("expected dst-workshop/1207269058 in cluster package list")
	}
	wantPrefix := "/apis/registry.gamepanel.io/v1/packages/dst-workshop.catalog/icon?u="
	if !strings.HasPrefix(found.IconURL, wantPrefix) {
		t.Fatalf("expected signed icon URL starting with %q, got %q", wantPrefix, found.IconURL)
	}
}
