// Package regionopsclient routes authenticated global-control reads to the
// Region that owns the requested operational data.
package regionopsclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regions"
)

var ErrUnavailable = errors.New("regional operations endpoint unavailable")
var ErrInvalidResponse = errors.New("invalid regional operations response")

type FileConfig struct {
	CertificateFile  string            `json:"certificateFile"`
	KeyFile          string            `json:"keyFile"`
	ServerCAFile     string            `json:"serverCaFile"`
	Timeout          string            `json:"timeout"`
	MaxResponseBytes int64             `json:"maxResponseBytes"`
	Endpoints        map[string]string `json:"endpoints"`
}

type Directory struct {
	endpoints map[string]*url.URL
	http      *http.Client
	transport *http.Transport
	maxBytes  int64
}

func Load(path string) (*Directory, error) {
	encoded, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(strings.NewReader(string(encoded)))
	decoder.DisallowUnknownFields()
	var config FileConfig
	if err := decoder.Decode(&config); err != nil {
		return nil, err
	}
	if err := requireEOF(decoder); err != nil {
		return nil, err
	}
	timeout, err := time.ParseDuration(config.Timeout)
	if err != nil || timeout <= 0 || timeout > time.Minute || config.MaxResponseBytes < 1 || config.MaxResponseBytes > 8<<20 {
		return nil, errors.New("invalid regional operations client limits")
	}
	certificate, err := tls.LoadX509KeyPair(config.CertificateFile, config.KeyFile)
	if err != nil {
		return nil, errors.New("cannot load global control client certificate")
	}
	caPEM, err := os.ReadFile(config.ServerCAFile)
	if err != nil {
		return nil, errors.New("cannot load regional operations server CA")
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("regional operations server CA contains no certificates")
	}
	return NewDirectory(config.Endpoints, certificate, pool, timeout, config.MaxResponseBytes)
}

func NewDirectory(endpoints map[string]string, certificate tls.Certificate, roots *x509.CertPool, timeout time.Duration, maxBytes int64) (*Directory, error) {
	if len(endpoints) == 0 || len(certificate.Certificate) == 0 || certificate.PrivateKey == nil || roots == nil || timeout <= 0 || timeout > time.Minute || maxBytes < 1 || maxBytes > 8<<20 {
		return nil, errors.New("invalid regional operations client configuration")
	}
	parsed := make(map[string]*url.URL, len(endpoints))
	for regionID, endpoint := range endpoints {
		if regions.ValidateIdentity(regionID, regionID) != nil {
			return nil, errors.New("invalid regional operations Region ID")
		}
		u, err := url.Parse(endpoint)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" || endpoint != u.String() {
			return nil, errors.New("invalid regional operations endpoint")
		}
		parsed[regionID] = u
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots.Clone(), Certificates: []tls.Certificate{certificate}}}
	return &Directory{endpoints: parsed, transport: transport, http: &http.Client{Transport: transport, Timeout: timeout}, maxBytes: maxBytes}, nil
}

func (d *Directory) ListRegionalNodes(ctx context.Context, regionID, after string, limit int) (regional.NodeOperationsPage, error) {
	endpoint, ok := d.endpoints[regionID]
	if !ok {
		return regional.NodeOperationsPage{}, ErrUnavailable
	}
	if limit < 1 || limit > 200 || len(after) > 128 || after != strings.TrimSpace(after) {
		return regional.NodeOperationsPage{}, regional.ErrInvalidNodeOperations
	}
	target := *endpoint
	target.Path = "/internal/operations/nodes"
	query := target.Query()
	query.Set("limit", fmt.Sprint(limit))
	if after != "" {
		query.Set("after", after)
	}
	target.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return regional.NodeOperationsPage{}, err
	}
	response, err := d.http.Do(request)
	if err != nil {
		return regional.NodeOperationsPage{}, ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, d.maxBytes))
		return regional.NodeOperationsPage{}, ErrUnavailable
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return regional.NodeOperationsPage{}, ErrInvalidResponse
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, d.maxBytes+1))
	if err != nil || int64(len(body)) > d.maxBytes {
		return regional.NodeOperationsPage{}, ErrInvalidResponse
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var page regional.NodeOperationsPage
	if err := decoder.Decode(&page); err != nil || requireEOF(decoder) != nil || page.RegionID != regionID || page.Validate() != nil {
		return regional.NodeOperationsPage{}, ErrInvalidResponse
	}
	return page, nil
}

func (d *Directory) Close() error {
	if d != nil && d.transport != nil {
		d.transport.CloseIdleConnections()
	}
	return nil
}

func requireEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return ErrInvalidResponse
		}
		return err
	}
	return nil
}
