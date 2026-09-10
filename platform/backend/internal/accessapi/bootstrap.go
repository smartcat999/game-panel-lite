package accessapi

import (
	"database/sql"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/smartcat999/game-panel-lite/platform/backend/internal/authentication"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/authorization"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/httpfilter"
)

type Config struct {
	GitHubClientID     string
	GitHubClientSecret string
	GitHubRedirectURL  string
	TOTPKeyBase64      string
	SecureCookies      bool
}

func PostgresRoutes(database *sql.DB, config Config) (http.Handler, error) {
	services, err := PostgresServices(database, config)
	if err != nil {
		return nil, err
	}
	return Routes(services), nil
}

func PostgresServices(database *sql.DB, config Config) (Services, error) {
	key, err := base64.StdEncoding.DecodeString(config.TOTPKeyBase64)
	if err != nil || len(key) != 32 {
		return Services{}, fmt.Errorf("TOTP encryption key must be 32 bytes encoded as base64")
	}
	if config.GitHubClientID == "" || config.GitHubClientSecret == "" || config.GitHubRedirectURL == "" {
		return Services{}, fmt.Errorf("GitHub OAuth configuration is incomplete")
	}
	secretCipher, err := authentication.NewSecretCipher(key)
	if err != nil {
		return Services{}, err
	}
	store := authentication.NewPostgresStore(database)
	sessions := authentication.NewSessionService(store, authentication.SessionPolicy{
		AbsoluteLifetime: 24 * time.Hour,
		IdleTimeout:      time.Hour,
		ReauthWindow:     10 * time.Minute,
		SecureCookies:    config.SecureCookies,
	})
	bindings := authorization.NewPostgresStore(database)
	return Services{
		OAuth:       authentication.NewOAuthService(store, authentication.NewGitHubClient(config.GitHubClientID, config.GitHubClientSecret, config.GitHubRedirectURL), sessions, 10*time.Minute),
		Passwords:   authentication.NewPasswordService(store, sessions),
		Sessions:    sessions,
		TOTP:        authentication.NewTOTPService(store, secretCipher),
		Invitations: authentication.NewInvitationService(store, bindings),
		Authorizer:  authorization.NewEngine(bindings),
		Resolver:    httpfilter.NewPostgresScopeResolver(database),
	}, nil
}

func WithFallback(access, fallback http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if isAccessRoute(request.Method, request.URL.Path) {
			access.ServeHTTP(response, request)
			return
		}
		fallback.ServeHTTP(response, request)
	})
}

func isAccessRoute(method, path string) bool {
	if strings.HasPrefix(path, "/v1/auth/") || path == "/v1/session" || strings.HasPrefix(path, "/v1/invitations/") {
		return true
	}
	if method == http.MethodPost && path == "/v1/platform/users" {
		return true
	}
	return method == http.MethodPost && strings.HasPrefix(path, "/v1/workspaces/") && strings.HasSuffix(path, "/invitations")
}
