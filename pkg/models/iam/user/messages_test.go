package user

import (
	"context"
	"testing"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
)

func TestBuildAlertInboxMessageInstanceAndNodeTransitions(t *testing.T) {
	now := time.Date(2026, 11, 11, 13, 30, 0, 0, time.UTC)

	firingEv := &corev1.Event{
		Metadata:  metav1.ObjectMeta{Name: "alt-dlv-001", Tenant: "studio-alpha", CreationTimestamp: now},
		Action:    corev1.AlertActionTransition,
		Timestamp: now,
		Attributes: map[string]string{
			"scope":       corev1.AlertScopeTenant,
			"tenant":      "studio-alpha",
			"state":       corev1.AlertStateFiring,
			"severity":    corev1.AlertSeverityCritical,
			"targetKind":  "instances",
			"targetName":  "alpha-mc-01",
			"ruleTitle":   "服务器运行状态异常",
			"metricValue": "OOMKilled",
			"message":     "当前指标 OOMKilled — 容器触发内存上限 OOMKilled (ExitCode 137)。",
		},
	}
	msg := buildAlertInboxMessage(firingEv)
	if msg == nil {
		t.Fatal("expected non-nil inbox message")
	}
	if msg.Category != iamv1.MessageCategoryAlert {
		t.Fatalf("expected category %q, got %q", iamv1.MessageCategoryAlert, msg.Category)
	}
	if msg.Title != "[告警触发] 服务器「alpha-mc-01」· 服务器运行状态异常" {
		t.Fatalf("unexpected firing title: %q", msg.Title)
	}
	if msg.Workspace != "studio-alpha" {
		t.Fatalf("expected workspace studio-alpha, got %q", msg.Workspace)
	}
	wantURL := "/console/servers/alpha-mc-01?tenant=studio-alpha&tab=console"
	if msg.ActionURL != wantURL || msg.ActionLabel != "查看服务器控制台" {
		t.Fatalf("unexpected action (%q, %q)", msg.ActionURL, msg.ActionLabel)
	}

	nodeEv := &corev1.Event{
		Metadata:  metav1.ObjectMeta{Name: "alt-dlv-002", CreationTimestamp: now},
		Action:    corev1.AlertActionTransition,
		Timestamp: now,
		Attributes: map[string]string{
			"scope":      corev1.AlertScopePlatform,
			"state":      corev1.AlertStateResolved,
			"targetKind": "nodes",
			"targetName": "node-sh-01",
			"ruleTitle":  "计算节点心跳恢复",
			"message":    "节点已恢复正常上报。",
		},
	}
	nodeMsg := buildAlertInboxMessage(nodeEv)
	if nodeMsg.Title != "[告警恢复] 节点「node-sh-01」· 计算节点心跳恢复" {
		t.Fatalf("unexpected resolved node title: %q", nodeMsg.Title)
	}
	if nodeMsg.Workspace != "" || nodeMsg.ActionURL != "/console/admin/nodes" {
		t.Fatalf("unexpected platform node routing: workspace=%q url=%q", nodeMsg.Workspace, nodeMsg.ActionURL)
	}
}

func TestBuildAlertInboxScopesIsolationAndDedup(t *testing.T) {
	ctx := context.Background()
	memberScopes := buildAlertInboxScopes(ctx, nil, map[string]bool{"team-a": true}, false)
	if len(memberScopes) != 1 || memberScopes[0] != "team-a" {
		t.Fatalf("regular member must not receive platform scope, got %v", memberScopes)
	}
	adminScopes := buildAlertInboxScopes(ctx, nil, map[string]bool{"admin": true}, true)
	if len(adminScopes) != 2 || adminScopes[0] != "" || adminScopes[1] != "admin" {
		t.Fatalf("platform admin must receive platform + workspace scopes, got %v", adminScopes)
	}

	failedInst := &corev1.Instance{
		Metadata: metav1.ObjectMeta{Name: "demo-mc-01", Tenant: "admin", Generation: 2},
		Status:   corev1.InstanceStatus{Phase: corev1.InstancePhaseFailed},
	}
	var suppressed []*iamv1.UserMessage
	appendInstanceLifecycleMessage(failedInst, map[string]bool{"admin/demo-mc-01": true}, &suppressed)
	if len(suppressed) != 0 {
		t.Fatalf("expected duplicate generic failed message to be suppressed, got %d", len(suppressed))
	}
	var fallback []*iamv1.UserMessage
	appendInstanceLifecycleMessage(failedInst, map[string]bool{}, &fallback)
	if len(fallback) != 1 || fallback[0].Metadata.Name != "srv-fail-admin-demo-mc-01-g2" {
		t.Fatalf("expected fallback failed message when no alert record exists, got %+v", fallback)
	}
}

