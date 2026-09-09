package paymentingress

import (
	"context"
	"errors"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/commerce"
)

type verifierFixture struct {
	capture commerce.CapturedPayment
	err     error
}

func (f verifierFixture) Verify(context.Context, Notification) (commerce.CapturedPayment, error) {
	return f.capture, f.err
}

type recorderFixture struct {
	capture commerce.CapturedPayment
	calls   int
}

func (f *recorderFixture) RecordCapturedPayment(_ context.Context, capture commerce.CapturedPayment) (commerce.PaymentReceipt, error) {
	f.capture = capture
	f.calls++
	return commerce.PaymentReceipt{ID: "receipt", OrderID: capture.OrderID, Disposition: "applied"}, nil
}

func TestCaptureRecordsOnlyVerifierProducedPayment(t *testing.T) {
	capture := commerce.CapturedPayment{Provider: "provider", MerchantID: "merchant", TransactionID: "transaction", EventID: "event", OrderID: "order", AmountMinor: 100, Currency: "CNY", PaidAtMS: 1}
	recorder := &recorderFixture{}
	service, err := New(verifierFixture{capture: capture}, recorder)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := service.Capture(context.Background(), Notification{Body: []byte(`{"untrusted":true}`)})
	if err != nil || receipt.ID != "receipt" || recorder.calls != 1 || recorder.capture != capture {
		t.Fatalf("capture: %+v %+v %v", receipt, recorder, err)
	}
}

func TestCaptureDoesNotRecordFailedOrInvalidVerification(t *testing.T) {
	recorder := &recorderFixture{}
	failed, err := New(verifierFixture{err: ErrVerificationFailed}, recorder)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := failed.Capture(context.Background(), Notification{Body: []byte(`{}`)}); !errors.Is(err, ErrVerificationFailed) || recorder.calls != 0 {
		t.Fatalf("verification failure recorded: calls=%d err=%v", recorder.calls, err)
	}
	invalid, err := New(verifierFixture{capture: commerce.CapturedPayment{}}, recorder)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := invalid.Capture(context.Background(), Notification{Body: []byte(`{}`)}); !errors.Is(err, ErrVerificationFailed) || recorder.calls != 0 {
		t.Fatalf("invalid capture recorded: calls=%d err=%v", recorder.calls, err)
	}
}
