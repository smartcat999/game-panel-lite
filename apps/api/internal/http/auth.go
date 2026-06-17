package http

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

const (
	sessionCookieName = "gamepanel_session"
	sessionTTL        = 7 * 24 * time.Hour
	passwordMinLength = 8
	pbkdf2Iterations  = 120000
	pbkdf2KeyLength   = 32
)

var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9_]{3,32}$`)

type authContextKey string

const authAccountContextKey authContextKey = "account"

type authAccountResponse struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

type authBootstrapResponse struct {
	Initialized bool                 `json:"initialized"`
	Account     *authAccountResponse `json:"account,omitempty"`
}

func (h *Handler) authBootstrap(w http.ResponseWriter, r *http.Request) {
	initialized, err := h.store.HasAdminAccount(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	response := authBootstrapResponse{Initialized: initialized}
	if account, ok := accountFromContext(r.Context()); ok {
		response.Account = &authAccountResponse{ID: account.ID, Username: account.Username}
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) setupAdmin(w http.ResponseWriter, r *http.Request) {
	initialized, err := h.store.HasAdminAccount(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if initialized {
		writeError(w, http.StatusConflict, "admin account already exists")
		return
	}
	var payload struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	username, password, err := validateCredentials(payload.Username, payload.Password)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	passwordHash, err := hashPassword(password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	account := domain.AdminAccount{
		ID:           uuid.NewString(),
		Username:     username,
		PasswordHash: passwordHash,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := h.store.CreateAdminAccount(r.Context(), &account); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.createSessionCookie(w, r, account); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, authAccountResponse{ID: account.ID, Username: account.Username})
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	account, err := h.store.GetAdminAccountByUsername(r.Context(), strings.TrimSpace(payload.Username))
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}
	if !verifyPassword(account.PasswordHash, payload.Password) {
		writeError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}
	if err := h.createSessionCookie(w, r, account); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, authAccountResponse{ID: account.ID, Username: account.Username})
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		if session, err := h.store.GetSessionByTokenHash(r.Context(), hashSessionToken(cookie.Value)); err == nil {
			_ = h.store.DeleteSession(r.Context(), session.ID)
		}
	}
	clearSessionCookie(w, r)
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}

func (h *Handler) currentAccount(w http.ResponseWriter, r *http.Request) {
	account, ok := accountFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	writeJSON(w, http.StatusOK, authAccountResponse{ID: account.ID, Username: account.Username})
}

func (h *Handler) changePassword(w http.ResponseWriter, r *http.Request) {
	account, ok := accountFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	var payload struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	persisted, err := h.store.GetAdminAccount(r.Context(), account.ID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	if !verifyPassword(persisted.PasswordHash, payload.CurrentPassword) {
		writeError(w, http.StatusUnauthorized, "current password is incorrect")
		return
	}
	if err := validatePassword(payload.NewPassword); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	passwordHash, err := hashPassword(payload.NewPassword)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	persisted.PasswordHash = passwordHash
	persisted.UpdatedAt = time.Now()
	if err := h.store.SaveAdminAccount(r.Context(), &persisted); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, authAccountResponse{ID: persisted.ID, Username: persisted.Username})
}

func (h *Handler) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		initialized, err := h.store.HasAdminAccount(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !initialized {
			next.ServeHTTP(w, r)
			return
		}
		account, ok := h.accountFromRequest(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), authAccountContextKey, account)))
	})
}

func (h *Handler) optionalAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if account, ok := h.accountFromRequest(r); ok {
			r = r.WithContext(context.WithValue(r.Context(), authAccountContextKey, account))
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) accountFromRequest(r *http.Request) (domain.AdminAccount, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return domain.AdminAccount{}, false
	}
	session, err := h.store.GetSessionByTokenHash(r.Context(), hashSessionToken(cookie.Value))
	if err != nil || time.Now().After(session.ExpiresAt) {
		if err == nil {
			_ = h.store.DeleteSession(r.Context(), session.ID)
		}
		return domain.AdminAccount{}, false
	}
	account, err := h.store.GetAdminAccount(r.Context(), session.AccountID)
	if err != nil {
		return domain.AdminAccount{}, false
	}
	return account, true
}

func accountFromContext(ctx context.Context) (domain.AdminAccount, bool) {
	account, ok := ctx.Value(authAccountContextKey).(domain.AdminAccount)
	return account, ok
}

func (h *Handler) createSessionCookie(w http.ResponseWriter, r *http.Request, account domain.AdminAccount) error {
	token, err := randomToken(32)
	if err != nil {
		return err
	}
	now := time.Now()
	session := domain.Session{
		ID:        uuid.NewString(),
		AccountID: account.ID,
		TokenHash: hashSessionToken(token),
		ExpiresAt: now.Add(sessionTTL),
		CreatedAt: now,
	}
	if err := h.store.DeleteExpiredSessions(r.Context(), now); err != nil {
		return err
	}
	if err := h.store.CreateSession(r.Context(), &session); err != nil {
		return err
	}
	http.SetCookie(w, sessionCookie(r, token, session.ExpiresAt))
	return nil
}

func sessionCookie(r *http.Request, token string, expires time.Time) *http.Cookie {
	return &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		MaxAge:   int(time.Until(expires).Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil,
	}
}

func clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	cookie := sessionCookie(r, "", time.Unix(0, 0))
	cookie.MaxAge = -1
	http.SetCookie(w, cookie)
}

func validateCredentials(username string, password string) (string, string, error) {
	username = strings.TrimSpace(username)
	if !usernamePattern.MatchString(username) {
		return "", "", errors.New("username must be 3 to 32 letters, digits, or underscores")
	}
	if err := validatePassword(password); err != nil {
		return "", "", err
	}
	return username, password, nil
}

func validatePassword(password string) error {
	if len(password) < passwordMinLength {
		return fmt.Errorf("password must be at least %d characters", passwordMinLength)
	}
	return nil
}

func hashPassword(password string) (string, error) {
	salt, err := randomBytes(16)
	if err != nil {
		return "", err
	}
	key := pbkdf2SHA256([]byte(password), salt, pbkdf2Iterations, pbkdf2KeyLength)
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s", pbkdf2Iterations, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

func verifyPassword(encoded string, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return false
	}
	iterations, err := strconv.Atoi(parts[1])
	if err != nil || iterations <= 0 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil {
		return false
	}
	actual := pbkdf2SHA256([]byte(password), salt, iterations, len(expected))
	return subtle.ConstantTimeCompare(actual, expected) == 1
}

func hashSessionToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func randomToken(size int) (string, error) {
	bytes, err := randomBytes(size)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func randomBytes(size int) ([]byte, error) {
	bytes := make([]byte, size)
	if _, err := rand.Read(bytes); err != nil {
		return nil, err
	}
	return bytes, nil
}

func pbkdf2SHA256(password []byte, salt []byte, iterations int, keyLen int) []byte {
	hashLen := sha256.Size
	numBlocks := (keyLen + hashLen - 1) / hashLen
	output := make([]byte, 0, numBlocks*hashLen)
	for block := 1; block <= numBlocks; block++ {
		mac := hmac.New(sha256.New, password)
		mac.Write(salt)
		mac.Write([]byte{byte(block >> 24), byte(block >> 16), byte(block >> 8), byte(block)})
		u := mac.Sum(nil)
		t := append([]byte(nil), u...)
		for i := 1; i < iterations; i++ {
			mac = hmac.New(sha256.New, password)
			mac.Write(u)
			u = mac.Sum(nil)
			for j := range t {
				t[j] ^= u[j]
			}
		}
		output = append(output, t...)
	}
	return output[:keyLen]
}
