package authentication

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

type GitHubClient struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	HTTPClient   *http.Client
	OAuthBaseURL string
	APIBaseURL   string
}

func NewGitHubClient(clientID, clientSecret, redirectURL string) *GitHubClient {
	return &GitHubClient{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		HTTPClient:   &http.Client{Timeout: 10 * time.Second},
		OAuthBaseURL: "https://github.com",
		APIBaseURL:   "https://api.github.com",
	}
}

func (c *GitHubClient) AuthorizationURL(state string) string {
	query := url.Values{
		"client_id":    {c.ClientID},
		"redirect_uri": {c.RedirectURL},
		"scope":        {"read:user user:email"},
		"state":        {state},
	}
	return c.OAuthBaseURL + "/login/oauth/authorize?" + query.Encode()
}

func (c *GitHubClient) Exchange(ctx context.Context, code string) (OAuthProfile, error) {
	payload, err := json.Marshal(map[string]string{
		"client_id":     c.ClientID,
		"client_secret": c.ClientSecret,
		"code":          code,
		"redirect_uri":  c.RedirectURL,
	})
	if err != nil {
		return OAuthProfile{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.OAuthBaseURL+"/login/oauth/access_token", bytes.NewReader(payload))
	if err != nil {
		return OAuthProfile{}, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	var tokenResponse struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := c.getJSON(request, &tokenResponse); err != nil || tokenResponse.AccessToken == "" || tokenResponse.Error != "" {
		return OAuthProfile{}, ErrInvalidCredentials
	}

	var account struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := c.apiGET(ctx, tokenResponse.AccessToken, "/user", &account); err != nil || account.ID == 0 || account.Login == "" {
		return OAuthProfile{}, ErrInvalidCredentials
	}
	if account.Email == "" {
		var emails []struct {
			Email    string `json:"email"`
			Primary  bool   `json:"primary"`
			Verified bool   `json:"verified"`
		}
		if err := c.apiGET(ctx, tokenResponse.AccessToken, "/user/emails", &emails); err != nil {
			return OAuthProfile{}, err
		}
		for _, candidate := range emails {
			if candidate.Primary && candidate.Verified {
				account.Email = candidate.Email
				break
			}
		}
	}
	if account.Email == "" {
		return OAuthProfile{}, fmt.Errorf("github account has no verified primary email")
	}
	displayName := account.Name
	if displayName == "" {
		displayName = account.Login
	}
	return OAuthProfile{Provider: "github", Subject: fmt.Sprintf("%d", account.ID), Username: account.Login, Email: account.Email, DisplayName: displayName}, nil
}

func (c *GitHubClient) apiGET(ctx context.Context, token, path string, target any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.APIBaseURL+path, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	return c.getJSON(request, target)
}

func (c *GitHubClient) getJSON(request *http.Request, target any) error {
	response, err := c.HTTPClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("github returned status %d", response.StatusCode)
	}
	return json.NewDecoder(response.Body).Decode(target)
}
