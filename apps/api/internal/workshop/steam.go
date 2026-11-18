package workshop

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

const (
	defaultSteamAPIBase       = "https://api.steampowered.com"
	defaultSteamCommunityBase = "https://steamcommunity.com"
	maxSteamResponse          = 4 << 20
	steamBatchSize            = 50
)

type SteamResolver struct {
	client        *http.Client
	apiBase       string
	communityBase string
}

var workshopItemTitlePattern = regexp.MustCompile(`(?is)<div\s+class=["']workshopItemTitle["']\s*>\s*([^<]+?)\s*</div>`)

func NewSteamResolver() *SteamResolver {
	resolver := NewSteamResolverWithClient(&http.Client{Timeout: 10 * time.Second}, defaultSteamAPIBase)
	resolver.communityBase = defaultSteamCommunityBase
	return resolver
}

func NewSteamResolverWithClient(client *http.Client, apiBase string) *SteamResolver {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &SteamResolver{
		client:        client,
		apiBase:       strings.TrimRight(apiBase, "/"),
		communityBase: strings.TrimRight(apiBase, "/"),
	}
}

func (r *SteamResolver) ResolveCollection(ctx context.Context, providerKey domain.ProviderKey, input string) (Collection, error) {
	appID, ok := workshopAppID(providerKey)
	if !ok {
		return Collection{}, ErrUnsupportedProvider
	}
	collectionID, err := ParseCollectionID(input)
	if err != nil {
		return Collection{}, err
	}
	itemIDs, err := r.expandCollection(ctx, collectionID, MaxCollectionDepth)
	if err != nil {
		return Collection{}, err
	}
	if len(itemIDs) == 0 {
		return Collection{}, fmt.Errorf("%w: collection is empty or unavailable", ErrInvalidCollection)
	}
	if len(itemIDs) > MaxCollectionItems {
		return Collection{}, ErrCollectionTooLarge
	}
	title, items, err := r.publishedFileDetails(ctx, collectionID, itemIDs, appID)
	if err != nil {
		return Collection{}, err
	}
	if len(items) == 0 {
		return Collection{}, fmt.Errorf("%w: collection has no compatible items", ErrInvalidCollection)
	}
	if title == "" {
		title, _ = r.collectionPageTitle(ctx, collectionID)
	}
	return Collection{ID: collectionID, Title: title, Items: items}, nil
}

