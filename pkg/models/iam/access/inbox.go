package access

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	tenantrolebindingdb "github.com/smartcat999/game-panel-cloud/pkg/repo/iam/tenantrolebinding"
	userdb "github.com/smartcat999/game-panel-cloud/pkg/repo/iam/user"
)

// PushUserMessage appends an in-app notification to the target user's inbox annotation.
func PushUserMessage(ctx context.Context, reg *repo.Registry, username string, act iamv1.UserMessageAction) {
	if reg == nil || reg.Pool() == nil {
		return
	}
	user := strings.TrimSpace(username)
	if user == "" || user == "*" {
		return
	}
	userDB := userdb.NewDB(reg.Pool())
	for range 3 {
		u, err := userDB.Get(ctx, user)
		if err != nil || u == nil || u.Metadata.DeletionTimestamp != nil {
			return
		}
		if u.Metadata.Annotations == nil {
			u.Metadata.Annotations = make(map[string]string)
		}
		if !AppendInboxMessageToUser(u, act) {
			return
		}
		if _, err := userDB.Update(ctx, u); err == nil || !errors.Is(err, db.ErrConflict) {
			return
		}
	}
}

// AppendInboxMessageToUser prepends a UserMessage to u.Metadata.Annotations[iamv1.AnnotationInboxMessages].
func AppendInboxMessageToUser(u *iamv1.User, act iamv1.UserMessageAction) bool {
	title := strings.TrimSpace(act.Title)
	content := strings.TrimSpace(act.Content)
	if u == nil || title == "" || content == "" {
		return false
	}
	cat := strings.TrimSpace(act.Category)
	if cat == "" {
		cat = iamv1.MessageCategorySystem
	}
	now := time.Now().UTC()
	id := "msg-" + db.NewUID()
	var custom []iamv1.UserMessage
	if raw := strings.TrimSpace(u.Metadata.Annotations[iamv1.AnnotationInboxMessages]); raw != "" {
		_ = json.Unmarshal([]byte(raw), &custom)
	}
	msg := iamv1.UserMessage{
		Metadata:    metav1.ObjectMeta{Name: id, UID: id, CreationTimestamp: now},
		Category:    cat,
		Title:       title,
		Content:     content,
		Workspace:   strings.TrimSpace(act.Workspace),
		ActionURL:   strings.TrimSpace(act.ActionURL),
		ActionLabel: strings.TrimSpace(act.ActionLabel),
		Timestamp:   now,
	}
	msg.DefaultMeta()
	custom = append([]iamv1.UserMessage{msg}, custom...)
	if len(custom) > 50 {
		custom = custom[:50]
	}
	if b, err := json.Marshal(custom); err == nil {
		if u.Metadata.Annotations == nil {
			u.Metadata.Annotations = make(map[string]string)
		}
		u.Metadata.Annotations[iamv1.AnnotationInboxMessages] = string(b)
		return true
	}
	return false
}

// DescribeTenantRoleName returns the product-facing Chinese role title for a TenantRole name.
func DescribeTenantRoleName(role string) string {
	switch strings.TrimSpace(role) {
	case "tenant-admin":
		return "工作区管理员"
	case "instance-operator":
		return "运维操作员"
	case "tenant-viewer":
		return "只读成员"
	default:
		return role
	}
}

// NotifyTenantBindingChanged pushes an in-app notification to each explicit User subject on a TenantRoleBinding change.
func NotifyTenantBindingChanged(ctx context.Context, reg *repo.Registry, tenant, roleName string, subjects []iamv1.Subject, mode string) {
	roleDesc := DescribeTenantRoleName(roleName)
	for _, s := range subjects {
		if (s.Kind != "" && s.Kind != iamv1.SubjectKindUser) || s.Name == "" || s.Name == "*" {
			continue
		}
		if mode == "created" && s.Name == tenant {
			continue
		}
		act := buildTenantMemberNotice(tenant, roleDesc, mode)
		PushUserMessage(ctx, reg, s.Name, act)
	}
}

func buildTenantMemberNotice(tenant, roleDesc, mode string) iamv1.UserMessageAction {
	switch mode {
	case "updated":
		return iamv1.UserMessageAction{
			Category:    iamv1.MessageCategoryMember,
			Title:       fmt.Sprintf("工作区「%s」角色权限已更新", tenant),
			Content:     fmt.Sprintf("您在工作区「%s」中的角色已调整为「%s」，新权限已立即生效。", tenant, roleDesc),
			Workspace:   tenant,
			ActionURL:   "/console",
			ActionLabel: "前往该工作区",
		}
	case "deleted":
		return iamv1.UserMessageAction{
			Category:  iamv1.MessageCategoryMember,
			Title:     fmt.Sprintf("已移出工作区「%s」", tenant),
			Content:   fmt.Sprintf("您已被移出工作区「%s」，不再拥有该工作区的资源访问权限。", tenant),
			Workspace: tenant,
		}
	default:
		return iamv1.UserMessageAction{
			Category:    iamv1.MessageCategoryMember,
			Title:       fmt.Sprintf("已加入工作区「%s」", tenant),
			Content:     fmt.Sprintf("您已被添加至团队工作区「%s」并授予「%s」权限，可通过顶部工作区切换器进入协作。", tenant, roleDesc),
			Workspace:   tenant,
			ActionURL:   "/console",
			ActionLabel: "前往该工作区",
		}
	}
}

