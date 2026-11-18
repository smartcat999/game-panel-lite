package user

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/models"
	instancemodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/instance"
	"github.com/smartcat999/game-panel-cloud/pkg/models/iam/access"
	auditdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/audit"
	instancedb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/instance"
	tenantdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/tenant"
)

const (
	inboxAlertWindow = 14 * 24 * time.Hour
	inboxAlertLimit  = 30
)

func (o *operator) ListMessages(ctx context.Context, name string) (*metav1.List[*iamv1.UserMessage], error) {
	u, err := o.db.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	items := o.buildUserInboxMessages(ctx, u)
	return &metav1.List[*iamv1.UserMessage]{
		TypeMeta: metav1.TypeMeta{APIVersion: iamv1.GroupVersion, Kind: "UserMessageList"},
		Metadata: metav1.ListMeta{TotalCount: len(items)},
		Items:    items,
	}, nil
}

func (o *operator) MutateMessages(ctx context.Context, name string, act *iamv1.UserMessageAction) (*metav1.List[*iamv1.UserMessage], error) {
	if act == nil {
		return nil, fmt.Errorf("%w: message action is required", models.ErrBadRequest)
	}
	if strings.TrimSpace(act.Action) == iamv1.MessageActionSend {
		if err := o.authorizeMessageSend(ctx, name); err != nil {
			return nil, err
		}
		if len(act.IDs) > 0 && strings.TrimSpace(act.IDs[0]) == "*" {
			if err := o.broadcastUserMessage(ctx, act); err != nil {
				return nil, err
			}
			return o.ListMessages(ctx, name)
		}
	}
	for attempt := range 3 {
		u, err := o.db.Get(ctx, name)
		if err != nil {
			return nil, err
		}
		if u.Metadata.Annotations == nil {
			u.Metadata.Annotations = make(map[string]string)
		}
		if err := o.applyMessageAction(ctx, u, act); err != nil {
			return nil, err
		}
		if _, err := o.db.Update(ctx, u); err != nil {
			if errors.Is(err, db.ErrConflict) && attempt < 2 {
				continue
			}
			return nil, err
		}
		break
	}
	return o.ListMessages(ctx, name)
}

func (o *operator) authorizeMessageSend(ctx context.Context, name string) error {
	if o.reg == nil || o.reg.Pool() == nil {
		return nil
	}
	u, explicit := iamv1.UserFrom(ctx)
	if !explicit || u.Kind == iamv1.SubjectKindSystem {
		return nil
	}
	allowed, err := access.AllowsGlobal(ctx, o.reg, u, iamv1.GroupName, "users/messages", iamv1.VerbCreate, name, nil)
	if err != nil {
		return err
	}
	if !allowed {
		return fmt.Errorf("%w: sending notifications requires platform administrator privileges", models.ErrForbidden)
	}
	return nil
}

func (o *operator) broadcastUserMessage(ctx context.Context, act *iamv1.UserMessageAction) error {
	if strings.TrimSpace(act.Title) == "" || strings.TrimSpace(act.Content) == "" {
		return fmt.Errorf("%w: title and content are required", models.ErrBadRequest)
	}
	list, err := o.db.List(ctx, db.ListOptions{})
	if err != nil {
		return err
	}
	for _, u := range list.Items {
		if u == nil || u.Metadata.DeletionTimestamp != nil || u.Spec.State == iamv1.UserStateDisabled {
			continue
		}
		if u.Metadata.Annotations == nil {
			u.Metadata.Annotations = make(map[string]string)
		}
		if err := appendCustomUserMessage(u, act); err == nil {
			_, _ = o.db.Update(ctx, u)
		}
	}
	return nil
}

func (o *operator) applyMessageAction(ctx context.Context, u *iamv1.User, act *iamv1.UserMessageAction) error {
	readSet := parseReadIDSet(u.Metadata.Annotations[iamv1.AnnotationInboxReadIDs])
	switch strings.TrimSpace(act.Action) {
	case iamv1.MessageActionMarkRead:
		for _, id := range act.IDs {
			if trimmed := strings.TrimSpace(id); trimmed != "" {
				readSet[trimmed] = true
			}
		}
		u.Metadata.Annotations[iamv1.AnnotationInboxReadIDs] = encodeReadIDSet(readSet)
	case iamv1.MessageActionMarkAllRead:
		for _, m := range o.buildUserInboxMessages(ctx, u) {
			readSet[m.Metadata.Name] = true
		}
		u.Metadata.Annotations[iamv1.AnnotationInboxReadIDs] = encodeReadIDSet(readSet)
		u.Metadata.Annotations[iamv1.AnnotationInboxReadAllAt] = time.Now().UTC().Format(time.RFC3339Nano)
	case iamv1.MessageActionSend:
		return appendCustomUserMessage(u, act)
	default:
		return fmt.Errorf("%w: unsupported message action %q", models.ErrBadRequest, act.Action)
	}
	return nil
}