func TestBuildTaskFailureInboxMessageBackupAndRestore(t *testing.T) {
	now := time.Date(2026, 11, 12, 2, 0, 0, 0, time.UTC)

	quotaEv := &corev1.Event{
		Metadata:  metav1.ObjectMeta{Name: "tsk-bak-001", Tenant: "admin", CreationTimestamp: now},
		Action:    corev1.TaskActionFailed,
		Timestamp: now,
		Attributes: map[string]string{
			"tenant": "admin", "instanceName": "demo-mc-01",
			"taskKind": corev1.TaskKindBackupArchive, "taskName": "自动备份 11-12 04:00",
			"operationID": "backup:op-1", "path": "tenants/admin/instances/demo-mc-01/backup-op-1.tar.gz",
			"quotaBlocked": "true",
			"message":      "工作区备份数量上限或备份存储配额已满，请清理旧备份快照或联系管理员扩容配额。",
		},
	}
	bakMsg := buildTaskFailureInboxMessage(quotaEv)
	if bakMsg == nil || bakMsg.Category != iamv1.MessageCategoryBackup {
		t.Fatalf("expected backup category message, got %+v", bakMsg)
	}
	if bakMsg.Title != "[备份受阻] 服务器「demo-mc-01」· 自动备份 11-12 04:00" {
		t.Fatalf("unexpected backup quota title: %q", bakMsg.Title)
	}
	if bakMsg.ActionURL != "/console/servers/demo-mc-01?tenant=admin&tab=backup" || bakMsg.ActionLabel != "查看服务器备份" {
		t.Fatalf("unexpected backup action: %q (%q)", bakMsg.ActionURL, bakMsg.ActionLabel)
	}

	restoreEv := &corev1.Event{
		Metadata:  metav1.ObjectMeta{Name: "tsk-rst-001", Tenant: "admin", CreationTimestamp: now},
		Action:    corev1.TaskActionFailed,
		Timestamp: now,
		Attributes: map[string]string{
			"tenant": "admin", "instanceName": "demo-mc-01",
			"taskKind": corev1.TaskKindBackupRestore, "taskName": "开荒前备份",
			"message": "对象存储中未找到目标备份归档文件，可能已被清理或上传未完成。",
		},
	}
	rstMsg := buildTaskFailureInboxMessage(restoreEv)
	if rstMsg == nil || rstMsg.Title != "[备份异常] 服务器「demo-mc-01」· 存档恢复失败" {
		t.Fatalf("unexpected restore failure message: %+v", rstMsg)
	}
	if rstMsg.Content != "存档快照「开荒前备份」恢复未完成：对象存储中未找到目标备份归档文件，可能已被清理或上传未完成。" {
		t.Fatalf("unexpected restore failure content: %q", rstMsg.Content)
	}
}

