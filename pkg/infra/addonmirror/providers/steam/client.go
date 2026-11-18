package steam

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror/driver"
)

// flexInt64 unmarshals JSON numbers or quoted numeric strings emitted by Steam Web API protobuf gateways.
type flexInt64 int64

func (f *flexInt64) UnmarshalJSON(data []byte) error {
	trimmed := strings.Trim(strings.TrimSpace(string(data)), `"`)
	if trimmed == "" || trimmed == "null" {
		*f = 0
		return nil
	}
	n, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil {
		*f = 0
		return nil
	}
	*f = flexInt64(n)
	return nil
}

type publishedFile struct {
	PublishedFileID       string     `json:"publishedfileid"`
	Result                int        `json:"result"`
	CreatorAppID          int        `json:"creator_app_id"`
	ConsumerAppID         int        `json:"consumer_app_id"`
	Title                 string     `json:"title"`
	FileDescription       string     `json:"file_description"`
	Description           string     `json:"description"`
	ShortDescription      string     `json:"short_description"`
	ChangeDescription     string     `json:"change_description"`
	PreviewURL            string     `json:"preview_url"`
	FileURL               string     `json:"file_url"`
	Filename              string     `json:"filename"`
	FileSize              flexInt64  `json:"file_size"`
	Subscriptions         flexInt64  `json:"subscriptions"`
	LifetimeSubscriptions flexInt64  `json:"lifetime_subscriptions"`
	TimeUpdated           flexInt64  `json:"time_updated"`
	Revision              flexInt64  `json:"revision"`
	HContentFile          string     `json:"hcontent_file"`
	Tags                  []steamTag `json:"tags"`
	Previews              []preview  `json:"previews"`
	Children              []depRef   `json:"children"`
}

type steamTag struct {
	Tag         string `json:"tag"`
	DisplayName string `json:"display_name"`
}

type preview struct {
	PreviewType int    `json:"preview_type"`
	URL         string `json:"url"`
}

type depRef struct {
	PublishedFileID string `json:"publishedfileid"`
}

type queryEnvelope struct {
	Response struct {
		Total                flexInt64       `json:"total"`
		PublishedFileDetails []publishedFile `json:"publishedfiledetails"`
	} `json:"response"`
}

type collectionEnvelope struct {
	Response struct {
		CollectionDetails []struct {
			PublishedFileID string   `json:"publishedfileid"`
			Result          int      `json:"result"`
			Children        []depRef `json:"children"`
		} `json:"collectiondetails"`
	} `json:"response"`
}

func hydrateDependencyTitles(ctx context.Context, rt driver.Runtime, base string, appID int, item *driver.Item) {
	if item == nil || len(item.Dependencies) == 0 {
		return
	}
	depIDs := make([]string, 0, len(item.Dependencies))
	for _, d := range item.Dependencies {
		if d.Title == "" && driver.IsDigitsOnly(d.ID) {
			depIDs = append(depIDs, d.ID)
		}
	}
	if len(depIDs) == 0 {
		return
	}
	deps, err := fetchFilesByIDs(ctx, rt, base, appID, depIDs)
	if err != nil {
		return
	}
	titles := make(map[string]string, len(deps))
	for _, d := range deps {
		titles[d.ID] = d.Title
	}
	for i := range item.Dependencies {
		if t := titles[item.Dependencies[i].ID]; t != "" {
			item.Dependencies[i].Title = t
		}
	}
}

func fetchCollectionChildren(ctx context.Context, rt driver.Runtime, base, id string) []string {
	form := url.Values{}
	form.Set("collectioncount", "1")
	form.Set("publishedfileids[0]", id)
	var env collectionEnvelope
	if err := rt.DoPostFormJSON(ctx, base+"/ISteamRemoteStorage/GetCollectionDetails/v1/", form, &env); err != nil {
		return nil
	}
	if len(env.Response.CollectionDetails) == 0 || len(env.Response.CollectionDetails[0].Children) == 0 {
		return nil
	}
	var out []string
	for _, ch := range env.Response.CollectionDetails[0].Children {
		if driver.IsDigitsOnly(ch.PublishedFileID) && !driver.ContainsString(out, ch.PublishedFileID) {
			out = append(out, ch.PublishedFileID)
		}
	}
	return out
}

func fetchFilesByIDs(ctx context.Context, rt driver.Runtime, base string, appID int, ids []string) ([]driver.Item, error) {
	if key := rt.SteamAPIKey(); key != "" {
		if items, err := fetchFilesViaService(ctx, rt, base, key, appID, ids); err == nil && len(items) > 0 {
			return items, nil
		}
	}
	return fetchFilesViaRemoteStorage(ctx, rt, base, appID, ids)
}