func appendCustomUserMessage(u *iamv1.User, act *iamv1.UserMessageAction) error {
	title := strings.TrimSpace(act.Title)
	content := strings.TrimSpace(act.Content)
	if title == "" || content == "" {
		return fmt.Errorf("%w: title and content are required", models.ErrBadRequest)
	}
	cat := strings.TrimSpace(act.Category)
	if cat == "" {
		cat = iamv1.MessageCategorySystem
	}
	now := time.Now().UTC()
	id := "msg-" + db.NewUID()
	custom := parseCustomMessages(u.Metadata.Annotations[iamv1.AnnotationInboxMessages])
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
		u.Metadata.Annotations[iamv1.AnnotationInboxMessages] = string(b)
	}
	return nil
}

func (o *operator) buildUserInboxMessages(ctx context.Context, u *iamv1.User) []*iamv1.UserMessage {
	if u == nil {
		return nil
	}
	var items []*iamv1.UserMessage
	hasCustomMember := make(map[string]bool)
	hasCustomQuota := make(map[string]bool)
	hasCustomGlobal := false
	for _, cm := range parseCustomMessages(u.Metadata.Annotations[iamv1.AnnotationInboxMessages]) {
		copyMsg := cm
		copyMsg.DefaultMeta()
		items = append(items, &copyMsg)
		if copyMsg.Category == iamv1.MessageCategoryMember {
			if copyMsg.Workspace != "" {
				hasCustomMember[copyMsg.Workspace] = true
			} else {
				hasCustomGlobal = true
			}
		} else if copyMsg.Category == iamv1.MessageCategoryQuota && copyMsg.Workspace != "" {
			hasCustomQuota[copyMsg.Workspace] = true
		}
	}
	items = append(items, buildWelcomeMessage(u))
	tenants, isPlatformAdmin := o.collectUserMemberMessages(ctx, u, hasCustomMember, hasCustomGlobal, &items)
	alertedInstances := o.collectAlertMessages(ctx, tenants, isPlatformAdmin, &items)
	notifiedTasks := o.collectTaskFailureMessages(ctx, tenants, &items)
	o.collectQuotaAndBackupMessages(ctx, tenants, hasCustomQuota, alertedInstances, notifiedTasks, &items)
	applyReadStates(u, items)
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].Timestamp.After(items[j].Timestamp)
	})
	return items
}

func buildWelcomeMessage(u *iamv1.User) *iamv1.UserMessage {
	ts := u.Metadata.CreationTimestamp
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	display := strings.TrimSpace(u.Spec.DisplayName)
	if display == "" {
		display = u.Metadata.Name
	}
	id := "sys-welcome-" + u.Metadata.Name
	msg := &iamv1.UserMessage{
		Metadata:    metav1.ObjectMeta{Name: id, UID: id, CreationTimestamp: ts},
		Category:    iamv1.MessageCategorySystem,
		Title:       "欢迎使用 GamePanel Lite 游戏服务器管理面板",
		Content:     fmt.Sprintf("您的账号「%s」已启用。您可通过顶部工作区切换器管理游戏服务器，并在此接收实时监控告警、成员权限变更、配额调整与备份任务通知。", display),
		ActionURL:   "/console",
		ActionLabel: "前往服务器列表",
		Timestamp:   ts,
	}
	msg.DefaultMeta()
	return msg
}

func (o *operator) collectUserMemberMessages(ctx context.Context, u *iamv1.User, skipMember map[string]bool, skipGlobal bool, items *[]*iamv1.UserMessage) (map[string]bool, bool) {
	tenants := make(map[string]bool)
	isPlatformAdmin := u.Metadata.Name == builtInAdminName()
	subjectOpt := db.ListOptions{FieldSelector: map[string]string{"subject": u.Metadata.Name}}
	if o.trbDB != nil {
		if list, err := o.trbDB.List(ctx, subjectOpt); err == nil && list != nil {
			for _, b := range list.Items {
				if b != nil && subjectListHasUser(b.Subjects, u.Metadata.Name) {
					tenants[b.Metadata.Tenant] = true
					if !skipMember[b.Metadata.Tenant] {
						*items = append(*items, buildTenantBindingMessage(b, u.Metadata.Name))
					}
				}
			}
		}
	}
	if o.grbDB != nil {
		if list, err := o.grbDB.List(ctx, subjectOpt); err == nil && list != nil {
			for _, b := range list.Items {
				if b != nil && subjectListHasUser(b.Subjects, u.Metadata.Name) {
					if strings.TrimSpace(b.RoleRef.Name) == "platform-admin" {
						isPlatformAdmin = true
					}
					if !skipGlobal {
						*items = append(*items, buildGlobalBindingMessage(b))
					}
				}
			}
		}
	}
	return tenants, isPlatformAdmin
}

