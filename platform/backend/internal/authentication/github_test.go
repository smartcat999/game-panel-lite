package authentication

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestGitHubClientUsesVerifiedPrimaryEmail(t *testing.T) {
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		var body string
		switch request.URL.Path {
		case "/login/oauth/access_token":
			body = `{"access_token":"secret-token"}`
		case "/user":
			if request.Header.Get("Authorization") != "Bearer secret-token" {
				t.Fatal("missing bearer token")
			}
			body = `{"id":42,"login":"octocat","name":"Octo Cat","email":""}`
		case "/user/emails":
			body = `[{"email":"unverified@example.test","primary":true,"verified":false},{"email":"octo@example.test","primary":true,"verified":true}]`
		default:
			return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader(`{}`)), Header: http.Header{}}, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{"Content-Type": {"application/json"}}}, nil
	})
	client := NewGitHubClient("client", "secret", "https://panel.test/callback")
	client.HTTPClient = &http.Client{Transport: transport}
	profile, err := client.Exchange(context.Background(), "valid-code")
	if err != nil {
		t.Fatal(err)
	}
	if profile.Provider != "github" || profile.Subject != "42" || profile.Username != "octocat" || profile.Email != "octo@example.test" {
		t.Fatalf("profile = %#v", profile)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}
