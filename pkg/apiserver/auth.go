package apiserver

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/apiserver/request"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	appmodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/app"
	sessionmodel "github.com/smartcat999/game-panel-cloud/pkg/models/iam/session"
	usermodel "github.com/smartcat999/game-panel-cloud/pkg/models/iam/user"
	registrybootstrap "github.com/smartcat999/game-panel-cloud/pkg/models/registry/bootstrap"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	runtimeclassdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/runtimeclass"
	globalroledb "github.com/smartcat999/game-panel-cloud/pkg/repo/iam/globalrole"
	globalrolebindingdb "github.com/smartcat999/game-panel-cloud/pkg/repo/iam/globalrolebinding"
	userdb "github.com/smartcat999/game-panel-cloud/pkg/repo/iam/user"
)

// Identity is an alias for request.UserInfo.
type Identity = request.UserInfo

// AuthConfig is an alias for sessionmodel.Config bound to the "auth" configuration section.
type AuthConfig = sessionmodel.Config

// NewAuthConfig creates a new session authentication configuration with defaults.
func NewAuthConfig() *AuthConfig {
	return sessionmodel.NewConfig()
}

// Bootstrap seeds built-in GlobalRoles, built-in RuntimeClasses, an initial admin User, and its default Personal Tenant.
func Bootstrap(ctx context.Context, reg *repo.Registry, adminName, adminEmail, adminPassword string) error {
	if err := seedBuiltInGlobalRBAC(ctx, reg); err != nil {
		return err
	}
	if err := seedBuiltInRuntimeClassesAndApps(ctx, reg); err != nil {
		return err
	}
	return seedBootstrapAdminUserAndTenants(ctx, reg, adminName, adminEmail, adminPassword)
}

func defaultBuiltInGlobalRoles() []*iamv1.GlobalRole {
	return []*iamv1.GlobalRole{
		{
			Metadata:    metav1.ObjectMeta{Name: "platform-admin"},
			Description: "Full platform administrator access across all groups and resources",
			Rules:       []iamv1.PolicyRule{{APIGroups: []string{"*"}, Resources: []string{"*"}, Verbs: []string{iamv1.VerbAll}}},
		},
		{
			Metadata:    metav1.ObjectMeta{Name: "node-agent"},
			Description: "Node agent access to report node status, execute prewarm jobs, and reconcile scheduled instances",
			Rules: []iamv1.PolicyRule{
				{APIGroups: []string{corev1.GroupName}, Resources: []string{"nodes", "nodes/status"}, Verbs: []string{iamv1.VerbGet, iamv1.VerbCreate, iamv1.VerbUpdate, iamv1.VerbPatch}},
				{APIGroups: []string{corev1.GroupName}, Resources: []string{"runtimeclasses", "apps"}, Verbs: []string{iamv1.VerbGet, iamv1.VerbList, iamv1.VerbWatch}},
				{APIGroups: []string{corev1.GroupName}, Resources: []string{"prewarmjobs", "prewarmjobs/status", "instances", "instances/status"}, Verbs: []string{iamv1.VerbGet, iamv1.VerbList, iamv1.VerbWatch, iamv1.VerbUpdate, iamv1.VerbPatch}},
				{APIGroups: []string{registryv1.GroupName}, Resources: []string{"repositories", "packages", "artifacts", "blobs"}, Verbs: []string{iamv1.VerbGet, iamv1.VerbList, iamv1.VerbWatch}},
			},
		},
		{
			Metadata:    metav1.ObjectMeta{Name: "authenticated-user"},
			Description: "Baseline access for authenticated users to list accessible workspaces, runtime classes, marketplace apps, and registry catalogs",
			Rules: []iamv1.PolicyRule{
				{APIGroups: []string{corev1.GroupName}, Resources: []string{"tenants"}, Verbs: []string{iamv1.VerbList, iamv1.VerbCreate}},
				{APIGroups: []string{corev1.GroupName}, Resources: []string{"runtimeclasses", "apps"}, Verbs: []string{iamv1.VerbGet, iamv1.VerbList, iamv1.VerbWatch}},
				{APIGroups: []string{registryv1.GroupName}, Resources: []string{"repositories", "packages", "packages/icon", "artifacts", "artifacts/log"}, Verbs: []string{iamv1.VerbGet, iamv1.VerbList, iamv1.VerbWatch}},
			},
		},
		{
			Metadata:    metav1.ObjectMeta{Name: "registry-worker"},
			Description: "Stateless Harbor-style registry worker access to claim, upload, and report artifact synchronization status",
			Rules: []iamv1.PolicyRule{
				{APIGroups: []string{registryv1.GroupName}, Resources: []string{"*"}, Verbs: []string{iamv1.VerbAll}},
			},
		},
	}
}