func TestBuildTaskFailureInboxMessageSchedule(t *testing.T) {
	now := time.Date(2026, 11, 12, 2, 0, 0, 0, time.UTC)
	schedEv := &corev1.Event{
		Metadata:  metav1.ObjectMeta{Name: "tsk-sch-001", Tenant: "admin", CreationTimestamp: now},
		Action:    corev1.TaskActionFailed,
		Timestamp: now,
		Attributes: map[string]string{
			"tenant": "admin", "instanceName": "demo-mc-01",
			"taskKind": corev1.TaskKindSchedule, "taskName": "每日凌晨重启",
			"runID": "run-abc", "scheduleID": "sched-restart",
			"scheduleAction": corev1.ScheduleActionRestart,
			"message":        "任务执行响应超时，请检查节点网络、磁盘负载或稍后重试。",
		},
	}
	schedMsg := buildTaskFailureInboxMessage(schedEv)
	if schedMsg == nil || schedMsg.Title != "[定时任务异常] 服务器「demo-mc-01」· 每日凌晨重启" {
		t.Fatalf("unexpected schedule failure message: %+v", schedMsg)
	}
	if schedMsg.ActionURL != "/console/servers/demo-mc-01?tenant=admin&tab=schedules" || schedMsg.ActionLabel != "查看定时任务" {
		t.Fatalf("unexpected schedule action: %q (%q)", schedMsg.ActionURL, schedMsg.ActionLabel)
	}
}

func TestTaskFailureStatusFallbackAndDedup(t *testing.T) {
	now := time.Date(2026, 11, 12, 2, 0, 0, 0, time.UTC)
	dedup := newTaskDedupSet()
	recordTaskDedup(&corev1.Event{Attributes: map[string]string{
		"tenant": "admin", "instanceName": "demo-mc-01", "operationID": "backup:op-1",
	}}, &dedup)
	recordTaskDedup(&corev1.Event{Attributes: map[string]string{
		"tenant": "admin", "instanceName": "demo-mc-01", "runID": "run-abc", "scheduleID": "sched-restart",
	}}, &dedup)

	createdAt := now.Add(-7 * 24 * time.Hour)
	observedAt := now.Add(-time.Minute)
	claimed2 := now.Add(-30 * time.Second)
	inst := &corev1.Instance{
		Metadata: metav1.ObjectMeta{Name: "demo-mc-01", Tenant: "admin", CreationTimestamp: createdAt},
		Spec: corev1.InstanceSpec{
			Schedules: []corev1.InstanceSchedule{
				{ID: "sched-restart", Name: "每日凌晨重启", Action: corev1.ScheduleActionRestart, Enabled: true},
				{ID: "sched-save", Name: "定时保存世界", Action: corev1.ScheduleActionCommand, Enabled: true},
			},
		},
		Status: corev1.InstanceStatus{
			Phase:      corev1.InstancePhaseRunning,
			ObservedAt: &observedAt,
			Backups: []corev1.InstanceBackupRecord{
				{OperationID: "backup:op-1", Phase: corev1.BackupPhaseUnknown, Path: "tenants/admin/instances/demo-mc-01/backup-op-1.tar.gz"},
				{OperationID: "backup:op-2", Phase: corev1.BackupPhaseUnknown, Path: "backups/demo-mc-01-20261112-040000--e887aae58aa8e5a487e4bbbd2031312d31322030343a3030.tar.gz"},
			},
			ScheduleStatuses: []corev1.InstanceScheduleStatus{
				{ID: "sched-restart", RunID: "run-abc", ClaimedAt: &observedAt, LastResult: corev1.ScheduleResultUnknown, LastOutput: "timeout"},
				{ID: "sched-save", RunID: "run-def", ClaimedAt: &claimed2, LastResult: corev1.ScheduleResultFailed, LastOutput: "rcon connection refused"},
			},
		},
	}

	var items []*iamv1.UserMessage
	appendInstanceBackupMessages(inst, map[string]bool{}, dedup, &items)
	if len(items) != 3 {
		t.Fatalf("expected 3 deduplicated status messages, got %d: %+v", len(items), items)
	}
	if items[1].Title != "[定时任务异常] 服务器「demo-mc-01」· 定时保存世界" || !items[1].Timestamp.Equal(claimed2) {
		t.Fatalf("unexpected fallback schedule message: %+v", items[1])
	}
	if items[2].Title != "[备份异常] 服务器「demo-mc-01」· 自动备份 11-12 04:00" || !items[2].Timestamp.Equal(observedAt) {
		t.Fatalf("unexpected fallback backup message (hex decode + observedAt timestamp): %+v", items[2])
	}
}
