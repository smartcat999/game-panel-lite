package driver

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
)

var webhookHTTPClient = resty.New().SetTimeout(5 * time.Second)

// PostJSONWebhook posts JSON payload to targetURL with GamePanel delivery/HMAC signature headers.
func PostJSONWebhook(ctx context.Context, targetURL, secret, deliveryID, tsSec string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req := webhookHTTPClient.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetHeader("X-GamePanel-Delivery-ID", deliveryID).
		SetHeader("X-GamePanel-Timestamp", tsSec).
		SetBody(body)
	if s := strings.TrimSpace(secret); s != "" {
		mac := hmac.New(sha256.New, []byte(s))
		_, _ = mac.Write([]byte(tsSec + "." + string(body)))
		req.SetHeader("X-GamePanel-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}
	resp, err := req.Post(targetURL)
	if err != nil {
		return err
	}
	return checkWebhookResponse(resp)
}

func checkWebhookResponse(resp *resty.Response) error {
	respBytes := resp.Body()
	if len(respBytes) > 4096 {
		respBytes = respBytes[:4096]
	}
	if resp.StatusCode() < 200 || resp.StatusCode() >= 300 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode(), strings.TrimSpace(string(respBytes)))
	}
	return ValidateUpstreamJSONStatus(respBytes)
}

// ValidateUpstreamJSONStatus rejects HTTP 200 responses carrying non-zero upstream errcode/code fields.
func ValidateUpstreamJSONStatus(respBytes []byte) error {
	if len(respBytes) == 0 || respBytes[0] != '{' {
		return nil
	}
	var parsed map[string]any
	if json.Unmarshal(respBytes, &parsed) != nil {
		return nil
	}
	if codeVal, ok := parsed["errcode"]; ok {
		if num, ok := codeVal.(float64); ok && num != 0 {
			return fmt.Errorf("upstream errcode=%d errmsg=%v", int(num), parsed["errmsg"])
		}
	}
	if codeVal, ok := parsed["code"]; ok {
		if num, ok := codeVal.(float64); ok && num != 0 {
			return fmt.Errorf("upstream code=%d msg=%v", int(num), parsed["msg"])
		}
	}
	return nil
}