func buildTenantBindingMessage(b *iamv1.TenantRoleBinding, selfUser string) *iamv1.UserMessage {
	ts := b.Metadata.CreationTimestamp
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	roleName := b.RoleRef.Name
	roleDesc := describeTenantRoleName(roleName)
	actionURL := "/console/members"
	actionLabel := "查看工作区成员"
	if b.Metadata.Tenant == selfUser {
		actionURL = "/console"
		actionLabel = "前往个人工作区"
	}
	id := fmt.Sprintf("mbr-trb-%s-%s", b.Metadata.Tenant, b.Metadata.Name)
	msg := &iamv1.UserMessage{
		Metadata:    metav1.ObjectMeta{Name: id, UID: id, Tenant: b.Metadata.Tenant, CreationTimestamp: ts},
		Category:    iamv1.MessageCategoryMember,
		Title:       fmt.Sprintf("工作区「%s」成员权限已生效", b.Metadata.Tenant),
		Content:     fmt.Sprintf("您已在工作区「%s」中获得「%s」权限，可按对应权限管理该工作区内的游戏服务器与资源。", b.Metadata.Tenant, roleDesc),
		Workspace:   b.Metadata.Tenant,
		ActionURL:   actionURL,
		ActionLabel: actionLabel,
		Timestamp:   ts,
	}
	msg.DefaultMeta()
	return msg
}

func buildGlobalBindingMessage(b *iamv1.GlobalRoleBinding) *iamv1.UserMessage {
	ts := b.Metadata.CreationTimestamp
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	roleDesc := describeGlobalRoleName(b.RoleRef.Name)
	id := "mbr-grb-" + b.Metadata.Name
	msg := &iamv1.UserMessage{
		Metadata:    metav1.ObjectMeta{Name: id, UID: id, CreationTimestamp: ts},
		Category:    iamv1.MessageCategoryMember,
		Title:       fmt.Sprintf("%s权限已授予", roleDesc),
		Content:     fmt.Sprintf("您的账号已被授予「%s」权限，可进入平台管理后台管理集群节点、应用市场、工作区与用户权限。", roleDesc),
		ActionURL:   "/console/admin/tenants",
		ActionLabel: "前往平台管理",
		Timestamp:   ts,
	}
	msg.DefaultMeta()
	return msg
}

func (o *operator) collectAlertMessages(ctx context.Context, tenants map[string]bool, isPlatformAdmin bool, items *[]*iamv1.UserMessage) map[string]bool {
	alertedInstances := make(map[string]bool)
	if o.reg == nil || o.reg.Pool() == nil {
		return alertedInstances
	}
	scopes := buildAlertInboxScopes(ctx, o.tenantDB, tenants, isPlatformAdmin)
	if len(scopes) == 0 {
		return alertedInstances
	}
	events, err := auditdb.NewDB(o.reg.Pool()).ListRecentAlertTransitions(ctx, scopes, time.Now().UTC().Add(-inboxAlertWindow), inboxAlertLimit)
	if err != nil {
		return alertedInstances
	}
	for _, ev := range events {
		msg := buildAlertInboxMessage(ev)
		if msg == nil {
			continue
		}
		*items = append(*items, msg)
		if ev.Attributes["targetKind"] == "instances" && msg.Workspace != "" {
			target := nonEmptyOr(ev.Attributes["targetName"], ev.TargetName)
			if target != "" {
				alertedInstances[msg.Workspace+"/"+target] = true
			}
		}
	}
	return alertedInstances
}

func buildAlertInboxScopes(ctx context.Context, tenantDB *tenantdb.DB, tenants map[string]bool, isPlatformAdmin bool) []string {
	seen := make(map[string]bool, len(tenants)+2)
	var scopes []string
	add := func(s string) {
		if !seen[s] {
			seen[s] = true
			scopes = append(scopes, s)
		}
	}
	if isPlatformAdmin {
		add("")
		if tenantDB != nil {
			if tList, err := tenantDB.List(ctx, db.ListOptions{}); err == nil && tList != nil {
				for _, t := range tList.Items {
					if t != nil && t.Metadata.DeletionTimestamp == nil && t.Metadata.Name != "" {
						add(t.Metadata.Name)
					}
				}
			}
		}
	}
	for t := range tenants {
		if t != "" {
			add(t)
		}
	}
	sort.Strings(scopes)
	return scopes
}

