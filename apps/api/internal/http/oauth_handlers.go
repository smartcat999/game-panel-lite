package http

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

const (
	oauthStateCookieName = "gamepanel_oauth_state"
	oauthStateTTL        = 10 * time.Minute
)

type oauthProvidersResponse struct {
	GitHub bool `json:"github"`
	Google bool `json:"google"`
}

// getOAuthProviders returns which OAuth providers are configured and available: GET /api/auth/oauth/providers
func (h *Handler) getOAuthProviders(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, oauthProvidersResponse{
		GitHub: h.cfg.GithubClientID != "" && h.cfg.GithubClientSecret != "",
		Google: h.cfg.GoogleClientID != "" && h.cfg.GoogleClientSecret != "",
	})
}

// oauthAuthorize redirects user to the provider's OAuth authorize page: GET /api/auth/oauth/{provider}/authorize
func (h *Handler) oauthAuthorize(w http.ResponseWriter, r *http.Request) {
	provider := strings.ToLower(chi.URLParam(r, "provider"))

	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate oauth state")
		return
	}
	state := hex.EncodeToString(stateBytes)

	// Set state cookie
	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookieName,
		Value:    state,
		Path:     "/",
		Expires:  time.Now().Add(oauthStateTTL),
		MaxAge:   int(oauthStateTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil,
	})

	redirectURI := h.oauthRedirectURI(r, provider)

	switch provider {
	case "github":
		if h.cfg.GithubClientID == "" {
			writeError(w, http.StatusBadRequest, "GitHub OAuth is not configured")
			return
		}
		authURL := fmt.Sprintf(
			"https://github.com/login/oauth/authorize?client_id=%s&redirect_uri=%s&scope=%s&state=%s",
			url.QueryEscape(h.cfg.GithubClientID),
			url.QueryEscape(redirectURI),
			url.QueryEscape("read:user user:email"),
			url.QueryEscape(state),
		)
		http.Redirect(w, r, authURL, http.StatusTemporaryRedirect)

	case "google":
		if h.cfg.GoogleClientID == "" {
			writeError(w, http.StatusBadRequest, "Google OAuth is not configured")
			return
		}
		authURL := fmt.Sprintf(
			"https://accounts.google.com/o/oauth2/v2/auth?client_id=%s&redirect_uri=%s&response_type=code&scope=%s&state=%s&prompt=select_account",
			url.QueryEscape(h.cfg.GoogleClientID),
			url.QueryEscape(redirectURI),
			url.QueryEscape("openid email profile"),
			url.QueryEscape(state),
		)
		http.Redirect(w, r, authURL, http.StatusTemporaryRedirect)

	default:
		writeError(w, http.StatusBadRequest, "unsupported OAuth provider")
	}
}

// oauthCallback handles provider callback, token exchange, and login/registration: GET /api/auth/oauth/{provider}/callback
func (h *Handler) oauthCallback(w http.ResponseWriter, r *http.Request) {
	provider := strings.ToLower(chi.URLParam(r, "provider"))

	// Validate state
	stateCookie, err := r.Cookie(oauthStateCookieName)
	if err != nil || stateCookie.Value == "" || stateCookie.Value != r.URL.Query().Get("state") {
		http.Redirect(w, r, "/login?error=oauth_state_invalid", http.StatusTemporaryRedirect)
		return
	}

	// Clear state cookie
	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
	})

	code := r.URL.Query().Get("code")
	if code == "" {
		errMsg := r.URL.Query().Get("error_description")
		if errMsg == "" {
			errMsg = "authorization_denied"
		}
		http.Redirect(w, r, "/login?error="+url.QueryEscape(errMsg), http.StatusTemporaryRedirect)
		return
	}

	redirectURI := h.oauthRedirectURI(r, provider)
	var identity *domain.OAuthIdentity

	switch provider {
	case "github":
		identity, err = h.exchangeGitHubOAuth(r.Context(), code, redirectURI)
	case "google":
		identity, err = h.exchangeGoogleOAuth(r.Context(), code, redirectURI)
	default:
		http.Redirect(w, r, "/login?error=unsupported_provider", http.StatusTemporaryRedirect)
		return
	}

	if err != nil {
		http.Redirect(w, r, "/login?error="+url.QueryEscape(err.Error()), http.StatusTemporaryRedirect)
		return
	}

	// Find or create account
	var account domain.AdminAccount
	existingIdentity, err := h.store.FindOAuthIdentity(r.Context(), identity.Provider, identity.ProviderUserID)
	if err == nil {
		// Existing linked identity -> fetch user
		acc, err := h.store.GetAdminAccount(r.Context(), existingIdentity.UserID)
		if err != nil {
			http.Redirect(w, r, "/login?error=account_not_found", http.StatusTemporaryRedirect)
			return
		}
		account = acc
	} else if errors.Is(err, store.ErrNotFound) {
		// New OAuth user -> create account + personal org with 100 starter credits
		preferredUsername := identity.Name
		if preferredUsername == "" {
			if atIdx := strings.Index(identity.Email, "@"); atIdx > 0 {
				preferredUsername = identity.Email[:atIdx]
			} else {
				preferredUsername = fmt.Sprintf("%s_%s", identity.Provider, identity.ProviderUserID)
			}
		}

		acc, _, err := h.store.CreateUserWithOAuth(r.Context(), identity, preferredUsername)
		if err != nil {
			http.Redirect(w, r, "/login?error="+url.QueryEscape("failed to create oauth account: "+err.Error()), http.StatusTemporaryRedirect)
			return
		}
		account = *acc
	} else {
		http.Redirect(w, r, "/login?error="+url.QueryEscape("database error"), http.StatusTemporaryRedirect)
		return
	}

	// Create session cookie
	if err := h.createSessionCookie(w, r, account); err != nil {
		http.Redirect(w, r, "/login?error=failed_to_create_session", http.StatusTemporaryRedirect)
		return
	}

	h.recordActivity(r.Context(), account.ID, "auth.oauth_login", fmt.Sprintf("User logged in via %s", provider), map[string]any{
		"provider": provider,
		"email":    identity.Email,
	})

	http.Redirect(w, r, "/dashboard", http.StatusTemporaryRedirect)
}

