package client

import (
	"errors"
	"fmt"
	"strings"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
)

// ResourceInfo describes the schema, naming, scoping, and instantiators for a declarative resource.
type ResourceInfo struct {
	Kind         string
	GroupVersion string
	Plural       string
	Singular     string
	Aliases      []string
	TenantScoped bool
	NewObject    func() metav1.Object
	NewList      func() any
}

// Registry contains all supported declarative API resources.
var Registry = []*ResourceInfo{
	{
		Kind:         "Instance",
		GroupVersion: corev1.GroupVersion,
		Plural:       "instances",
		Singular:     "instance",
		Aliases:      []string{"inst"},
		TenantScoped: true,
		NewObject:    func() metav1.Object { return &corev1.Instance{} },
		NewList:      func() any { return &metav1.List[corev1.Instance]{} },
	},
	{
		Kind:         "Tenant",
		GroupVersion: corev1.GroupVersion,
		Plural:       "tenants",
		Singular:     "tenant",
		Aliases:      []string{"tn", "ten"},
		TenantScoped: false,
		NewObject:    func() metav1.Object { return &corev1.Tenant{} },
		NewList:      func() any { return &metav1.List[corev1.Tenant]{} },
	},
	{
		Kind:         "Node",
		GroupVersion: corev1.GroupVersion,
		Plural:       "nodes",
		Singular:     "node",
		Aliases:      []string{"no"},
		TenantScoped: false,
		NewObject:    func() metav1.Object { return &corev1.Node{} },
		NewList:      func() any { return &metav1.List[corev1.Node]{} },
	},
	{
		Kind:         "RuntimeClass",
		GroupVersion: corev1.GroupVersion,
		Plural:       "runtimeclasses",
		Singular:     "runtimeclass",
		Aliases:      []string{"rc"},
		TenantScoped: false,
		NewObject:    func() metav1.Object { return &corev1.RuntimeClass{} },
		NewList:      func() any { return &metav1.List[corev1.RuntimeClass]{} },
	},
	{
		Kind:         corev1.KindResourcePlan,
		GroupVersion: corev1.GroupVersion,
		Plural:       "resourceplans",
		Singular:     "resourceplan",
		Aliases:      []string{"rp", "plans"},
		TenantScoped: false,
		NewObject:    func() metav1.Object { return &corev1.ResourcePlan{} },
		NewList:      func() any { return &metav1.List[corev1.ResourcePlan]{} },
	},
	{
		Kind:         "App",
		GroupVersion: corev1.GroupVersion,
		Plural:       "apps",
		Singular:     "app",
		Aliases:      []string{"application"},
		TenantScoped: false,
		NewObject:    func() metav1.Object { return &corev1.App{} },
		NewList:      func() any { return &metav1.List[corev1.App]{} },
	},
	{
		Kind:         corev1.KindPrewarmJob,
		GroupVersion: corev1.GroupVersion,
		Plural:       "prewarmjobs",
		Singular:     "prewarmjob",
		Aliases:      []string{"pwj"},
		TenantScoped: false,
		NewObject:    func() metav1.Object { return &corev1.PrewarmJob{} },
		NewList:      func() any { return &metav1.List[corev1.PrewarmJob]{} },
	},
	{
		Kind:         "TenantMetering",
		GroupVersion: corev1.GroupVersion,
		Plural:       "metering",
		Singular:     "metering",
		Aliases:      []string{"tm", "tenantmetering"},
		TenantScoped: true,
		NewObject:    func() metav1.Object { return &corev1.TenantMetering{} },
		NewList:      func() any { return &metav1.List[corev1.TenantMetering]{} },
	},
	{
		Kind:         "User",
		GroupVersion: iamv1.GroupVersion,
		Plural:       "users",
		Singular:     "user",
		Aliases:      []string{"usr"},
		TenantScoped: false,
		NewObject:    func() metav1.Object { return &iamv1.User{} },
		NewList:      func() any { return &metav1.List[iamv1.User]{} },
	},
	{
		Kind:         "GlobalRole",
		GroupVersion: iamv1.GroupVersion,
		Plural:       "globalroles",
		Singular:     "globalrole",
		Aliases:      []string{"gr"},
		TenantScoped: false,
		NewObject:    func() metav1.Object { return &iamv1.GlobalRole{} },
		NewList:      func() any { return &metav1.List[iamv1.GlobalRole]{} },
	},
	{
		Kind:         "GlobalRoleBinding",
		GroupVersion: iamv1.GroupVersion,
		Plural:       "globalrolebindings",
		Singular:     "globalrolebinding",
		Aliases:      []string{"grb"},
		TenantScoped: false,
		NewObject:    func() metav1.Object { return &iamv1.GlobalRoleBinding{} },
		NewList:      func() any { return &metav1.List[iamv1.GlobalRoleBinding]{} },
	},
	{
		Kind:         "TenantRole",
		GroupVersion: iamv1.GroupVersion,
		Plural:       "tenantroles",
		Singular:     "tenantrole",
		Aliases:      []string{"tr"},
		TenantScoped: true,
		NewObject:    func() metav1.Object { return &iamv1.TenantRole{} },
		NewList:      func() any { return &metav1.List[iamv1.TenantRole]{} },
	},
	{
		Kind:         "TenantRoleBinding",
		GroupVersion: iamv1.GroupVersion,
		Plural:       "tenantrolebindings",
		Singular:     "tenantrolebinding",
		Aliases:      []string{"trb"},
		TenantScoped: true,
		NewObject:    func() metav1.Object { return &iamv1.TenantRoleBinding{} },
		NewList:      func() any { return &metav1.List[iamv1.TenantRoleBinding]{} },
	},
	{
		Kind:         "Session",
		GroupVersion: iamv1.GroupVersion,
		Plural:       "sessions",
		Singular:     "session",
		Aliases:      []string{"sess"},
		TenantScoped: false,
		NewObject:    func() metav1.Object { return &iamv1.Session{} },
		NewList:      func() any { return &metav1.List[iamv1.Session]{} },
	},
	{
		Kind:         registryv1.KindUpstream,
		GroupVersion: registryv1.GroupVersion,
		Plural:       "upstreams",
		Singular:     "upstream",
		Aliases:      []string{"up"},
		TenantScoped: false,
		NewObject:    func() metav1.Object { return &registryv1.Upstream{} },
		NewList:      func() any { return &metav1.List[registryv1.Upstream]{} },
	},
	{
		Kind:         registryv1.KindRepository,
		GroupVersion: registryv1.GroupVersion,
		Plural:       "repositories",
		Singular:     "repository",
		Aliases:      []string{"repo"},
		TenantScoped: false,
		NewObject:    func() metav1.Object { return &registryv1.Repository{} },
		NewList:      func() any { return &metav1.List[registryv1.Repository]{} },
	},
	{
		Kind:         registryv1.KindPackage,
		GroupVersion: registryv1.GroupVersion,
		Plural:       "packages",
		Singular:     "package",
		Aliases:      []string{"pkg"},
		TenantScoped: false,
		NewObject:    func() metav1.Object { return &registryv1.Package{} },
		NewList:      func() any { return &metav1.List[registryv1.Package]{} },
	},
	{
		Kind:         registryv1.KindArtifact,
		GroupVersion: registryv1.GroupVersion,
		Plural:       "artifacts",
		Singular:     "artifact",
		Aliases:      []string{"art"},
		TenantScoped: false,
		NewObject:    func() metav1.Object { return &registryv1.Artifact{} },
		NewList:      func() any { return &metav1.List[registryv1.Artifact]{} },
	},
	{
		Kind:         registryv1.KindBlob,
		GroupVersion: registryv1.GroupVersion,
		Plural:       "blobs",
		Singular:     "blob",
		TenantScoped: false,
		NewObject:    func() metav1.Object { return &registryv1.Blob{} },
		NewList:      func() any { return &metav1.List[registryv1.Blob]{} },
	},
	{
		Kind:         registryv1.KindBlobUpload,
		GroupVersion: registryv1.GroupVersion,
		Plural:       "blobuploads",
		Singular:     "blobupload",
		TenantScoped: false,
		NewObject:    func() metav1.Object { return &registryv1.BlobUpload{} },
		NewList:      func() any { return &metav1.List[registryv1.BlobUpload]{} },
	},
	{
		Kind:         registryv1.KindReplicationPolicy,
		GroupVersion: registryv1.GroupVersion,
		Plural:       "replicationpolicies",
		Singular:     "replicationpolicy",
		Aliases:      []string{"replpolicy"},
		TenantScoped: false,
		NewObject:    func() metav1.Object { return &registryv1.ReplicationPolicy{} },
		NewList:      func() any { return &metav1.List[registryv1.ReplicationPolicy]{} },
	},
}