func buildAlertInboxMessage(ev *corev1.Event) *iamv1.UserMessage {
	if ev == nil || ev.Metadata.Name == "" {
		return nil
	}
	attrs := ev.Attributes
	scope := strings.TrimSpace(attrs["scope"])
	tenant := nonEmptyOr(attrs["tenant"], ev.Metadata.Tenant)
	state := strings.TrimSpace(attrs["state"])
	targetKind := strings.TrimSpace(attrs["targetKind"])
	targetName := nonEmptyOr(attrs["targetName"], ev.TargetName)
	ruleTitle := nonEmptyOr(attrs["ruleTitle"], "监控规则状态变更")
	ts := ev.Timestamp
	if ts.IsZero() {
		ts = ev.Metadata.CreationTimestamp
	}
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	workspace := tenant
	if scope == corev1.AlertScopePlatform {
		workspace = ""
	}
	actionURL, actionLabel := alertInboxAction(scope, tenant, targetKind, targetName)
	category := iamv1.MessageCategoryAlert
	if targetKind == "subscription" {
		category = iamv1.MessageCategoryQuota
	}
	msg := &iamv1.UserMessage{
		Metadata:    metav1.ObjectMeta{Name: ev.Metadata.Name, UID: ev.Metadata.Name, Tenant: workspace, CreationTimestamp: ts},
		Category:    category,
		Title:       formatAlertInboxTitle(scope, state, tenant, targetKind, targetName, ruleTitle),
		Content:     nonEmptyOr(attrs["message"], ruleTitle),
		Workspace:   workspace,
		ActionURL:   actionURL,
		ActionLabel: actionLabel,
		Timestamp:   ts,
	}
	msg.DefaultMeta()
	return msg
}

func formatAlertInboxTitle(scope, state, tenant, targetKind, targetName, ruleTitle string) string {
	isResolved := state == corev1.AlertStateResolved
	tag := "告警触发"
	if scope == corev1.AlertScopePlatform {
		tag = "平台告警"
	}
	if isResolved {
		tag = "告警恢复"
	}
	switch {
	case targetKind == "nodes" && targetName != "":
		return fmt.Sprintf("[%s] 节点「%s」· %s", tag, targetName, ruleTitle)
	case targetKind == "instances" && targetName != "":
		return fmt.Sprintf("[%s] 服务器「%s」· %s", tag, targetName, ruleTitle)
	case targetKind == "subscription":
		return fmt.Sprintf("[订阅提醒] 工作区「%s」· %s", nonEmptyOr(targetName, tenant), ruleTitle)
	default:
		return fmt.Sprintf("[%s] %s", tag, ruleTitle)
	}
}

func alertInboxAction(scope, tenant, targetKind, targetName string) (string, string) {
	if targetKind == "subscription" && tenant != "" {
		return fmt.Sprintf("/console/usage?tenant=%s&tab=subscription", url.QueryEscape(tenant)), "续期工作区套餐"
	}
	if targetKind == "instances" && targetName != "" {
		escaped := url.PathEscape(targetName)
		if tenant != "" {
			return fmt.Sprintf("/console/servers/%s?tenant=%s&tab=console", escaped, url.QueryEscape(tenant)), "查看服务器控制台"
		}
		return fmt.Sprintf("/console/servers/%s?tab=console", escaped), "查看服务器控制台"
	}
	if targetKind == "nodes" {
		return "/console/admin/nodes", "查看计算节点"
	}
	if scope == corev1.AlertScopePlatform {
		return "/console/admin/alerts", "查看平台告警"
	}
	if tenant != "" {
		return fmt.Sprintf("/console/events?tenant=%s", url.QueryEscape(tenant)), "查看告警记录"
	}
	return "/console", "前往查看"
}

type taskDedupSet struct {
	backupOps    map[string]bool
	scheduleRuns map[string]bool
}

func newTaskDedupSet() taskDedupSet {
	return taskDedupSet{
		backupOps:    make(map[string]bool),
		scheduleRuns: make(map[string]bool),
	}
}

