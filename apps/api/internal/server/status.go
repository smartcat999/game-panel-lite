package server

import (
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func initialStatus(now time.Time) domain.ServerRuntimeStatus {
	return domain.ServerRuntimeStatus{
		Phase:            domain.PhasePending,
		ActualState:      domain.ActualUnknown,
		LastTransitionAt: now,
	}
}

func setPhase(status *domain.ServerRuntimeStatus, phase domain.ServerPhase, now time.Time) {
	if status.Phase != phase {
		status.Phase = phase
		status.LastTransitionAt = now
	}
	if phase == domain.PhasePending || phase == domain.PhaseReconciling {
		status.LastError = ""
	}
}

func markPending(status *domain.ServerRuntimeStatus, now time.Time) {
	setPhase(status, domain.PhasePending, now)
}

func markDeleting(status *domain.ServerRuntimeStatus, now time.Time) {
	setPhase(status, domain.PhaseDeleting, now)
}
