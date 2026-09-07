package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/smartcat999/game-panel-lite/internal/workload"
)

// artifactSource derives every request from the configured control plane. Remote
// manifests never supply a URL, and redirects never receive the node credential.
type artifactSource struct {
	base   string
	token  string
	client http.Client
}

func newArtifactSource(masterURL, token string, client *http.Client) (*artifactSource, error) {
	base, err := url.Parse(masterURL)
	if err != nil || base == nil || (base.Scheme != "https" && base.Scheme != "http") || base.Host == "" || base.User != nil || base.RawQuery != "" || base.ForceQuery || base.Fragment != "" || strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("artifact delivery requires an HTTP(S) control-plane URL without credentials, query or fragment and a node token")
	}
	copied := *client
	copied.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	copied.Jar = nil
	return &artifactSource{base: strings.TrimRight(base.String(), "/"), token: token, client: copied}, nil
}

func (s *artifactSource) Open(ctx context.Context, assignment workload.Assignment, item workload.Artifact) (io.ReadCloser, error) {
	if assignment.Generation <= 0 || assignment.UID == "" || len(assignment.UID) > 128 {
		return nil, fmt.Errorf("invalid artifact assignment identity")
	}
	for _, ch := range assignment.UID {
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '-' || ch == '_') {
			return nil, fmt.Errorf("invalid artifact assignment UID")
		}
	}
	if err := workload.ValidateArtifacts(workload.Options{Artifacts: []workload.Artifact{item}}); err != nil {
		return nil, err
	}
	endpoint := fmt.Sprintf("%s/api/agent/assignments/%s/artifacts/%s?generation=%d", s.base, url.PathEscape(assignment.UID), url.PathEscape(item.ID), assignment.Generation)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Node-Token", s.token)
	req.Header.Set("Accept-Encoding", "identity")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK || resp.ContentLength != item.SizeBytes || resp.Header.Get("ETag") != strconv.Quote(item.SHA256) || (resp.Header.Get("Content-Encoding") != "" && resp.Header.Get("Content-Encoding") != "identity") {
		resp.Body.Close()
		return nil, fmt.Errorf("artifact response rejected (status %d): expected exact length, digest ETag and identity encoding", resp.StatusCode)
	}
	// The runtime verifies the complete stream's byte count and SHA-256 before
	// replacing a container. Closing this body also cancels network resources.
	return resp.Body, nil
}

func (cfg AgentConfig) workloadCapabilities() []string {
	if cfg.ArtifactsEnabled {
		return []string{workload.ArtifactCapability}
	}
	return nil
}
