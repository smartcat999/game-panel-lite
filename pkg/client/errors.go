package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-resty/resty/v2"
)

// Status represents the standard JSON error or status payload returned by the API server.
type Status struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Status     string `json:"status"`
	Code       int    `json:"code"`
	Message    string `json:"message"`
	Reason     string `json:"reason,omitempty"`
}

// APIError represents an HTTP error response from the API server.
type APIError struct {
	StatusCode int
	Message    string
	Status     *Status
}

func (e *APIError) Error() string {
	msg := e.Message
	if msg == "" && e.Status != nil {
		msg = e.Status.Message
	}
	statusText := http.StatusText(e.StatusCode)
	if statusText == "" {
		statusText = fmt.Sprintf("%d", e.StatusCode)
	}
	if msg != "" {
		return fmt.Sprintf("Error from server (%s %d): %s", statusText, e.StatusCode, msg)
	}
	return fmt.Sprintf("Error from server (%s %d)", statusText, e.StatusCode)
}

// FromRestyResponse parses a resty HTTP error response (status code >= 400) into an APIError.
func FromRestyResponse(resp *resty.Response) error {
	if resp == nil {
		return errors.New("nil response")
	}
	if resp.StatusCode() < 400 {
		return nil
	}
	return parseAPIErrorBytes(resp.StatusCode(), resp.Body())
}

// FromResponse parses an HTTP error response (status code >= 400) into an APIError.
func FromResponse(resp *http.Response) error {
	if resp == nil {
		return errors.New("nil response")
	}
	if resp.StatusCode < 400 {
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return &APIError{
			StatusCode: resp.StatusCode,
			Message:    fmt.Sprintf("HTTP %d: failed to read error body: %v", resp.StatusCode, err),
		}
	}
	return parseAPIErrorBytes(resp.StatusCode, body)
}

func parseAPIErrorBytes(statusCode int, body []byte) error {
	var status Status
	if err := json.Unmarshal(body, &status); err == nil && (status.Message != "" || status.Code != 0) {
		msg := status.Message
		if msg == "" {
			msg = fmt.Sprintf("HTTP %d", statusCode)
		}
		return &APIError{
			StatusCode: statusCode,
			Message:    msg,
			Status:     &status,
		}
	}
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		trimmed = http.StatusText(statusCode)
	}
	return &APIError{
		StatusCode: statusCode,
		Message:    trimmed,
	}
}

// IsNotFound returns true if the error indicates a 404 Not Found condition.
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == http.StatusNotFound
	}
	low := strings.ToLower(err.Error())
	return strings.Contains(low, "not found") || strings.Contains(low, "404")
}

// IsConflict returns true if the error indicates a 409 Conflict condition (or already exists).
func IsConflict(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == http.StatusConflict
	}
	low := strings.ToLower(err.Error())
	return strings.Contains(low, "conflict") || strings.Contains(low, "already exists") || strings.Contains(low, "409")
}

// IsUnauthorized returns true if the error indicates a 401 Unauthorized condition.
func IsUnauthorized(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == http.StatusUnauthorized
	}
	low := strings.ToLower(err.Error())
	return strings.Contains(low, "unauthorized") || strings.Contains(low, "401")
}

// IsForbidden returns true if the error indicates a 403 Forbidden condition.
func IsForbidden(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == http.StatusForbidden
	}
	low := strings.ToLower(err.Error())
	return strings.Contains(low, "forbidden") || strings.Contains(low, "403")
}

// IsBadRequest returns true if the error indicates a 400 Bad Request condition.
func IsBadRequest(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == http.StatusBadRequest
	}
	low := strings.ToLower(err.Error())
	return strings.Contains(low, "bad request") || strings.Contains(low, "400")
}

// IsBadGateway returns true if the error indicates a 502 Bad Gateway condition.
func IsBadGateway(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == http.StatusBadGateway
	}
	low := strings.ToLower(err.Error())
	return strings.Contains(low, "bad gateway") || strings.Contains(low, "502")
}
