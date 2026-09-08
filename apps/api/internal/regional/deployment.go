package regional

import "errors"

var ErrDeploymentConflict = errors.New("regional deployment identity or version conflicts")

// Deployment records regional desired configuration after asset preparation.
// It grants no execution authority and has no Node allocation until scheduling.
// RevisionOperationID references the protected snapshot in the regional task.
type Deployment struct {
	ID                  string
	OrganizationID      string
	ServerID            string
	PlacementEpoch      int64
	RevisionID          string
	RevisionOperationID string
	SpecGeneration      int64
	IntentVersion       int64
	DesiredState        string
	Status              string
}