func (h *Handler) oauthRedirectURI(r *http.Request, provider string) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	host := r.Host
	if h.cfg.PublicHost != "" {
		host = h.cfg.PublicHost
	}
	return fmt.Sprintf("%s://%s/api/auth/oauth/%s/callback", scheme, host, provider)
}

func (h *Handler) exchangeGitHubOAuth(ctx context.Context, code string, redirectURI string) (*domain.OAuthIdentity, error) {
	tokenURL := "https://github.com/login/oauth/access_token"
	data := url.Values{
		"client_id":     {h.cfg.GithubClientID},
		"client_secret": {h.cfg.GithubClientSecret},
		"code":          {code},
		"redirect_uri":  {redirectURI},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github token exchange failed: %w", err)
	}
	defer resp.Body.Close()

	var tokenRes struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenRes); err != nil {
		return nil, errors.New("invalid token response from github")
	}
	if tokenRes.Error != "" {
		return nil, fmt.Errorf("github error: %s", tokenRes.ErrorDesc)
	}

	// Fetch GitHub user profile
	userReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user", nil)
	userReq.Header.Set("Authorization", "Bearer "+tokenRes.AccessToken)
	userReq.Header.Set("Accept", "application/json")
	userResp, err := client.Do(userReq)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch github profile: %w", err)
	}
	defer userResp.Body.Close()

	var ghUser struct {
		ID        int64  `json:"id"`
		Login     string `json:"login"`
		Name      string `json:"name"`
		Email     string `json:"email"`
		AvatarURL string `json:"avatar_url"`
	}
	if err := json.NewDecoder(userResp.Body).Decode(&ghUser); err != nil {
		return nil, errors.New("failed to decode github user")
	}

	userName := ghUser.Name
	if userName == "" {
		userName = ghUser.Login
	}

	return &domain.OAuthIdentity{
		Provider:       "github",
		ProviderUserID: fmt.Sprintf("%d", ghUser.ID),
		Email:          ghUser.Email,
		Name:           userName,
		AvatarURL:      ghUser.AvatarURL,
	}, nil
}

func (h *Handler) exchangeGoogleOAuth(ctx context.Context, code string, redirectURI string) (*domain.OAuthIdentity, error) {
	tokenURL := "https://oauth2.googleapis.com/token"
	data := url.Values{
		"code":          {code},
		"client_id":     {h.cfg.GoogleClientID},
		"client_secret": {h.cfg.GoogleClientSecret},
		"redirect_uri":  {redirectURI},
		"grant_type":    {"authorization_code"},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("google token exchange failed: %w", err)
	}
	defer resp.Body.Close()

	var tokenRes struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &tokenRes); err != nil {
		return nil, errors.New("invalid token response from google")
	}
	if tokenRes.Error != "" {
		return nil, fmt.Errorf("google error: %s", tokenRes.ErrorDesc)
	}

	// Fetch Google user profile
	userReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.googleapis.com/oauth2/v2/userinfo", nil)
	userReq.Header.Set("Authorization", "Bearer "+tokenRes.AccessToken)
	userResp, err := client.Do(userReq)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch google profile: %w", err)
	}
	defer userResp.Body.Close()

	var gUser struct {
		ID      string `json:"id"`
		Email   string `json:"email"`
		Name    string `json:"name"`
		Picture string `json:"picture"`
	}
	if err := json.NewDecoder(userResp.Body).Decode(&gUser); err != nil {
		return nil, errors.New("failed to decode google user")
	}

	return &domain.OAuthIdentity{
		Provider:       "google",
		ProviderUserID: gUser.ID,
		Email:          gUser.Email,
		Name:           gUser.Name,
		AvatarURL:      gUser.Picture,
	}, nil
}