func (o *operator) collectTaskFailureMessages(ctx context.Context, tenants map[string]bool, items *[]*iamv1.UserMessage) taskDedupSet {
	dedup := newTaskDedupSet()
	if o.reg == nil || o.reg.Pool() == nil || len(tenants) == 0 {
		return dedup
	}
	scopes := make([]string, 0, len(tenants))
	for t := range tenants {
		if t != "" {
			scopes = append(scopes, t)
		}
	}
	if len(scopes) == 0 {
		return dedup
	}
	sort.Strings(scopes)
	events, err := auditdb.NewDB(o.reg.Pool()).ListRecentTaskFailures(ctx, scopes, time.Now().UTC().Add(-inboxAlertWindow), inboxAlertLimit)
	if err != nil {
		return dedup
	}
	for _, ev := range events {
		msg := buildTaskFailureInboxMessage(ev)
		if msg == nil {
			continue
		}
		*items = append(*items, msg)
		recordTaskDedup(ev, &dedup)
	}
	return dedup
}

func recordTaskDedup(ev *corev1.Event, dedup *taskDedupSet) {
	if ev == nil || dedup == nil {
		return
	}
	attrs := ev.Attributes
	tenant := nonEmptyOr(attrs["tenant"], ev.Metadata.Tenant)
	instName := nonEmptyOr(attrs["instanceName"], ev.TargetName)
	if opID := strings.TrimSpace(attrs["operationID"]); opID != "" {
		dedup.backupOps[opID] = true
	}
	if path := strings.TrimSpace(attrs["path"]); path != "" && tenant != "" && instName != "" {
		dedup.backupOps[tenant+"/"+instName+"/"+path] = true
	}
	if runID := strings.TrimSpace(attrs["runID"]); runID != "" {
		dedup.scheduleRuns[runID] = true
	}
	if schedID := strings.TrimSpace(attrs["scheduleID"]); schedID != "" && tenant != "" && instName != "" {
		dedup.scheduleRuns[tenant+"/"+instName+"/"+schedID] = true
	}
}

func buildTaskFailureInboxMessage(ev *corev1.Event) *iamv1.UserMessage {
	if ev == nil || ev.Metadata.Name == "" {
		return nil
	}
	attrs := ev.Attributes
	tenant := nonEmptyOr(attrs["tenant"], ev.Metadata.Tenant)
	instName := nonEmptyOr(attrs["instanceName"], ev.TargetName)
	if tenant == "" || instName == "" {
		return nil
	}
	ts := ev.Timestamp
	if ts.IsZero() {
		ts = ev.Metadata.CreationTimestamp
	}
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	title, content, actionURL, actionLabel := formatTaskFailureDetails(tenant, instName, attrs)
	msg := &iamv1.UserMessage{
		Metadata:    metav1.ObjectMeta{Name: ev.Metadata.Name, UID: ev.Metadata.Name, Tenant: tenant, CreationTimestamp: ts},
		Category:    iamv1.MessageCategoryBackup,
		Title:       title,
		Content:     content,
		Workspace:   tenant,
		ActionURL:   actionURL,
		ActionLabel: actionLabel,
		Timestamp:   ts,
	}
	msg.DefaultMeta()
	return msg
}

func formatTaskFailureDetails(tenant, instName string, attrs map[string]string) (string, string, string, string) {
	escapedName := url.PathEscape(instName)
	escapedTenant := url.QueryEscape(tenant)
	reason := nonEmptyOr(attrs["message"], "后台任务执行未完成，请前往服务器页面检查详情。")
	taskName := strings.TrimSpace(attrs["taskName"])
	switch strings.TrimSpace(attrs["taskKind"]) {
	case corev1.TaskKindSchedule:
		schedTitle := nonEmptyOr(taskName, describeScheduleAction(attrs["scheduleAction"]))
		return fmt.Sprintf("[定时任务异常] 服务器「%s」· %s", instName, schedTitle),
			reason,
			fmt.Sprintf("/console/servers/%s?tenant=%s&tab=schedules", escapedName, escapedTenant),
			"查看定时任务"
	case corev1.TaskKindBackupRestore:
		content := reason
		if taskName != "" {
			content = fmt.Sprintf("存档快照「%s」恢复未完成：%s", taskName, reason)
		}
		return fmt.Sprintf("[备份异常] 服务器「%s」· 存档恢复失败", instName),
			content,
			fmt.Sprintf("/console/servers/%s?tenant=%s&tab=backup", escapedName, escapedTenant),
			"查看服务器备份"
	default:
		tag := "备份异常"
		if attrs["quotaBlocked"] == "true" {
			tag = "备份受阻"
		}
		snapTitle := nonEmptyOr(taskName, "备份快照归档")
		return fmt.Sprintf("[%s] 服务器「%s」· %s", tag, instName, snapTitle),
			reason,
			fmt.Sprintf("/console/servers/%s?tenant=%s&tab=backup", escapedName, escapedTenant),
			"查看服务器备份"
	}
}