func (r *SteamResolver) ResolveItems(ctx context.Context, providerKey domain.ProviderKey, workshopIDs []string) ([]Item, error) {
	appID, ok := workshopAppID(providerKey)
	if !ok {
		return nil, ErrUnsupportedProvider
	}
	unique := make([]string, 0, len(workshopIDs))
	seen := make(map[string]struct{}, len(workshopIDs))
	for _, id := range workshopIDs {
		id = strings.TrimSpace(id)
		if !isDigits(id) {
			return nil, fmt.Errorf("%w: invalid item ID %q", ErrInvalidCollection, id)
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	if len(unique) == 0 {
		return nil, fmt.Errorf("%w: enter at least one item ID", ErrInvalidCollection)
	}
	if len(unique) > MaxCollectionItems {
		return nil, ErrCollectionTooLarge
	}
	_, items, err := r.publishedFileDetails(ctx, "", unique, appID)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("%w: items are unavailable or incompatible with the selected game", ErrInvalidCollection)
	}
	return items, nil
}

func (r *SteamResolver) collectionPageTitle(ctx context.Context, collectionID string) (string, error) {
	requestURL := r.communityBase + "/sharedfiles/filedetails/?id=" + url.QueryEscape(collectionID)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("User-Agent", "GamePanel-Lite/1.0")
	response, err := r.client.Do(request)
	if err != nil {
		return "", fmt.Errorf("Steam Workshop page request failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Steam Workshop page returned HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxSteamResponse))
	if err != nil {
		return "", err
	}
	match := workshopItemTitlePattern.FindSubmatch(body)
	if len(match) != 2 {
		return "", fmt.Errorf("Steam Workshop collection title was not found")
	}
	title := truncateRunes(strings.TrimSpace(html.UnescapeString(string(match[1]))), 300)
	if title == "" {
		return "", fmt.Errorf("Steam Workshop collection title was empty")
	}
	return title, nil
}

func ParseCollectionID(input string) (string, error) {
	input = strings.TrimSpace(input)
	if isDigits(input) {
		return input, nil
	}
	parsed, err := url.Parse(input)
	if err != nil || parsed.Scheme != "https" {
		return "", fmt.Errorf("%w: enter a numeric ID or an https://steamcommunity.com collection URL", ErrInvalidCollection)
	}
	host := strings.ToLower(parsed.Hostname())
	if host != "steamcommunity.com" && host != "www.steamcommunity.com" {
		return "", fmt.Errorf("%w: only steamcommunity.com URLs are accepted", ErrInvalidCollection)
	}
	path := strings.TrimSuffix(parsed.EscapedPath(), "/")
	if path != "/sharedfiles/filedetails" && path != "/workshop/filedetails" {
		return "", fmt.Errorf("%w: unsupported Steam Workshop URL", ErrInvalidCollection)
	}
	id := strings.TrimSpace(parsed.Query().Get("id"))
	if !isDigits(id) {
		return "", fmt.Errorf("%w: collection ID is missing", ErrInvalidCollection)
	}
	return id, nil
}

func (r *SteamResolver) expandCollection(ctx context.Context, rootID string, remainingDepth int) ([]string, error) {
	type queuedCollection struct {
		id    string
		depth int
	}
	queue := []queuedCollection{{id: rootID, depth: remainingDepth}}
	seenCollections := map[string]struct{}{}
	items := map[string]struct{}{}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if _, seen := seenCollections[current.id]; seen {
			continue
		}
		seenCollections[current.id] = struct{}{}
		details, err := r.collectionDetails(ctx, []string{current.id})
		if err != nil {
			return nil, err
		}
		children, ok := details[current.id]
		if !ok {
			return nil, fmt.Errorf("%w: collection %s is private, unavailable, or not a collection", ErrInvalidCollection, current.id)
		}
		for _, child := range children {
			switch child.FileType {
			case 2:
				if current.depth <= 0 {
					return nil, fmt.Errorf("%w: nested collection depth exceeds %d", ErrInvalidCollection, MaxCollectionDepth)
				}
				queue = append(queue, queuedCollection{id: child.PublishedFileID, depth: current.depth - 1})
			default:
				items[child.PublishedFileID] = struct{}{}
				if len(items) > MaxCollectionItems {
					return nil, ErrCollectionTooLarge
				}
			}
		}
	}
	ids := make([]string, 0, len(items))
	for id := range items {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids, nil
}

type collectionChild struct {
	PublishedFileID string `json:"publishedfileid"`
	FileType        int    `json:"filetype"`
}

func (r *SteamResolver) collectionDetails(ctx context.Context, ids []string) (map[string][]collectionChild, error) {
	form := url.Values{"collectioncount": {strconv.Itoa(len(ids))}}
	for i, id := range ids {
		form.Set(fmt.Sprintf("publishedfileids[%d]", i), id)
	}
	var payload struct {
		Response struct {
			CollectionDetails []struct {
				PublishedFileID string            `json:"publishedfileid"`
				Result          int               `json:"result"`
				Children        []collectionChild `json:"children"`
			} `json:"collectiondetails"`
		} `json:"response"`
	}
	if err := r.postForm(ctx, "/ISteamRemoteStorage/GetCollectionDetails/v1/", form, &payload); err != nil {
		return nil, err
	}
	result := make(map[string][]collectionChild, len(payload.Response.CollectionDetails))
	for _, detail := range payload.Response.CollectionDetails {
		if detail.Result == 1 && isDigits(detail.PublishedFileID) {
			result[detail.PublishedFileID] = detail.Children
		}
	}
	return result, nil
}

func (r *SteamResolver) publishedFileDetails(ctx context.Context, collectionID string, ids []string, expectedAppID int) (string, []Item, error) {
	allIDs := append([]string(nil), ids...)
	if collectionID != "" {
		allIDs = append([]string{collectionID}, allIDs...)
	}
	collectionTitle := ""
	items := make([]Item, 0, len(ids))
	for start := 0; start < len(allIDs); start += steamBatchSize {
		end := start + steamBatchSize
		if end > len(allIDs) {
			end = len(allIDs)
		}
		title, batch, err := r.publishedFileDetailsBatch(ctx, allIDs[start:end], expectedAppID, collectionID)
		if err != nil {
			return "", nil, err
		}
		if title != "" {
			collectionTitle = title
		}
		items = append(items, batch...)
	}
	sort.Slice(items, func(i, j int) bool {
		return strings.ToLower(items[i].Title) < strings.ToLower(items[j].Title)
	})
	return collectionTitle, items, nil
}

func (r *SteamResolver) publishedFileDetailsBatch(ctx context.Context, ids []string, expectedAppID int, collectionID string) (string, []Item, error) {
	form := url.Values{"itemcount": {strconv.Itoa(len(ids))}}
	for i, id := range ids {
		form.Set(fmt.Sprintf("publishedfileids[%d]", i), id)
	}
	var payload struct {
		Response struct {
			PublishedFileDetails []struct {
				PublishedFileID string `json:"publishedfileid"`
				Result          int    `json:"result"`
				Creator         string `json:"creator"`
				ConsumerAppID   int    `json:"consumer_app_id"`
				FileSize        int64  `json:"file_size,string"`
				PreviewURL      string `json:"preview_url"`
				Title           string `json:"title"`
				Description     string `json:"description"`
				TimeCreated     int64  `json:"time_created"`
				TimeUpdated     int64  `json:"time_updated"`
				FileType        int    `json:"file_type"`
				Subscriptions   int    `json:"subscriptions"`
				Favorited       int    `json:"favorited"`
				Views           int    `json:"views"`
				Tags            []struct {
					Tag string `json:"tag"`
				} `json:"tags"`
			} `json:"publishedfiledetails"`
		} `json:"response"`
	}
	if err := r.postForm(ctx, "/ISteamRemoteStorage/GetPublishedFileDetails/v1/", form, &payload); err != nil {
		return "", nil, err
	}
	collectionTitle := ""
	items := make([]Item, 0, len(payload.Response.PublishedFileDetails))
	for _, detail := range payload.Response.PublishedFileDetails {
		if detail.PublishedFileID == collectionID {
			if detail.Result == 1 && detail.ConsumerAppID == expectedAppID && detail.FileType == 2 {
				collectionTitle = truncateRunes(strings.TrimSpace(detail.Title), 300)
			}
			continue
		}
		if detail.Result != 1 || detail.ConsumerAppID != expectedAppID || detail.FileType == 2 || !isDigits(detail.PublishedFileID) {
			continue
		}
		tags := make([]string, 0, len(detail.Tags))
		for _, tag := range detail.Tags {
			if len(tags) >= 64 {
				break
			}
			if value := truncateRunes(strings.TrimSpace(tag.Tag), 100); value != "" {
				tags = append(tags, value)
			}
		}
		items = append(items, Item{
			WorkshopID:     detail.PublishedFileID,
			Title:          truncateRunes(strings.TrimSpace(detail.Title), 300),
			CreatorSteamID: detail.Creator,
			PreviewURL:     detail.PreviewURL,
			Description:    truncateRunes(strings.TrimSpace(detail.Description), 8000),
			FileSize:       detail.FileSize,
			Subscriptions:  detail.Subscriptions,
			Favorited:      detail.Favorited,
			Views:          detail.Views,
			TimeCreated:    detail.TimeCreated,
			TimeUpdated:    detail.TimeUpdated,
			Tags:           tags,
		})
	}
	return collectionTitle, items, nil
}

func (r *SteamResolver) postForm(ctx context.Context, path string, form url.Values, target any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, r.apiBase+path, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("User-Agent", "GamePanel-Lite/1.0")
	response, err := r.client.Do(request)
	if err != nil {
		return fmt.Errorf("Steam Workshop request failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1024))
		return fmt.Errorf("Steam Workshop returned HTTP %d", response.StatusCode)
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxSteamResponse))
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode Steam Workshop response: %w", err)
	}
	return nil
}

func workshopAppID(providerKey domain.ProviderKey) (int, bool) {
	switch providerKey {
	case domain.ProviderTerrariaTModLoader:
		return 1281930, true
	case domain.ProviderDST:
		return 322330, true
	default:
		return 0, false
	}
}

func isDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
