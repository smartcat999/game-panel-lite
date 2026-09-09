package commerce

import "errors"

var ErrSubscriptionUnavailable = errors.New("paid order cannot provision a subscription")
var ErrSubscriptionConflict = errors.New("instance already has a different subscription")
var ErrSubscriptionExpired = errors.New("server subscription has expired, please renew to start")

// Subscription records the purchased service, independently of user run/stop
// intent. pending_activation preserves the full purchased duration until a
// verified delivery activates service. It is not a Node execution permission.
type Subscription struct {
	ID             string
	OrganizationID string
	ServerID       string
	OrderID        string
	PaymentID      string
	RevisionID     string
	PlacementEpoch int64
	Quote          Quote
	Status         string
	CreatedAtMS    int64
}
