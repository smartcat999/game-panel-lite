package modio

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror/driver"
)

const (
	maxAvailableVersions = 25
	unconfiguredMessage  = "未配置 mod.io API 密钥，请在管理后台配置 GP_ADDON_MIRROR_MODIO_API_KEY 或上游凭证后浏览 mod.io 官方模组。"
)

type modsResponse struct {
	Data        []modioMod `json:"data"`
	ResultTotal int        `json:"result_total"`
}

type modfilesResponse struct {
	Data []modioFile `json:"data"`
}

type dependenciesResponse struct {
	Data []modioDependency `json:"data"`
}

type modioMod struct {
	ID          int64      `json:"id"`
	NameID      string     `json:"name_id"`
	Name        string     `json:"name"`
	Summary     string     `json:"summary"`
	Description string     `json:"description_plaintext"`
	DescHTML    string     `json:"description"`
	ProfileURL  string     `json:"profile_url"`
	HomepageURL string     `json:"homepage_url"`
	DateUpdated int64      `json:"date_updated"`
	Logo        modioLogo  `json:"logo"`
	SubmittedBy modioUser  `json:"submitted_by"`
	Modfile     modioFile  `json:"modfile"`
	Stats       modioStats `json:"stats"`
	Tags        []modioTag `json:"tags"`
}

type modioLogo struct {
	Original  string `json:"original"`
	Thumb320  string `json:"thumb_320x180"`
	Thumb640  string `json:"thumb_640x360"`
	Thumb1280 string `json:"thumb_1280x720"`
	Filename  string `json:"filename"`
}

type modioUser struct {
	Username string `json:"username"`
}

type modioFile struct {
	ID        int64         `json:"id"`
	DateAdded int64         `json:"date_added"`
	Version   string        `json:"version"`
	Filename  string        `json:"filename"`
	Filesize  int64         `json:"filesize"`
	Changelog string        `json:"changelog"`
	Download  modioDownload `json:"download"`
}

type modioDownload struct {
	BinaryURL string `json:"binary_url"`
}

type modioStats struct {
	DownloadsTotal int64 `json:"downloads_total"`
}

type modioTag struct {
	Name string `json:"name"`
}

type modioDependency struct {
	ModID int64  `json:"mod_id"`
	Name  string `json:"name"`
}

func effectiveAPIKey(rt driver.Runtime, src driver.Source) string {
	return driver.FirstNonEmpty(src.APIKey, rt.ModIOAPIKey())
}

func parseModIOEndpoint(raw string) (apiBase, gameID string, err error) {
	trimmed := strings.TrimRight(strings.TrimSpace(raw), "/")
	if trimmed == "" {
		return "", "", fmt.Errorf("%w: mod.io upstream requires endpoint ending with /games/<gameID>", driver.ErrInvalidRequest)
	}
	u, err := url.Parse(trimmed)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", "", fmt.Errorf("%w: invalid mod.io endpoint %q", driver.ErrInvalidRequest, raw)
	}
	segs := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i := 0; i+1 < len(segs); i++ {
		if strings.EqualFold(segs[i], "games") && segs[i+1] != "" {
			prefix := strings.Join(segs[:i], "/")
			base := u.Scheme + "://" + u.Host
			if prefix != "" {
				base += "/" + prefix
			}
			return base, segs[i+1], nil
		}
	}
	return "", "", fmt.Errorf("%w: mod.io endpoint %q must include /games/<gameID>", driver.ErrInvalidRequest, raw)
}

func mapModIOSort(sortMode string) string {
	switch sortMode {
	case driver.SortTrend:
		return "-popular"
	case driver.SortTopRated:
		return "-rating"
	case driver.SortNewest:
		return "-date_added"
	case driver.SortUpdated:
		return "-date_live"
	default:
		return "-downloads_total"
	}
}

func fetchModRecord(ctx context.Context, rt driver.Runtime, apiBase, gameID, key, id string) (*modioMod, error) {
	trimmed := strings.TrimSpace(id)
	if driver.IsDigitsOnly(trimmed) {
		u := fmt.Sprintf("%s/games/%s/mods/%s?api_key=%s", apiBase, url.PathEscape(gameID), url.PathEscape(trimmed), url.QueryEscape(key))
		var m modioMod
		if err := rt.DoGetJSON(ctx, u, &m); err != nil {
			return nil, err
		}
		if m.ID <= 0 {
			return nil, driver.ErrNotFound
		}
		return &m, nil
	}
	q := url.Values{}
	q.Set("api_key", key)
	q.Set("name_id", trimmed)
	q.Set("_limit", "1")
	u := fmt.Sprintf("%s/games/%s/mods?%s", apiBase, url.PathEscape(gameID), q.Encode())
	var env modsResponse
	if err := rt.DoGetJSON(ctx, u, &env); err != nil {
		return nil, err
	}
	if len(env.Data) == 0 {
		return nil, driver.ErrNotFound
	}
	return &env.Data[0], nil
}