// ResolveResource finds a ResourceInfo by kind, plural, singular, or alias (case-insensitive).
func ResolveResource(nameOrAlias string) (*ResourceInfo, error) {
	target := strings.ToLower(strings.TrimSpace(nameOrAlias))
	if target == "" {
		return nil, fmt.Errorf("empty resource type specified")
	}

	// Support <plural>.<group> syntax, e.g. "instances.core.gamepanel.io"
	if idx := strings.Index(target, "."); idx != -1 {
		target = target[:idx]
	}

	for _, r := range Registry {
		if strings.EqualFold(r.Kind, target) || strings.EqualFold(r.Plural, target) || strings.EqualFold(r.Singular, target) {
			return r, nil
		}
		for _, a := range r.Aliases {
			if strings.EqualFold(a, target) {
				return r, nil
			}
		}
	}
	return nil, fmt.Errorf("the server doesn't have a resource type %q", nameOrAlias)
}

// ResolveGVK finds a ResourceInfo by GroupVersion and Kind.
func ResolveGVK(groupVersion, kind string) (*ResourceInfo, error) {
	for _, r := range Registry {
		if strings.EqualFold(r.Kind, kind) {
			if groupVersion == "" || strings.EqualFold(r.GroupVersion, groupVersion) {
				return r, nil
			}
		}
	}
	return nil, fmt.Errorf("unknown resource GVK: %s, Kind=%s", groupVersion, kind)
}

