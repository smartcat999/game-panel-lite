package workshop

import (
	"context"
	"errors"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

const (
	MaxCollectionItems = 200
	MaxCollectionDepth = 3
)

var (
	ErrInvalidCollection   = errors.New("invalid Steam Workshop collection")
	ErrCollectionTooLarge  = errors.New("Steam Workshop collection contains too many items")
	ErrUnsupportedProvider = errors.New("Steam Workshop collections are not supported for this provider")
)

type Resolver interface {
	ResolveCollection(ctx context.Context, providerKey domain.ProviderKey, input string) (Collection, error)
}

type Collection struct {
	ID    string `json:"collectionId"`
	Items []Item `json:"items"`
}

type Item struct {
	WorkshopID     string   `json:"workshopId"`
	Title          string   `json:"title"`
	CreatorSteamID string   `json:"creatorSteamId,omitempty"`
	PreviewURL     string   `json:"previewUrl,omitempty"`
	Description    string   `json:"description,omitempty"`
	FileSize       int64    `json:"fileSize"`
	Subscriptions  int      `json:"subscriptions,omitempty"`
	Favorited      int      `json:"favorited,omitempty"`
	Views          int      `json:"views,omitempty"`
	TimeCreated    int64    `json:"timeCreated,omitempty"`
	TimeUpdated    int64    `json:"timeUpdated,omitempty"`
	Tags           []string `json:"tags,omitempty"`
}

func (item Item) UpdatedAt() time.Time {
	if item.TimeUpdated <= 0 {
		return time.Time{}
	}
	return time.Unix(item.TimeUpdated, 0)
}
