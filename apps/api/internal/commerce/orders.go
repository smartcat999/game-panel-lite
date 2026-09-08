package commerce

import "errors"

var ErrOrderNotCancellable = errors.New("order is not awaiting payment")

var ErrInvalidOrder = errors.New("invalid prepaid order")
var ErrOrderConflict = errors.New("order request has different parameters")
var ErrOrderUnavailable = errors.New("instance is unavailable for this purchase")

type OrderRequest struct {
	OrganizationID string `json:"organizationId"`
	ServerID       string `json:"serverId"`
	PlanID         string `json:"planId"`
	PlanVersion    int64  `json:"planVersion"`
	Periods        int64  `json:"periods"`
	IdempotencyKey string `json:"idempotencyKey"`
}

func (r OrderRequest) Validate() error {
	if !validID(r.OrganizationID) || !validID(r.ServerID) || !validID(r.PlanID) || !validID(r.IdempotencyKey) || r.PlanVersion < 1 || r.Periods < 1 {
		return ErrInvalidOrder
	}
	return nil
}

// Order captures purchased terms for one logical instance. Pending orders are
// not payment receipts, subscriptions, resource reservations or execution grants.
type Order struct {
	CancelReason   string `json:"cancelReason,omitempty"`
	CancelledAtMS  int64  `json:"cancelledAtMs,omitempty"`
	CancelledBy    string `json:"cancelledBy,omitempty"`
	ID             string `json:"id"`
	OrganizationID string `json:"organizationId"`
	ServerID       string `json:"serverId"`
	RevisionID     string `json:"revisionId"`
	PlacementEpoch int64  `json:"placementEpoch"`
	Quote          Quote  `json:"quote"`
	Status         string `json:"status"`
	CreatedAtMS    int64  `json:"createdAtMs"`
	ExpiresAtMS    int64  `json:"expiresAtMs"`
}
