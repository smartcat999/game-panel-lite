package systemupdate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/buildinfo"
)

const maxResponseBytes = 1 << 20

var versionPattern = regexp.MustCompile(`^v?([0-9]+)\.([0-9]+)\.([0-9]+)(?:[-+]([0-9A-Za-z.-]+))?$`)

type Manifest struct {
	SchemaVersion   int               `json:"schemaVersion"`
	Channel         string            `json:"channel"`
	Version         string            `json:"version"`
	PublishedAt     string            `json:"publishedAt,omitempty"`
	ReleaseNotesURL string            `json:"releaseNotesUrl,omitempty"`
	Images          map[string]Images `json:"images,omitempty"`
}

type Images struct {
	DockerHub string `json:"dockerHub,omitempty"`
	Aliyun    string `json:"aliyun,omitempty"`
	Digest    string `json:"digest,omitempty"`
}

type Job struct {
	ID        string `json:"id,omitempty"`
	Version   string `json:"version,omitempty"`
	Status    string `json:"status,omitempty"`
	Stage     string `json:"stage,omitempty"`
	Message   string `json:"message,omitempty"`
	StartedAt string `json:"startedAt,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

type Status struct {
	Current          buildinfo.Info `json:"current"`
	Latest           *Manifest      `json:"latest,omitempty"`
	UpdateAvailable  bool           `json:"updateAvailable"`
	CheckedAt        string         `json:"checkedAt,omitempty"`
	CheckError       string         `json:"checkError,omitempty"`
	AutoCheckEnabled bool           `json:"autoCheckEnabled"`
	IntervalHours    int            `json:"intervalHours"`
	UpdaterAvailable bool           `json:"updaterAvailable"`
	Job              *Job           `json:"job,omitempty"`
}

type Service struct {
	manifestURL  string
	updaterURL   string
	updaterToken string
	client       *http.Client
	current      buildinfo.Info

	mu         sync.RWMutex
	latest     *Manifest
	checkedAt  time.Time
	checkError string
}

func New(manifestURL, updaterURL, updaterToken string, timeout time.Duration) *Service {
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	return &Service{
		manifestURL:  strings.TrimSpace(manifestURL),
		updaterURL:   strings.TrimRight(strings.TrimSpace(updaterURL), "/"),
		updaterToken: strings.TrimSpace(updaterToken),
		client:       &http.Client{Timeout: timeout},
		current:      buildinfo.Current(),
	}
}

func (s *Service) Check(ctx context.Context, autoCheck bool, intervalHours int) (Status, error) {
	if s.manifestURL == "" {
		err := errors.New("release manifest URL is not configured")
		s.setCheckError(err)
		return s.snapshot(autoCheck, intervalHours, nil), err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.manifestURL, nil)
	if err != nil {
		return s.snapshot(autoCheck, intervalHours, nil), err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		s.setCheckError(err)
		return s.snapshot(autoCheck, intervalHours, nil), err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("release manifest returned HTTP %d", resp.StatusCode)
		s.setCheckError(err)
		return s.snapshot(autoCheck, intervalHours, nil), err
	}
	var manifest Manifest
	decoder := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&manifest); err != nil {
		err = fmt.Errorf("invalid release manifest: %w", err)
		s.setCheckError(err)
		return s.snapshot(autoCheck, intervalHours, nil), err
	}
	if manifest.SchemaVersion != 1 || !validVersion(manifest.Version) {
		err = errors.New("release manifest has an unsupported schema or version")
		s.setCheckError(err)
		return s.snapshot(autoCheck, intervalHours, nil), err
	}
	s.mu.Lock()
	s.latest = &manifest
	s.checkedAt = time.Now().UTC()
	s.checkError = ""
	s.mu.Unlock()
	return s.snapshot(autoCheck, intervalHours, nil), nil
}

func (s *Service) Status(ctx context.Context, autoCheck bool, intervalHours int) Status {
	var job *Job
	if s.updaterURL != "" {
		if current, err := s.updaterRequest(ctx, http.MethodGet, "/status", nil); err == nil {
			job = current
		}
	}
	return s.snapshot(autoCheck, intervalHours, job)
}

func (s *Service) Apply(ctx context.Context, version string) (*Job, error) {
	version = strings.TrimSpace(version)
	if s.updaterURL == "" {
		return nil, errors.New("panel updater is not configured")
	}
	if !validVersion(version) {
		return nil, errors.New("invalid target version")
	}
	s.mu.RLock()
	latest := s.latest
	s.mu.RUnlock()
	if latest == nil || latest.Version != version {
		return nil, errors.New("target version does not match the checked release")
	}
	if compareVersions(version, s.current.Version) <= 0 {
		return nil, errors.New("panel is already up to date")
	}
	return s.updaterRequest(ctx, http.MethodPost, "/apply", map[string]string{"version": version})
}

func (s *Service) snapshot(autoCheck bool, intervalHours int, job *Job) Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	status := Status{
		Current:          s.current,
		AutoCheckEnabled: autoCheck,
		IntervalHours:    intervalHours,
		UpdaterAvailable: s.updaterURL != "" && s.updaterToken != "",
		CheckError:       s.checkError,
		Job:              job,
	}
	if s.latest != nil {
		copy := *s.latest
		status.Latest = &copy
		status.UpdateAvailable = compareVersions(copy.Version, s.current.Version) > 0
	}
	if !s.checkedAt.IsZero() {
		status.CheckedAt = s.checkedAt.Format(time.RFC3339)
	}
	return status
}

func (s *Service) setCheckError(err error) {
	s.mu.Lock()
	s.checkedAt = time.Now().UTC()
	s.checkError = err.Error()
	s.mu.Unlock()
}

func (s *Service) updaterRequest(ctx context.Context, method, path string, payload any) (*Job, error) {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, s.updaterURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if s.updaterToken != "" {
		req.Header.Set("Authorization", "Bearer "+s.updaterToken)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("panel updater is unavailable: %w", err)
	}
	defer resp.Body.Close()
	var job Job
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&job); err != nil {
		return nil, fmt.Errorf("invalid panel updater response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if job.Message == "" {
			job.Message = fmt.Sprintf("panel updater returned HTTP %d", resp.StatusCode)
		}
		return nil, errors.New(job.Message)
	}
	return &job, nil
}

func validVersion(value string) bool {
	return versionPattern.MatchString(strings.TrimSpace(value))
}

func compareVersions(left, right string) int {
	l := versionPattern.FindStringSubmatch(strings.TrimSpace(left))
	r := versionPattern.FindStringSubmatch(strings.TrimSpace(right))
	if len(l) == 0 || len(r) == 0 {
		return strings.Compare(left, right)
	}
	for i := 1; i <= 3; i++ {
		lv, _ := strconv.Atoi(l[i])
		rv, _ := strconv.Atoi(r[i])
		if lv < rv {
			return -1
		}
		if lv > rv {
			return 1
		}
	}
	if l[4] == r[4] {
		return 0
	}
	if l[4] == "" {
		return 1
	}
	if r[4] == "" {
		return -1
	}
	return strings.Compare(l[4], r[4])
}
