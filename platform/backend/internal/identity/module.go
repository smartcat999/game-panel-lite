package identity

import (
	"context"
	"errors"
	"sync"

	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
)

var (
	ErrSessionNotFound = errors.New("session not found")
	ErrUserNotFound    = errors.New("user not found")
	ErrInvalidLocale   = errors.New("unsupported locale")
	ErrInvalidTheme    = errors.New("unsupported theme")
)

type User struct {
	ID          contract.UserID `json:"id"`
	DisplayName string          `json:"displayName"`
	Email       string          `json:"email"`
}

type Identity struct {
	ID       contract.IdentityID `json:"id"`
	UserID   contract.UserID     `json:"userId"`
	Provider string              `json:"provider"`
	Subject  string              `json:"subject"`
}

type Preferences struct {
	Locale   string `json:"locale"`
	Theme    string `json:"theme"`
	TimeZone string `json:"timeZone"`
}

type Session struct {
	UserID contract.UserID `json:"userId"`
}

type Seed struct {
	Users             []User
	Identities        []Identity
	Preferences       map[contract.UserID]Preferences
	Sessions          map[string]contract.UserID
	PlatformOperators []contract.UserID
	RegionOperators   map[contract.UserID][]contract.RegionID
}

type Module struct {
	mu                sync.RWMutex
	users             map[contract.UserID]User
	identities        map[contract.IdentityID]Identity
	preferences       map[contract.UserID]Preferences
	sessions          map[string]contract.UserID
	platformOperators map[contract.UserID]bool
	regionOperators   map[contract.UserID]map[contract.RegionID]bool
}

func New(seed Seed) *Module {
	module := &Module{
		users:             make(map[contract.UserID]User),
		identities:        make(map[contract.IdentityID]Identity),
		preferences:       make(map[contract.UserID]Preferences),
		sessions:          make(map[string]contract.UserID),
		platformOperators: make(map[contract.UserID]bool),
		regionOperators:   make(map[contract.UserID]map[contract.RegionID]bool),
	}
	for _, user := range seed.Users {
		module.users[user.ID] = user
	}
	for _, identity := range seed.Identities {
		module.identities[identity.ID] = identity
	}
	for userID, preferences := range seed.Preferences {
		module.preferences[userID] = preferences
	}
	for token, userID := range seed.Sessions {
		module.sessions[token] = userID
	}
	for _, userID := range seed.PlatformOperators {
		module.platformOperators[userID] = true
	}
	for userID, regionIDs := range seed.RegionOperators {
		module.regionOperators[userID] = make(map[contract.RegionID]bool, len(regionIDs))
		for _, regionID := range regionIDs {
			module.regionOperators[userID][regionID] = true
		}
	}
	return module
}

func (m *Module) RestoreSession(_ context.Context, token string) (Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	userID, ok := m.sessions[token]
	if !ok {
		return Session{}, ErrSessionNotFound
	}
	return Session{UserID: userID}, nil
}

func (m *Module) UserPreferences(_ context.Context, userID contract.UserID) (Preferences, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	preferences, ok := m.preferences[userID]
	if !ok {
		return Preferences{}, ErrUserNotFound
	}
	return preferences, nil
}

func (m *Module) UsersByID(_ context.Context, userIDs []contract.UserID) []User {
	m.mu.RLock()
	defer m.mu.RUnlock()
	users := make([]User, 0, len(userIDs))
	for _, userID := range userIDs {
		if user, ok := m.users[userID]; ok {
			users = append(users, user)
		}
	}
	return users
}

func (m *Module) UpdateUserPreferences(_ context.Context, userID contract.UserID, preferences Preferences) (Preferences, error) {
	if preferences.Locale != "en" && preferences.Locale != "zh-CN" {
		return Preferences{}, ErrInvalidLocale
	}
	if preferences.Theme != "light" && preferences.Theme != "dark" && preferences.Theme != "system" {
		return Preferences{}, ErrInvalidTheme
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.users[userID]; !ok {
		return Preferences{}, ErrUserNotFound
	}
	m.preferences[userID] = preferences
	return preferences, nil
}

func (m *Module) IsPlatformOperator(_ context.Context, userID contract.UserID) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.platformOperators[userID]
}

func (m *Module) IsRegionOperator(_ context.Context, userID contract.UserID, regionID contract.RegionID) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.platformOperators[userID] && m.regionOperators[userID][regionID]
}