func fetchModFiles(ctx context.Context, rt driver.Runtime, apiBase, gameID, key string, modID int64) []modioFile {
	u := fmt.Sprintf("%s/games/%s/mods/%d/files?api_key=%s&_limit=%d&_sort=-id",
		apiBase, url.PathEscape(gameID), modID, url.QueryEscape(key), maxAvailableVersions)
	var env modfilesResponse
	if err := rt.DoGetJSON(ctx, u, &env); err != nil {
		return nil
	}
	return env.Data
}

func fetchModDependencies(ctx context.Context, rt driver.Runtime, apiBase, gameID, key string, modID int64) []driver.Dependency {
	u := fmt.Sprintf("%s/games/%s/mods/%d/dependencies?api_key=%s",
		apiBase, url.PathEscape(gameID), modID, url.QueryEscape(key))
	var env dependenciesResponse
	if err := rt.DoGetJSON(ctx, u, &env); err != nil || len(env.Data) == 0 {
		return nil
	}
	out := make([]driver.Dependency, 0, len(env.Data))
	seen := make(map[string]bool, len(env.Data))
	for _, d := range env.Data {
		if d.ModID <= 0 {
			continue
		}
		depID := strconv.FormatInt(d.ModID, 10)
		if !seen[depID] {
			seen[depID] = true
			out = append(out, driver.Dependency{ID: depID, Title: driver.FirstNonEmpty(d.Name, depID)})
		}
	}
	return out
}

func fileVersionString(f modioFile) string {
	if v := strings.TrimSpace(f.Version); v != "" {
		return v
	}
	if f.ID > 0 {
		return strconv.FormatInt(f.ID, 10)
	}
	return ""
}

func buildModIOItem(m modioMod, files []modioFile, deps []driver.Dependency, includeDesc bool) driver.Item {
	id := strconv.FormatInt(m.ID, 10)
	vers := extractVersions(m.Modfile, files)
	latestVer := ""
	if len(vers) > 0 {
		latestVer = vers[0]
	}
	tags := make([]string, 0, len(m.Tags))
	for _, t := range m.Tags {
		if name := strings.TrimSpace(t.Name); name != "" {
			tags = append(tags, name)
		}
	}
	var updated string
	if m.DateUpdated > 0 {
		updated = time.Unix(m.DateUpdated, 0).UTC().Format(time.RFC3339)
	}
	item := driver.Item{
		ID: id, Title: driver.FirstNonEmpty(m.Name, m.NameID, id),
		Summary: strings.TrimSpace(m.Summary), Author: strings.TrimSpace(m.SubmittedBy.Username),
		IconURL:           driver.FirstNonEmpty(m.Logo.Thumb320, m.Logo.Thumb640, m.Logo.Original),
		SourceURL:         strings.TrimSpace(m.ProfileURL),
		UpstreamURL:       driver.FirstNonEmpty(m.HomepageURL, m.ProfileURL),
		FileURL:           strings.TrimSpace(m.Modfile.Download.BinaryURL),
		FileName:          strings.TrimSpace(m.Modfile.Filename),
		LatestVersion:     latestVer,
		AvailableVersions: vers,
		Downloads:         m.Stats.DownloadsTotal,
		SizeBytes:         m.Modfile.Filesize,
		UpdatedAt:         updated,
		Tags:              tags,
		Dependencies:      deps,
	}
	if includeDesc {
		item.Description = driver.FirstNonEmpty(m.Description, m.DescHTML, m.Summary)
		item.Changelog = strings.TrimSpace(m.Modfile.Changelog)
	}
	return item
}

func extractVersions(primary modioFile, files []modioFile) []string {
	var vers []string
	add := func(v string) {
		trimmed := strings.TrimSpace(v)
		if trimmed != "" && !slices.Contains(vers, trimmed) {
			vers = append(vers, trimmed)
		}
	}
	add(fileVersionString(primary))
	for _, f := range files {
		add(fileVersionString(f))
	}
	return vers
}
