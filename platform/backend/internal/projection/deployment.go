package projection

import (
	"context"
	"time"

	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/instancecontrol"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/messaging"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/persistence"
)

type DeploymentObserved struct {
	MessageID            contract.EventID
	LogicalInstanceID    contract.LogicalInstanceID
	RegionalDeploymentID contract.RegionalDeploymentID
	RegionID             contract.RegionID
	Sequence             int64
	ObservedState        string
	ObservedAt           time.Time
}

type Module struct {
	inbox     *messaging.Module
	instances *instancecontrol.Module
}

func New(inbox *messaging.Module, instances *instancecontrol.Module) *Module {
	return &Module{inbox: inbox, instances: instances}
}

func (m *Module) Consume(ctx context.Context, event DeploymentObserved, receivedAt time.Time) (bool, error) {
	return m.inbox.HandleOnce(event.MessageID, receivedAt, func() error {
		m.instances.ApplyDeploymentSummary(ctx, instancecontrol.DeploymentSummary{LogicalInstanceID: event.LogicalInstanceID, RegionalDeploymentID: event.RegionalDeploymentID, RegionID: event.RegionID, Sequence: event.Sequence, ObservedState: event.ObservedState, ObservedAt: event.ObservedAt})
		return nil
	})
}

type Postgres struct {
	inbox     *messaging.Postgres
	instances *instancecontrol.Postgres
}

func NewPostgres(inbox *messaging.Postgres, instances *instancecontrol.Postgres) *Postgres {
	return &Postgres{inbox: inbox, instances: instances}
}

func (p *Postgres) Consume(ctx context.Context, event DeploymentObserved, receivedAt time.Time) (bool, error) {
	return p.inbox.HandleOnce(ctx, event.MessageID, "deployment.observed.v1", receivedAt, func(query persistence.DBTX) error {
		_, err := p.instances.ApplyDeploymentSummary(ctx, query, instancecontrol.DeploymentSummary{LogicalInstanceID: event.LogicalInstanceID, RegionalDeploymentID: event.RegionalDeploymentID, RegionID: event.RegionID, Sequence: event.Sequence, ObservedState: event.ObservedState, ObservedAt: event.ObservedAt})
		return err
	})
}