func describeScheduleAction(action string) string {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case corev1.ScheduleActionBackup:
		return "定时自动备份"
	case corev1.ScheduleActionRestart:
		return "定时例行重启"
	default:
		return "定时指令任务"
	}
}

func (o *operator) collectQuotaAndBackupMessages(ctx context.Context, tenants map[string]bool, skipQuota, alertedInstances map[string]bool, notifiedTasks taskDedupSet, items *[]*iamv1.UserMessage) {
	if len(tenants) == 0 {
		return
	}
	if o.tenantDB != nil {
		for tenantName := range tenants {
			if skipQuota[tenantName] {
				continue
			}
			if t, err := o.tenantDB.Get(ctx, tenantName); err == nil && t != nil {
				*items = append(*items, buildTenantQuotaMessage(t))
			}
		}
	}
	if o.reg == nil || o.reg.Pool() == nil {
		return
	}
	instDB := instancedb.NewDB(o.reg.Pool())
	if instDB == nil {
		return
	}
	for tenantName := range tenants {
		if iList, err := instDB.List(ctx, db.ListOptions{Tenant: tenantName}); err == nil && iList != nil {
			for _, inst := range iList.Items {
				if inst != nil {
					appendInstanceBackupMessages(inst, alertedInstances, notifiedTasks, items)
				}
			}
		}
	}
}

func buildTenantQuotaMessage(t *corev1.Tenant) *iamv1.UserMessage {
	ts := t.Metadata.CreationTimestamp
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	q := t.Spec.Quota
	display := strings.TrimSpace(t.Spec.DisplayName)
	if t.Spec.Type != corev1.TenantTypeOrganization && strings.HasSuffix(strings.ToLower(display), " personal") {
		display = strings.TrimSpace(display[:len(display)-len(" personal")])
	}
	if display == "" {
		display = t.Metadata.Name
	}
	id := fmt.Sprintf("quota-%s-g%d", t.Metadata.Name, max(t.Metadata.Generation, 1))
	msg := &iamv1.UserMessage{
		Metadata:    metav1.ObjectMeta{Name: id, UID: id, Tenant: t.Metadata.Name, CreationTimestamp: ts},
		Category:    iamv1.MessageCategoryQuota,
		Title:       fmt.Sprintf("工作区「%s」配额方案已配置", display),
		Content:     fmt.Sprintf("工作区「%s」当前资源配额上限：CPU %s 核 · 内存 %s · 存储 %s · 服务器上限 %s 台。", display, nonEmptyOr(string(q[corev1.ResourceCPU]), "无限制"), nonEmptyOr(string(q[corev1.ResourceMemory]), "无限制"), nonEmptyOr(string(q[corev1.ResourceStorage]), "无限制"), nonEmptyOr(string(q[corev1.ResourceInstances]), "无限制")),
		Workspace:   t.Metadata.Name,
		ActionURL:   "/console/usage",
		ActionLabel: "查看用量配额",
		Timestamp:   ts,
	}
	msg.DefaultMeta()
	return msg
}

func appendInstanceBackupMessages(inst *corev1.Instance, alertedInstances map[string]bool, notifiedTasks taskDedupSet, items *[]*iamv1.UserMessage) {
	appendInstanceLifecycleMessage(inst, alertedInstances, items)
	appendInstanceScheduleMessages(inst, notifiedTasks, items)
	escapedName := url.PathEscape(inst.Metadata.Name)
	escapedTenant := url.QueryEscape(inst.Metadata.Tenant)
	for _, b := range inst.Status.Backups {
		if b.Phase != corev1.BackupPhaseCompleted && b.Phase != corev1.BackupPhaseUnknown {
			continue
		}
		if b.Phase == corev1.BackupPhaseUnknown && isBackupTaskNotified(inst, b, notifiedTasks) {
			continue
		}
		ts := firstNonZeroTime(b.CompletedAt, inst.Status.ObservedAt, inst.Status.StartedAt, &inst.Metadata.CreationTimestamp)
		snapName := cleanBackupDisplayName(inst.Metadata.Name, b.Path, b.UID)
		id := fmt.Sprintf("bak-%s-%s-%s-%s", b.Phase, inst.Metadata.Tenant, inst.Metadata.Name, nonEmptyOr(b.UID, b.OperationID))
		title := fmt.Sprintf("服务器「%s」备份快照已完成", inst.Metadata.Name)
		content := fmt.Sprintf("备份快照「%s」已成功归档并可随时用于恢复。", snapName)
		if b.Phase == corev1.BackupPhaseUnknown {
			title = fmt.Sprintf("[备份异常] 服务器「%s」· %s", inst.Metadata.Name, snapName)
			content = fmt.Sprintf("备份快照「%s」执行中断或未完成，请前往服务器备份页面查看详情或重新发起备份。", snapName)
		}
		msg := &iamv1.UserMessage{
			Metadata:    metav1.ObjectMeta{Name: id, UID: id, Tenant: inst.Metadata.Tenant, CreationTimestamp: ts},
			Category:    iamv1.MessageCategoryBackup,
			Title:       title,
			Content:     content,
			Workspace:   inst.Metadata.Tenant,
			ActionURL:   fmt.Sprintf("/console/servers/%s?tenant=%s&tab=backup", escapedName, escapedTenant),
			ActionLabel: "查看服务器备份",
			Timestamp:   ts,
		}
		msg.DefaultMeta()
		*items = append(*items, msg)
	}
}

