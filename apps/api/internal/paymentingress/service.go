// Package paymentingress verifies provider notifications before recording money movement.
package paymentingress

import (
	"context"
	"errors"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/commerce"
)

var ErrInvalidNotification = errors.New("invalid payment notification")
var ErrVerificationFailed = errors.New("payment notification verification failed")

type Notification struct {
	Body    []byte
	Headers map[string][]string
}

type Verifier interface {
	Verify(context.Context, Notification) (commerce.CapturedPayment, error)
}

type Recorder interface {
	RecordCapturedPayment(context.Context, commerce.CapturedPayment) (commerce.PaymentReceipt, error)
}

type Service struct {
	verifier Verifier
	recorder Recorder
}

func New(verifier Verifier, recorder Recorder) (*Service, error) {
	if verifier == nil || recorder == nil {
		return nil, errors.New("payment verifier and recorder are required")
	}
	return &Service{verifier: verifier, recorder: recorder}, nil
}

// Capture accepts only a verifier-produced capture. HTTP fields and client time
// never become accounting facts without a provider adapter authenticating them.
func (s *Service) Capture(ctx context.Context, notification Notification) (commerce.PaymentReceipt, error) {
	if err := ctx.Err(); err != nil {
		return commerce.PaymentReceipt{}, err
	}
	if len(notification.Body) == 0 {
		return commerce.PaymentReceipt{}, ErrInvalidNotification
	}
	capture, err := s.verifier.Verify(ctx, notification)
	if err != nil {
		return commerce.PaymentReceipt{}, err
	}
	if capture.Validate() != nil {
		return commerce.PaymentReceipt{}, ErrVerificationFailed
	}
	return s.recorder.RecordCapturedPayment(ctx, capture)
}
