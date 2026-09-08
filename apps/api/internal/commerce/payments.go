package commerce

import "errors"

var ErrInvalidPayment = errors.New("invalid captured payment")
var ErrPaymentConflict = errors.New("payment identity has conflicting data")

// CapturedPayment must come from a trusted adapter after signature, merchant,
// environment and captured-status verification. It is not a public request DTO.
// Shape validation alone does not establish any of those facts.
type CapturedPayment struct {
	Provider      string
	MerchantID    string
	TransactionID string
	EventID       string
	OrderID       string
	AmountMinor   int64
	Currency      string
	PaidAtMS      int64
}

func (p CapturedPayment) Validate() error {
	if !validID(p.Provider) || !validID(p.MerchantID) || !validID(p.TransactionID) || !validID(p.EventID) || !validID(p.OrderID) || p.AmountMinor <= 0 || p.PaidAtMS <= 0 || len(p.Currency) != 3 {
		return ErrInvalidPayment
	}
	for _, c := range p.Currency {
		if c < 'A' || c > 'Z' {
			return ErrInvalidPayment
		}
	}
	return nil
}

// PaymentReceipt records local application of a verified capture. Review means
// funds were reported but require reconciliation/refund; it grants no service.
type PaymentReceipt struct {
	ID           string
	OrderID      string
	Disposition  string
	Reason       string
	RecordedAtMS int64
}