func isBackupTaskNotified(inst *corev1.Instance, b corev1.InstanceBackupRecord, notifiedTasks taskDedupSet) bool {
	if b.OperationID != "" && notifiedTasks.backupOps[b.OperationID] {
		return true
	}
	if b.Path != "" && notifiedTasks.backupOps[inst.Metadata.Tenant+"/"+inst.Metadata.Name+"/"+b.Path] {
		return true
	}
	return false
}

func appendInstanceScheduleMessages(inst *corev1.Instance, notifiedTasks taskDedupSet, items *[]*iamv1.UserMessage) {
	if inst == nil || inst.Metadata.DeletionTimestamp != nil || len(inst.Status.ScheduleStatuses) == 0 {
		return
	}
	byID := make(map[string]corev1.InstanceSchedule, len(inst.Spec.Schedules))
	for _, s := range inst.Spec.Schedules {
		byID[s.ID] = s
	}
	escapedName := url.PathEscape(inst.Metadata.Name)
	escapedTenant := url.QueryEscape(inst.Metadata.Tenant)
	for _, st := range inst.Status.ScheduleStatuses {
		if st.LastResult != corev1.ScheduleResultFailed && st.LastResult != corev1.ScheduleResultUnknown {
			continue
		}
		if (st.RunID != "" && notifiedTasks.scheduleRuns[st.RunID]) || notifiedTasks.scheduleRuns[inst.Metadata.Tenant+"/"+inst.Metadata.Name+"/"+st.ID] {
			continue
		}
		sched := byID[st.ID]
		schedTitle := nonEmptyOr(sched.Name, describeScheduleAction(sched.Action))
		ts := firstNonZeroTime(st.ClaimedAt, st.LastRunAt, inst.Status.ObservedAt, &inst.Metadata.CreationTimestamp)
		id := fmt.Sprintf("sch-err-%s-%s-%s", inst.Metadata.Tenant, inst.Metadata.Name, nonEmptyOr(st.RunID, st.ID))
		msg := &iamv1.UserMessage{
			Metadata:    metav1.ObjectMeta{Name: id, UID: id, Tenant: inst.Metadata.Tenant, CreationTimestamp: ts},
			Category:    iamv1.MessageCategoryBackup,
			Title:       fmt.Sprintf("[定时任务异常] 服务器「%s」· %s", inst.Metadata.Name, schedTitle),
			Content:     nonEmptyOr(st.LastOutput, "定时任务执行未完成，请前往定时任务页面检查配置与日志。"),
			Workspace:   inst.Metadata.Tenant,
			ActionURL:   fmt.Sprintf("/console/servers/%s?tenant=%s&tab=schedules", escapedName, escapedTenant),
			ActionLabel: "查看定时任务",
			Timestamp:   ts,
		}
		msg.DefaultMeta()
		*items = append(*items, msg)
	}
}

func firstNonZeroTime(candidates ...*time.Time) time.Time {
	for _, c := range candidates {
		if c != nil && !c.IsZero() {
			return c.UTC()
		}
	}
	return time.Now().UTC()
}

