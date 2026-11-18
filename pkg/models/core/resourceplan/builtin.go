package resourceplan

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"sort"

	"gopkg.in/yaml.v3"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	plandb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/resourceplan"
)

//go:embed manifests/*.yaml
var manifestsFS embed.FS

// BuiltInResourcePlans parses and returns built-in ResourcePlan definitions in sort order.
func BuiltInResourcePlans() []*corev1.ResourcePlan {
	entries, err := fs.ReadDir(manifestsFS, "manifests")
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	plans := make([]*corev1.ResourcePlan, 0, len(names))
	for _, filename := range names {
		if plan, ok := parseBuiltInManifest(filename); ok {
			plans = append(plans, plan)
		}
	}
	return plans
}

func parseBuiltInManifest(filename string) (*corev1.ResourcePlan, bool) {
	rawYAML, err := manifestsFS.ReadFile("manifests/" + filename)
	if err != nil {
		return nil, false
	}
	var doc map[string]any
	if err := yaml.Unmarshal(rawYAML, &doc); err != nil {
		return nil, false
	}
	rawJSON, err := json.Marshal(doc)
	if err != nil {
		return nil, false
	}
	var plan corev1.ResourcePlan
	if err := json.Unmarshal(rawJSON, &plan); err != nil {
		return nil, false
	}
	plan.DefaultMeta()
	sum := sha256.Sum256(rawYAML)
	if plan.Metadata.Annotations == nil {
		plan.Metadata.Annotations = map[string]string{}
	}
	plan.Metadata.Annotations[corev1.AnnotationAppBuiltInHash] = hex.EncodeToString(sum[:])
	return &plan, plan.Validate() == nil
}

// EnsureBuiltInResourcePlans idempotently seeds or refreshes built-in ResourcePlans at startup.
func EnsureBuiltInResourcePlans(ctx context.Context, reg *repo.Registry) error {
	if reg == nil || reg.Pool() == nil {
		return nil
	}
	planDB := plandb.NewDB(reg.Pool())
	for _, p := range BuiltInResourcePlans() {
		if err := upsertBuiltInPlan(ctx, planDB, p); err != nil {
			return err
		}
	}
	return nil
}

func upsertBuiltInPlan(ctx context.Context, planDB *plandb.DB, p *corev1.ResourcePlan) error {
	existing, err := planDB.Get(ctx, p.Metadata.Name)
	if errors.Is(err, db.ErrNotFound) {
		if _, err := planDB.Create(ctx, p); err != nil && !errors.Is(err, db.ErrAlreadyExists) {
			return fmt.Errorf("create built-in resourceplan %s: %w", p.Metadata.Name, err)
		}
		return nil
	}
	if err != nil || existing == nil {
		return err
	}
	wantHash := p.Metadata.Annotations[corev1.AnnotationAppBuiltInHash]
	if existing.Metadata.Annotations[corev1.AnnotationAppBuiltInHash] == wantHash {
		return nil
	}
	_, err = planDB.UpdateObject(ctx, p.Metadata.Name, func(cur *corev1.ResourcePlan) error {
		cur.DisplayName = p.DisplayName
		cur.Description = p.Description
		cur.PlanKind = p.PlanKind
		cur.Quota = p.Quota
		cur.SortOrder = p.SortOrder
		cur.Published = p.Published
		cur.Prices = p.Prices
		if cur.Metadata.Labels == nil {
			cur.Metadata.Labels = map[string]string{}
		}
		for k, v := range p.Metadata.Labels {
			cur.Metadata.Labels[k] = v
		}
		if cur.Metadata.Annotations == nil {
			cur.Metadata.Annotations = map[string]string{}
		}
		cur.Metadata.Annotations[corev1.AnnotationAppBuiltInHash] = wantHash
		return nil
	})
	return err
}
