package instance

import (
	"testing"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
)

func TestScheduleOccurrenceManualTrigger(t *testing.T) {
	now := time.Now().UTC()
	triggered := now.Add(-5 * time.Second)
	inst := &corev1.Instance{
		Status: corev1.InstanceStatus{
			ScheduleStatuses: []corev1.InstanceScheduleStatus{
				{ID: "s1", LastResult: corev1.ScheduleResultSucceeded},
			},
		},
	}
	sched := corev1.InstanceSchedule{
		ID:              "s1",
		Enabled:         false,
		Action:          corev1.ScheduleActionCommand,
		ManualTriggerAt: &triggered,
	}
	due, ok := scheduleOccurrence(inst, sched, now)
	if !ok || !due.Equal(triggered) {
		t.Fatalf("expected manual trigger due=%v ok=true, got due=%v ok=%v", triggered, due, ok)
	}

	inst.Status.ScheduleStatuses[0].LastRunAt = &triggered
	if _, ok := scheduleOccurrence(inst, sched, now); ok {
		t.Fatalf("expected already-executed manualTriggerAt not to trigger again")
	}
}

func TestScheduleClaimTTLAndTaskID(t *testing.T) {
	inst := &corev1.Instance{
		Spec: corev1.InstanceSpec{
			Schedules: []corev1.InstanceSchedule{
				{ID: "bak", Action: corev1.ScheduleActionBackup},
				{ID: "cmd", Action: corev1.ScheduleActionCommand},
			},
		},
	}
	if ttl := scheduleClaimTTL(inst, "bak"); ttl != 11*time.Minute {
		t.Fatalf("expected 11m TTL for backup schedule, got %v", ttl)
	}
	if ttl := scheduleClaimTTL(inst, "cmd"); ttl != scheduleUnknownAfter {
		t.Fatalf("expected %v TTL for command schedule, got %v", scheduleUnknownAfter, ttl)
	}
	if got := scheduleTaskID("run-1"); got != "sched-run-1" {
		t.Fatalf("expected sched-run-1, got %q", got)
	}
	if got := scheduleTaskID("sched-run-2"); got != "sched-run-2" {
		t.Fatalf("expected sched-run-2 unchanged, got %q", got)
	}
}