func appendInstanceLifecycleMessage(inst *corev1.Instance, alertedInstances map[string]bool, items *[]*iamv1.UserMessage) {
	if inst.Metadata.DeletionTimestamp != nil {
		return
	}
	ts := inst.Metadata.CreationTimestamp
	if inst.Status.StartedAt != nil && !inst.Status.StartedAt.IsZero() {
		ts = *inst.Status.StartedAt
	}
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	escapedName := url.PathEscape(inst.Metadata.Name)
	escapedTenant := url.QueryEscape(inst.Metadata.Tenant)
	switch inst.Status.Phase {
	case corev1.InstancePhaseRunning:
		id := fmt.Sprintf("srv-ready-%s-%s", inst.Metadata.Tenant, inst.Metadata.Name)
		msg := &iamv1.UserMessage{
			Metadata:    metav1.ObjectMeta{Name: id, UID: id, Tenant: inst.Metadata.Tenant, CreationTimestamp: ts},
			Category:    iamv1.MessageCategorySystem,
			Title:       fmt.Sprintf("服务器「%s」已部署就绪", inst.Metadata.Name),
			Content:     fmt.Sprintf("服务器「%s」已完成启动并正在运行，可前往查看连接地址与实时监控。", inst.Metadata.Name),
			Workspace:   inst.Metadata.Tenant,
			ActionURL:   fmt.Sprintf("/console/servers/%s?tenant=%s", escapedName, escapedTenant),
			ActionLabel: "查看服务器详情",
			Timestamp:   ts,
		}
		msg.DefaultMeta()
		*items = append(*items, msg)
	case corev1.InstancePhaseFailed:
		if alertedInstances[inst.Metadata.Tenant+"/"+inst.Metadata.Name] {
			return
		}
		id := fmt.Sprintf("srv-fail-%s-%s-g%d", inst.Metadata.Tenant, inst.Metadata.Name, max(inst.Metadata.Generation, 1))
		msg := &iamv1.UserMessage{
			Metadata:    metav1.ObjectMeta{Name: id, UID: id, Tenant: inst.Metadata.Tenant, CreationTimestamp: ts},
			Category:    iamv1.MessageCategorySystem,
			Title:       fmt.Sprintf("服务器「%s」运行异常", inst.Metadata.Name),
			Content:     fmt.Sprintf("服务器「%s」当前处于异常状态，请前往控制台查看日志排查原因。", inst.Metadata.Name),
			Workspace:   inst.Metadata.Tenant,
			ActionURL:   fmt.Sprintf("/console/servers/%s?tenant=%s&tab=console", escapedName, escapedTenant),
			ActionLabel: "查看控制台日志",
			Timestamp:   ts,
		}
		msg.DefaultMeta()
		*items = append(*items, msg)
	}
}

func cleanBackupDisplayName(instanceName, rawPath, fallbackUID string) string {
	return instancemodel.FormatBackupDisplayName(instanceName, rawPath, fallbackUID)
}

func applyReadStates(u *iamv1.User, items []*iamv1.UserMessage) {
	readSet := parseReadIDSet(u.Metadata.Annotations[iamv1.AnnotationInboxReadIDs])
	var readAllBefore time.Time
	if raw := strings.TrimSpace(u.Metadata.Annotations[iamv1.AnnotationInboxReadAllAt]); raw != "" {
		readAllBefore, _ = time.Parse(time.RFC3339Nano, raw)
	}
	for _, item := range items {
		if readSet[item.Metadata.Name] {
			item.Read = true
		} else if !readAllBefore.IsZero() && !item.Timestamp.After(readAllBefore) {
			item.Read = true
		}
	}
}

func subjectListHasUser(subjects []iamv1.Subject, username string) bool {
	for _, s := range subjects {
		if (s.Kind == "" || s.Kind == iamv1.SubjectKindUser) && s.Name == username {
			return true
		}
	}
	return false
}

func describeTenantRoleName(role string) string {
	switch role {
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

func describeGlobalRoleName(role string) string {
	switch role {
	case "platform-admin":
		return "平台管理员"
	case "authenticated-user":
		return "认证用户"
	default:
		return role
	}
}

func nonEmptyOr(v, fallback string) string {
	if strings.TrimSpace(v) != "" {
		return v
	}
	return fallback
}

func parseReadIDSet(raw string) map[string]bool {
	set := make(map[string]bool)
	if strings.TrimSpace(raw) == "" {
		return set
	}
	var ids []string
	if json.Unmarshal([]byte(raw), &ids) == nil {
		for _, id := range ids {
			if id != "" {
				set[id] = true
			}
		}
	}
	return set
}

func encodeReadIDSet(set map[string]bool) string {
	ids := make([]string, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	b, _ := json.Marshal(ids)
	return string(b)
}

func parseCustomMessages(raw string) []iamv1.UserMessage {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var list []iamv1.UserMessage
	_ = json.Unmarshal([]byte(raw), &list)
	return list
}
