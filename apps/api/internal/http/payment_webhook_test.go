package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/commerce"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/paymentingress"
)

type paymentCaptureFixture struct {
	notification paymentingress.Notification
	err          error
}

func (f *paymentCaptureFixture) Capture(_ context.Context, notification paymentingress.Notification) (commerce.PaymentReceipt, error) {
	f.notification = paymentingress.Notification{
		Body:    append([]byte(nil), notification.Body...),
		Headers: make(map[string][]string, len(notification.Headers)),
	}
	for name, values := range notification.Headers {
		f.notification.Headers[name] = append([]string(nil), values...)
	}
	return commerce.PaymentReceipt{ID: "receipt", OrderID: "order", Disposition: "applied"}, f.err
}

func TestPaymentWebhookRequiresConfiguredVerifier(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/commerce/payments/webhook", strings.NewReader(`{"forged":true}`))
	response := httptest.NewRecorder()
	new(Handler).handlePaymentWebhook(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("unverified payment accepted: %d %s", response.Code, response.Body.String())
	}
}

func TestPaymentWebhookPassesOpaqueNotificationToVerifier(t *testing.T) {
	captures := &paymentCaptureFixture{}
	handler := new(Handler).WithPaymentCaptures(captures)
	request := httptest.NewRequest(http.MethodPost, "/api/commerce/payments/webhook", strings.NewReader(`{"provider_payload":true}`))
	request.Header.Set("Payment-Signature", "provider-signature")
	response := httptest.NewRecorder()
	handler.handlePaymentWebhook(response, request)
	if response.Code != http.StatusOK || string(captures.notification.Body) != `{"provider_payload":true}` || len(captures.notification.Headers["Payment-Signature"]) != 1 {
		t.Fatalf("verified notification: %d %s %+v", response.Code, response.Body.String(), captures.notification)
	}

	captures.err = paymentingress.ErrVerificationFailed
	response = httptest.NewRecorder()
	handler.handlePaymentWebhook(response, httptest.NewRequest(http.MethodPost, "/api/commerce/payments/webhook", strings.NewReader(`{}`)))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("verification failure status: %d %s", response.Code, response.Body.String())
	}
	captures.err = errors.New("recorder unavailable")
	response = httptest.NewRecorder()
	handler.handlePaymentWebhook(response, httptest.NewRequest(http.MethodPost, "/api/commerce/payments/webhook", strings.NewReader(`{}`)))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("recorder failure status: %d %s", response.Code, response.Body.String())
	}
}
