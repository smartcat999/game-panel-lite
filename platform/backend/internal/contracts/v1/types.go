package v1

import "time"

type WorkspaceID string
type UserID string
type IdentityID string
type MembershipID string
type PlanID string
type PlanVersionID string
type InstanceRevisionID string
type PlacementID string
type OrderID string
type PaymentID string
type EntitlementID string
type BackupRequestID string
type LogicalInstanceID string
type RegionID string
type RegionalDeploymentID string
type NodeID string
type CommandID string
type EventID string
type IdempotencyKey string

type Envelope[T any] struct {
	SchemaVersion  int            `json:"schemaVersion"`
	MessageID      EventID        `json:"messageId"`
	MessageType    string         `json:"messageType"`
	OccurredAt     time.Time      `json:"occurredAt"`
	IdempotencyKey IdempotencyKey `json:"idempotencyKey"`
	Payload        T              `json:"payload"`
}

type CommandIdentity struct {
	CommandID      CommandID      `json:"commandId"`
	IdempotencyKey IdempotencyKey `json:"idempotencyKey"`
}