func seedBuiltInGlobalRBAC(ctx context.Context, reg *repo.Registry) error {
	roleDB := globalroledb.NewDB(reg.Pool())
	grbDB := globalrolebindingdb.NewDB(reg.Pool())
	for _, role := range defaultBuiltInGlobalRoles() {
		existing, err := roleDB.Get(ctx, role.Metadata.Name)
		if errors.Is(err, db.ErrNotFound) {
			if _, err := roleDB.Create(ctx, role); err != nil {
				return fmt.Errorf("create globalrole %s: %w", role.Metadata.Name, err)
			}
		} else if err == nil && existing != nil {
			syncBuiltInGlobalRole(ctx, roleDB, existing, role)
		}
	}
	if _, err := grbDB.Get(ctx, "system-node-agents"); errors.Is(err, db.ErrNotFound) {
		nodeAgentBinding := &iamv1.GlobalRoleBinding{
			Metadata: metav1.ObjectMeta{Name: "system-node-agents"},
			RoleRef:  iamv1.RoleRef{Name: "node-agent"},
			Subjects: []iamv1.Subject{{Kind: iamv1.SubjectKindNode, Name: "*"}},
		}
		if _, err := grbDB.Create(ctx, nodeAgentBinding); err != nil {
			return fmt.Errorf("create globalrolebinding system-node-agents: %w", err)
		}
	}
	if _, err := grbDB.Get(ctx, "system-authenticated-users"); errors.Is(err, db.ErrNotFound) {
		authUserBinding := &iamv1.GlobalRoleBinding{
			Metadata: metav1.ObjectMeta{Name: "system-authenticated-users"},
			RoleRef:  iamv1.RoleRef{Name: "authenticated-user"},
			Subjects: []iamv1.Subject{{Kind: iamv1.SubjectKindUser, Name: "*"}},
		}
		if _, err := grbDB.Create(ctx, authUserBinding); err != nil {
			return fmt.Errorf("create globalrolebinding system-authenticated-users: %w", err)
		}
	}
	if _, err := grbDB.Get(ctx, "system-registry-workers"); errors.Is(err, db.ErrNotFound) {
		workerBinding := &iamv1.GlobalRoleBinding{
			Metadata: metav1.ObjectMeta{Name: "system-registry-workers"},
			RoleRef:  iamv1.RoleRef{Name: "registry-worker"},
			Subjects: []iamv1.Subject{{Kind: iamv1.SubjectKindSystem, Name: registryv1.PrincipalRegistryWorker}},
		}
		if _, err := grbDB.Create(ctx, workerBinding); err != nil {
			return fmt.Errorf("create globalrolebinding system-registry-workers: %w", err)
		}
	}
	return nil
}

func syncBuiltInGlobalRole(ctx context.Context, roleDB *globalroledb.DB, existing, desired *iamv1.GlobalRole) {
	if reflect.DeepEqual(existing.Rules, desired.Rules) && existing.Description == desired.Description {
		return
	}
	existing.Rules = desired.Rules
	existing.Description = desired.Description
	_, _ = roleDB.Update(ctx, existing)
}

func seedBuiltInRuntimeClassesAndApps(ctx context.Context, reg *repo.Registry) error {
	rcDB := runtimeclassdb.NewDB(reg.Pool())
	builtInRuntimeClasses := []*corev1.RuntimeClass{
		{
			Metadata:   metav1.ObjectMeta{Name: corev1.DefaultDockerDriverName},
			Handler:    corev1.DefaultDockerDriverName,
			IsDefault:  true,
			Parameters: map[string]string{"restartPolicy": "unless-stopped"},
		},
	}
	for _, rc := range builtInRuntimeClasses {
		if _, err := rcDB.Get(ctx, rc.Metadata.Name); errors.Is(err, db.ErrNotFound) {
			if _, err := rcDB.Create(ctx, rc); err != nil {
				return fmt.Errorf("create runtimeclass %s: %w", rc.Metadata.Name, err)
			}
		}
	}
	if err := registrybootstrap.EnsureBuiltInRegistry(ctx, reg); err != nil {
		return fmt.Errorf("seed built-in registry: %w", err)
	}
	if err := appmodel.EnsureBuiltInApps(ctx, reg); err != nil {
		return fmt.Errorf("seed built-in apps: %w", err)
	}
	return nil
}

func seedBootstrapAdminUserAndTenants(ctx context.Context, reg *repo.Registry, adminName, adminEmail, adminPassword string) error {
	if adminName == "" {
		return nil
	}
	userDB := userdb.NewDB(reg.Pool())
	grbDB := globalrolebindingdb.NewDB(reg.Pool())
	if _, err := userDB.Get(ctx, adminName); errors.Is(err, db.ErrNotFound) {
		hash, err := usermodel.HashPassword(adminPassword)
		if err != nil {
			return err
		}
		user := &iamv1.User{
			Metadata: metav1.ObjectMeta{Name: adminName},
			Spec:     iamv1.UserSpec{Email: adminEmail, DisplayName: adminName, PasswordHash: hash, State: iamv1.UserStateActive},
			Status:   iamv1.UserStatus{ObservedGeneration: 1, Phase: iamv1.UserStateActive, DefaultTenant: adminName},
		}
		if _, err := userDB.Create(ctx, user); err != nil {
			return err
		}
	}
	bindingName := adminName + "-platform-admin"
	if _, err := grbDB.Get(ctx, bindingName); errors.Is(err, db.ErrNotFound) {
		binding := &iamv1.GlobalRoleBinding{
			Metadata: metav1.ObjectMeta{Name: bindingName},
			RoleRef:  iamv1.RoleRef{Name: "platform-admin"},
			Subjects: []iamv1.Subject{{Kind: iamv1.SubjectKindUser, Name: adminName}},
		}
		if _, err := grbDB.Create(ctx, binding); err != nil {
			return err
		}
	}
	if err := usermodel.EnsurePersonalTenant(ctx, reg, adminName); err != nil {
		return err
	}
	if users, err := userDB.List(ctx, db.ListOptions{}); err == nil && users != nil {
		for _, u := range users.Items {
			if u != nil && u.Metadata.Name != "" && u.Metadata.Name != adminName {
				_ = usermodel.EnsurePersonalTenant(ctx, reg, u.Metadata.Name)
			}
		}
	}
	return nil
}
