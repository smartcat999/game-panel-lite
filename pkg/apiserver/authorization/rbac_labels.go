package authorization

import (
	"context"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	appdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/app"
	instancedb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/instance"
	nodedb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/node"
	prewarmjobdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/prewarmjob"
	runtimeclassdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/runtimeclass"
	tenantdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/tenant"
	globalroledb "github.com/smartcat999/game-panel-cloud/pkg/repo/iam/globalrole"
	globalrolebindingdb "github.com/smartcat999/game-panel-cloud/pkg/repo/iam/globalrolebinding"
	tenantroledb "github.com/smartcat999/game-panel-cloud/pkg/repo/iam/tenantrole"
	tenantrolebindingdb "github.com/smartcat999/game-panel-cloud/pkg/repo/iam/tenantrolebinding"
	userdb "github.com/smartcat999/game-panel-cloud/pkg/repo/iam/user"
	artifactdb "github.com/smartcat999/game-panel-cloud/pkg/repo/registry/artifact"
	blobdb "github.com/smartcat999/game-panel-cloud/pkg/repo/registry/blob"
	blobuploaddb "github.com/smartcat999/game-panel-cloud/pkg/repo/registry/blobupload"
	pkgdb "github.com/smartcat999/game-panel-cloud/pkg/repo/registry/pkg"
	policydb "github.com/smartcat999/game-panel-cloud/pkg/repo/registry/policy"
	repositorydb "github.com/smartcat999/game-panel-cloud/pkg/repo/registry/repository"
	upstreamdb "github.com/smartcat999/game-panel-cloud/pkg/repo/registry/upstream"
)

func nonNilLabels(labels map[string]string) map[string]string {
	if labels == nil {
		return map[string]string{}
	}
	return labels
}

func (r *RBACAuthorizer) loadTargetLabels(ctx context.Context, a AttributesRecord) map[string]string {
	if r == nil || r.reg == nil || r.reg.Pool() == nil || a.Name == "" || a.Name == "*" {
		return nil
	}
	switch a.APIGroup {
	case iamv1.GroupName:
		return r.loadIAMLabels(ctx, a)
	case corev1.GroupName:
		return r.loadCoreLabels(ctx, a)
	case registryv1.GroupName:
		return r.loadRegistryLabels(ctx, a)
	default:
		return nil
	}
}

func (r *RBACAuthorizer) loadIAMLabels(ctx context.Context, a AttributesRecord) map[string]string {
	pool := r.reg.Pool()
	switch a.Resource {
	case "users":
		if o, err := userdb.NewDB(pool).Get(ctx, a.Name); err == nil && o != nil {
			return nonNilLabels(o.Metadata.Labels)
		}
	case "globalroles":
		if o, err := globalroledb.NewDB(pool).Get(ctx, a.Name); err == nil && o != nil {
			return nonNilLabels(o.Metadata.Labels)
		}
	case "globalrolebindings":
		if o, err := globalrolebindingdb.NewDB(pool).Get(ctx, a.Name); err == nil && o != nil {
			return nonNilLabels(o.Metadata.Labels)
		}
	case "tenantroles":
		if a.Tenant != "" {
			if o, err := tenantroledb.NewDB(pool).Get(ctx, a.Tenant, a.Name); err == nil && o != nil {
				return nonNilLabels(o.Metadata.Labels)
			}
		}
	case "tenantrolebindings":
		if a.Tenant != "" {
			if o, err := tenantrolebindingdb.NewDB(pool).Get(ctx, a.Tenant, a.Name); err == nil && o != nil {
				return nonNilLabels(o.Metadata.Labels)
			}
		}
	}
	return nil
}

func (r *RBACAuthorizer) loadCoreLabels(ctx context.Context, a AttributesRecord) map[string]string {
	pool := r.reg.Pool()
	switch a.Resource {
	case "tenants":
		if o, err := tenantdb.NewDB(pool).Get(ctx, a.Name); err == nil && o != nil {
			return nonNilLabels(o.Metadata.Labels)
		}
	case "runtimeclasses":
		if o, err := runtimeclassdb.NewDB(pool).Get(ctx, a.Name); err == nil && o != nil {
			return nonNilLabels(o.Metadata.Labels)
		}
	case "apps":
		if o, err := appdb.NewDB(pool).Get(ctx, a.Name); err == nil && o != nil {
			return nonNilLabels(o.Metadata.Labels)
		}
	case "nodes":
		if o, err := nodedb.NewDB(pool).Get(ctx, a.Name); err == nil && o != nil {
			return nonNilLabels(o.Metadata.Labels)
		}
	case "prewarmjobs":
		if o, err := prewarmjobdb.NewDB(pool).Get(ctx, a.Name); err == nil && o != nil {
			return nonNilLabels(o.Metadata.Labels)
		}
	case "instances":
		if a.Tenant != "" {
			if o, err := instancedb.NewDB(pool).Get(ctx, a.Tenant, a.Name); err == nil && o != nil {
				return nonNilLabels(o.Metadata.Labels)
			}
		}
	}
	return nil
}

func (r *RBACAuthorizer) loadRegistryLabels(ctx context.Context, a AttributesRecord) map[string]string {
	pool := r.reg.Pool()
	switch a.Resource {
	case "upstreams":
		if o, err := upstreamdb.NewDB(pool).Get(ctx, a.Name); err == nil && o != nil {
			return nonNilLabels(o.Metadata.Labels)
		}
	case "repositories":
		if o, err := repositorydb.NewDB(pool).Get(ctx, a.Name); err == nil && o != nil {
			return nonNilLabels(o.Metadata.Labels)
		}
	case "packages":
		if o, err := pkgdb.NewDB(pool).Get(ctx, a.Name); err == nil && o != nil {
			return nonNilLabels(o.Metadata.Labels)
		}
	case "artifacts":
		if o, err := artifactdb.NewDB(pool).Get(ctx, a.Name); err == nil && o != nil {
			return nonNilLabels(o.Metadata.Labels)
		}
	case "blobs":
		if o, err := blobdb.NewDB(pool).Get(ctx, a.Name); err == nil && o != nil {
			return nonNilLabels(o.Metadata.Labels)
		}
	case "blobuploads":
		if o, err := blobuploaddb.NewDB(pool).Get(ctx, a.Name); err == nil && o != nil {
			return nonNilLabels(o.Metadata.Labels)
		}
	case "replicationpolicies":
		if o, err := policydb.NewDB(pool).Get(ctx, a.Name); err == nil && o != nil {
			return nonNilLabels(o.Metadata.Labels)
		}
	}
	return nil
}