// ResolveKind finds a ResourceInfo matching the Kind.
func ResolveKind(kind string) (*ResourceInfo, error) {
	for _, r := range Registry {
		if strings.EqualFold(r.Kind, kind) {
			return r, nil
		}
	}
	return nil, fmt.Errorf("the server doesn't have a resource type %q", kind)
}

// ResolveObject finds a ResourceInfo for the given object.
// It checks TypeMeta (APIVersion and Kind) first, and falls back to a Go type switch
// for programmatic structs that may have empty TypeMeta. If TypeMeta is empty and
// the object matches a known type, DefaultMeta() is invoked to populate it.
func ResolveObject(obj metav1.Object) (*ResourceInfo, error) {
	if obj == nil {
		return nil, errors.New("cannot resolve nil object")
	}
	tm := obj.GetTypeMeta()
	if tm != nil && tm.Kind != "" {
		if tm.APIVersion != "" {
			if info, err := ResolveGVK(tm.APIVersion, tm.Kind); err == nil {
				return info, nil
			}
		}
		if info, err := ResolveKind(tm.Kind); err == nil {
			return info, nil
		}
	}
	kind, ok := kindForConcreteObject(obj)
	if !ok {
		return nil, fmt.Errorf("the server doesn't have a resource type for %T", obj)
	}
	obj.DefaultMeta()
	return ResolveKind(kind)
}

func kindForConcreteObject(obj metav1.Object) (string, bool) {
	switch obj.(type) {
	case *corev1.Instance:
		return "Instance", true
	case *corev1.Tenant:
		return "Tenant", true
	case *corev1.Node:
		return "Node", true
	case *corev1.RuntimeClass:
		return "RuntimeClass", true
	case *corev1.ResourcePlan:
		return corev1.KindResourcePlan, true
	case *corev1.App:
		return "App", true
	case *corev1.PrewarmJob:
		return corev1.KindPrewarmJob, true
	case *corev1.TenantMetering:
		return "TenantMetering", true
	case *iamv1.User:
		return "User", true
	case *iamv1.GlobalRole:
		return "GlobalRole", true
	case *iamv1.GlobalRoleBinding:
		return "GlobalRoleBinding", true
	case *iamv1.TenantRole:
		return "TenantRole", true
	case *iamv1.TenantRoleBinding:
		return "TenantRoleBinding", true
	case *iamv1.Session:
		return "Session", true
	default:
		return kindForRegistryObject(obj)
	}
}

func kindForRegistryObject(obj metav1.Object) (string, bool) {
	switch obj.(type) {
	case *registryv1.Upstream:
		return registryv1.KindUpstream, true
	case *registryv1.Repository:
		return registryv1.KindRepository, true
	case *registryv1.Package:
		return registryv1.KindPackage, true
	case *registryv1.Artifact:
		return registryv1.KindArtifact, true
	case *registryv1.Blob:
		return registryv1.KindBlob, true
	case *registryv1.BlobUpload:
		return registryv1.KindBlobUpload, true
	case *registryv1.ReplicationPolicy:
		return registryv1.KindReplicationPolicy, true
	default:
		return "", false
	}
}

// ItemPath builds the REST path for a single resource item.
func ItemPath(info *ResourceInfo, tenant, name string) string {
	if info.TenantScoped {
		if tenant == "" {
			tenant = "default"
		}
		return fmt.Sprintf("/apis/%s/tenants/%s/%s/%s", info.GroupVersion, tenant, info.Plural, name)
	}
	return fmt.Sprintf("/apis/%s/%s/%s", info.GroupVersion, info.Plural, name)
}

// SubresourcePath builds the REST path for a single resource's subresource (e.g. "status", "state", "log", "exec").
func SubresourcePath(info *ResourceInfo, tenant, name, subresource string) string {
	return ItemPath(info, tenant, name) + "/" + strings.TrimPrefix(subresource, "/")
}

// CollectionPath builds the REST path for a collection or list request.
func CollectionPath(info *ResourceInfo, tenant string) string {
	if info.TenantScoped {
		if (info.Kind == "Instance" || info.Kind == "TenantMetering") && tenant == "" {
			// Cluster-wide list
			return fmt.Sprintf("/apis/%s/%s", info.GroupVersion, info.Plural)
		}
		if tenant == "" {
			tenant = "default"
		}
		return fmt.Sprintf("/apis/%s/tenants/%s/%s", info.GroupVersion, tenant, info.Plural)
	}
	return fmt.Sprintf("/apis/%s/%s", info.GroupVersion, info.Plural)
}