func fetchFilesViaService(ctx context.Context, rt driver.Runtime, base, apiKey string, appID int, ids []string) ([]driver.Item, error) {
	q := url.Values{}
	q.Set("key", apiKey)
	q.Set("includevotes", "true")
	q.Set("includetags", "true")
	q.Set("includeadditionalpreviews", "true")
	q.Set("includechildren", "true")
	q.Set("includemetadata", "true")
	q.Set("language", "6")
	for i, id := range ids {
		q.Set(fmt.Sprintf("publishedfileids[%d]", i), id)
	}
	var env queryEnvelope
	if err := rt.DoGetJSON(ctx, base+"/IPublishedFileService/GetDetails/v1/?"+q.Encode(), &env); err != nil {
		return nil, err
	}
	return filterMappedFiles(env.Response.PublishedFileDetails, appID), nil
}

func fetchFilesViaRemoteStorage(ctx context.Context, rt driver.Runtime, base string, appID int, ids []string) ([]driver.Item, error) {
	form := url.Values{}
	form.Set("itemcount", strconv.Itoa(len(ids)))
	for i, id := range ids {
		form.Set(fmt.Sprintf("publishedfileids[%d]", i), id)
	}
	var env queryEnvelope
	if err := rt.DoPostFormJSON(ctx, base+"/ISteamRemoteStorage/GetPublishedFileDetails/v1/", form, &env); err != nil {
		return nil, err
	}
	return filterMappedFiles(env.Response.PublishedFileDetails, appID), nil
}

func filterMappedFiles(files []publishedFile, expectedAppID int) []driver.Item {
	out := make([]driver.Item, 0, len(files))
	for _, pf := range files {
		if pf.PublishedFileID == "" || (pf.Result != 0 && pf.Result != 1) || strings.TrimSpace(pf.Title) == "" {
			continue
		}
		if expectedAppID > 0 && pf.ConsumerAppID > 0 && pf.ConsumerAppID != expectedAppID {
			continue
		}
		out = append(out, mapFileToItem(pf, expectedAppID))
	}
	return out
}

func mapFileToItem(pf publishedFile, appID int) driver.Item {
	rawDesc := pf.FileDescription
	if rawDesc == "" {
		rawDesc = pf.Description
	}
	cleanedDesc := CleanBBCode(rawDesc)
	summary := CleanBBCode(pf.ShortDescription)
	if summary != "" {
		summary = SummarizeText(summary, 110)
	} else {
		summary = SummarizeText(cleanedDesc, 110)
	}
	subs := int64(pf.Subscriptions)
	if subs <= 0 && pf.LifetimeSubscriptions > 0 {
		subs = int64(pf.LifetimeSubscriptions)
	}
	item := driver.Item{
		ID:            pf.PublishedFileID,
		Title:         strings.TrimSpace(pf.Title),
		Summary:       summary,
		Description:   FormatBBCodeMarkdown(rawDesc),
		Changelog:     FormatBBCodeMarkdown(strings.TrimSpace(pf.ChangeDescription)),
		IconURL:       strings.TrimSpace(pf.PreviewURL),
		SourceURL:     "https://steamcommunity.com/sharedfiles/filedetails/?id=" + pf.PublishedFileID,
		UpstreamURL:   "https://steamcommunity.com/sharedfiles/filedetails/changelog/" + pf.PublishedFileID,
		FileURL:       strings.TrimSpace(pf.FileURL),
		FileName:      strings.TrimSpace(pf.Filename),
		LatestVersion: "1",
		Revision:      int64(pf.Revision),
		Downloads:     subs,
		SizeBytes:     int64(pf.FileSize),
		Tags:          extractTags(pf.Tags),
		GalleryURLs:   extractGallery(pf.PreviewURL, pf.Previews),
		Dependencies:  extractDeps(pf.Children),
	}
	if pf.TimeUpdated > 0 {
		item.LatestVersion = strconv.FormatInt(int64(pf.TimeUpdated), 10)
		item.UpdatedAt = time.Unix(int64(pf.TimeUpdated), 0).UTC().Format(time.RFC3339)
	}
	if appID == driver.SteamAppProjectZomboid || pf.ConsumerAppID == driver.SteamAppProjectZomboid {
		item.ModIDs, item.MapFolders = ExtractProjectZomboidMeta(rawDesc)
	}
	return item
}

func extractTags(tags []steamTag) []string {
	var out []string
	for _, t := range tags {
		label := driver.FirstNonEmpty(t.DisplayName, t.Tag)
		if label != "" && !driver.ContainsString(out, label) {
			out = append(out, label)
		}
	}
	return out
}

func extractGallery(mainPreview string, previews []preview) []string {
	var out []string
	if m := strings.TrimSpace(mainPreview); m != "" {
		out = append(out, m)
	}
	for _, p := range previews {
		if p.PreviewType == 0 && strings.TrimSpace(p.URL) != "" && !driver.ContainsString(out, strings.TrimSpace(p.URL)) {
			out = append(out, strings.TrimSpace(p.URL))
		}
	}
	return out
}

func extractDeps(children []depRef) []driver.Dependency {
	var out []driver.Dependency
	for _, ch := range children {
		id := strings.TrimSpace(ch.PublishedFileID)
		if driver.IsDigitsOnly(id) {
			out = append(out, driver.Dependency{ID: id})
		}
	}
	return out
}