// NotifyGlobalBindingChanged pushes an in-app notification when a GlobalRoleBinding is created or deleted.
func NotifyGlobalBindingChanged(ctx context.Context, reg *repo.Registry, roleName string, subjects []iamv1.Subject, mode string) {
	if strings.TrimSpace(roleName) != "platform-admin" {
		return
	}
	for _, s := range subjects {
		if (s.Kind != "" && s.Kind != iamv1.SubjectKindUser) || s.Name == "" || s.Name == "*" {
			continue
		}
		if mode == "deleted" {
			PushUserMessage(ctx, reg, s.Name, iamv1.UserMessageAction{
				Category: iamv1.MessageCategoryMember,
				Title:    "平台管理员权限已撤销",
				Content:  "您的平台管理员权限已被移除，账号已恢复为普通成员权限。",
			})
			continue
		}
		PushUserMessage(ctx, reg, s.Name, iamv1.UserMessageAction{
			Category:    iamv1.MessageCategoryMember,
			Title:       "平台管理员权限已授予",
			Content:     "您的账号已被授予「平台管理员」权限，可进入平台管理后台管理集群节点、应用市场、工作区与用户权限。",
			ActionURL:   "/console/admin/tenants",
			ActionLabel: "前往平台管理",
		})
	}
}

// NotifyTenantQuotaUpdated pushes an in-app notification when a workspace's resource quota changes.
func NotifyTenantQuotaUpdated(ctx context.Context, reg *repo.Registry, before, after *corev1.Tenant) {
	if reg == nil || reg.Pool() == nil || before == nil || after == nil {
		return
	}
	if quotasEqual(before.Spec.Quota, after.Spec.Quota) {
		return
	}
	display := strings.TrimSpace(after.Spec.DisplayName)
	if after.Spec.Type != corev1.TenantTypeOrganization && strings.HasSuffix(strings.ToLower(display), " personal") {
		display = strings.TrimSpace(display[:len(display)-len(" personal")])
	}
	if display == "" {
		display = after.Metadata.Name
	}
	q := after.Spec.Quota
	act := iamv1.UserMessageAction{
		Category:    iamv1.MessageCategoryQuota,
		Title:       fmt.Sprintf("工作区「%s」配额已调整", display),
		Content:     fmt.Sprintf("工作区「%s」最新配额上限：CPU %s 核 · 内存 %s · 存储 %s · 实例上限 %s 台。", display, nonEmptyQty(q[corev1.ResourceCPU]), nonEmptyQty(q[corev1.ResourceMemory]), nonEmptyQty(q[corev1.ResourceStorage]), nonEmptyQty(q[corev1.ResourceInstances])),
		Workspace:   after.Metadata.Name,
		ActionURL:   "/console/usage",
		ActionLabel: "查看用量配额",
	}
	for _, user := range collectTenantRecipients(ctx, reg, after) {
		PushUserMessage(ctx, reg, user, act)
	}
}

func collectTenantRecipients(ctx context.Context, reg *repo.Registry, t *corev1.Tenant) []string {
	seen := make(map[string]bool)
	var users []string
	add := func(name string) {
		n := strings.TrimSpace(name)
		if n != "" && n != "*" && !seen[n] {
			seen[n] = true
			users = append(users, n)
		}
	}
	add(t.Spec.Owner)
	trbDB := tenantrolebindingdb.NewDB(reg.Pool())
	if list, err := trbDB.List(ctx, db.ListOptions{Tenant: t.Metadata.Name}); err == nil && list != nil {
		for _, b := range list.Items {
			if b == nil {
				continue
			}
			for _, s := range b.Subjects {
				if s.Kind == "" || s.Kind == iamv1.SubjectKindUser {
					add(s.Name)
				}
			}
		}
	}
	return users
}

func quotasEqual(a, b corev1.ResourceList) bool {
	for _, k := range []corev1.ResourceName{corev1.ResourceCPU, corev1.ResourceMemory, corev1.ResourceStorage, corev1.ResourceInstances, corev1.ResourceBackups, corev1.ResourceBackupStorage} {
		if strings.TrimSpace(string(a[k])) != strings.TrimSpace(string(b[k])) {
			return false
		}
	}
	return true
}

func nonEmptyQty(q corev1.Quantity) string {
	if s := strings.TrimSpace(string(q)); s != "" {
		return s
	}
	return "无限制"
}
