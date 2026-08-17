package main

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/mail"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

var versionPattern = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?$`)
var domainPattern = regexp.MustCompile(`(?i)^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)

var controlPlaneServices = []string{"updater", "api", "web", "nginx", "gamepanel-exporter", "prometheus", "cadvisor", "node-exporter"}

type job struct {
	ID        string `json:"id,omitempty"`
	Kind      string `json:"kind,omitempty"`
	Version   string `json:"version,omitempty"`
	Status    string `json:"status,omitempty"`
	Stage     string `json:"stage,omitempty"`
	Message   string `json:"message,omitempty"`
	StartedAt string `json:"startedAt,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

type deploymentService struct {
	Name   string `json:"name"`
	State  string `json:"state"`
	Health string `json:"health,omitempty"`
	Image  string `json:"image,omitempty"`
}

type httpsStatus struct {
	Configured    bool              `json:"configured"`
	Domain        string            `json:"domain,omitempty"`
	Certificate   string            `json:"certificate"`
	ExpiresAt     string            `json:"expiresAt,omitempty"`
	DaysRemaining int               `json:"daysRemaining,omitempty"`
	AutoRenewal   autoRenewalStatus `json:"autoRenewal"`
}

type autoRenewalStatus struct {
	Enabled       bool   `json:"enabled"`
	Method        string `json:"method,omitempty"`
	InstalledAt   string `json:"installedAt,omitempty"`
	LastCheckedAt string `json:"lastCheckedAt,omitempty"`
	LastStatus    string `json:"lastStatus,omitempty"`
}

type deploymentStatus struct {
	Mode         string                 `json:"mode"`
	Manager      string                 `json:"manager"`
	CheckedAt    string                 `json:"checkedAt"`
	Capabilities deploymentCapabilities `json:"capabilities"`
	Healthy      bool                   `json:"healthy"`
	Services     []deploymentService    `json:"services"`
	HTTPS        httpsStatus            `json:"https"`
	Job          job                    `json:"job,omitempty"`
}

type deploymentCapabilities struct {
	Reconcile  bool `json:"reconcile"`
	Restart    bool `json:"restart"`
	HTTPSSetup bool `json:"httpsSetup"`
	HTTPSRenew bool `json:"httpsRenew"`
}

type updater struct {
	workspace string
	token     string
	logger    *slog.Logger
	mu        sync.RWMutex
	job       job
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	u := &updater{
		workspace: env("GAMEPANEL_UPDATER_WORKSPACE", "/workspace"),
		token:     strings.TrimSpace(os.Getenv("GAMEPANEL_UPDATER_TOKEN")),
		logger:    logger,
	}
	u.load()
	go u.runRenewalScheduler(context.Background())
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.Handle("GET /status", u.authorize(http.HandlerFunc(u.status)))
	mux.Handle("POST /apply", u.authorize(http.HandlerFunc(u.apply)))
	mux.Handle("GET /deployment", u.authorize(http.HandlerFunc(u.deployment)))
	mux.Handle("POST /deployment/reconcile", u.authorize(http.HandlerFunc(u.reconcile)))
	mux.Handle("POST /deployment/restart", u.authorize(http.HandlerFunc(u.restart)))
	mux.Handle("POST /deployment/https/setup", u.authorize(http.HandlerFunc(u.setupHTTPS)))
	mux.Handle("POST /deployment/https/renew", u.authorize(http.HandlerFunc(u.renewHTTPS)))
	addr := env("GAMEPANEL_UPDATER_HOST", "0.0.0.0") + ":" + env("GAMEPANEL_UPDATER_PORT", "4020")
	logger.Info("panel updater listening", "addr", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		logger.Error("panel updater stopped", "error", err)
		os.Exit(1)
	}
}

func (u *updater) authorize(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u.token == "" || r.Header.Get("Authorization") != "Bearer "+u.token {
			writeJSON(w, http.StatusUnauthorized, job{Status: "failed", Message: "unauthorized"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (u *updater) status(w http.ResponseWriter, _ *http.Request) {
	u.mu.RLock()
	defer u.mu.RUnlock()
	writeJSON(w, http.StatusOK, u.job)
}

func (u *updater) deployment(w http.ResponseWriter, r *http.Request) {
	status, err := u.readDeploymentStatus(r.Context())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (u *updater) reconcile(w http.ResponseWriter, r *http.Request) {
	if !u.composeManaged(r.Context()) {
		writeJSON(w, http.StatusConflict, job{Status: "failed", Message: "the current deployment manager does not support service recovery"})
		return
	}
	u.queueMaintenance(w, "reconcile", "Restoring control-plane services", func() error {
		args := append(u.composePrefix(), "up", "-d", "--remove-orphans", "--pull", "never")
		args = append(args, controlPlaneServices...)
		return u.command(nil, args...)
	})
}

func (u *updater) restart(w http.ResponseWriter, r *http.Request) {
	if !u.composeManaged(r.Context()) {
		writeJSON(w, http.StatusConflict, job{Status: "failed", Message: "the current deployment manager does not support control-plane restart"})
		return
	}
	u.queueMaintenance(w, "restart", "Restarting control-plane services", func() error {
		services := []string{"api", "web", "nginx", "gamepanel-exporter", "prometheus", "cadvisor", "node-exporter"}
		args := append(u.composePrefix(), "restart")
		args = append(args, services...)
		return u.command(nil, args...)
	})
}

func (u *updater) setupHTTPS(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Domain string `json:"domain"`
		Email  string `json:"email"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, job{Status: "failed", Message: "invalid request body"})
		return
	}
	payload.Domain = strings.ToLower(strings.TrimSpace(payload.Domain))
	payload.Email = strings.TrimSpace(payload.Email)
	if !domainPattern.MatchString(payload.Domain) {
		writeJSON(w, http.StatusBadRequest, job{Status: "failed", Message: "invalid domain"})
		return
	}
	if payload.Email != "" {
		address, err := mail.ParseAddress(payload.Email)
		if err != nil || address.Address != payload.Email {
			writeJSON(w, http.StatusBadRequest, job{Status: "failed", Message: "invalid email"})
			return
		}
	}
	if !u.composeManaged(r.Context()) {
		writeJSON(w, http.StatusConflict, job{Status: "failed", Message: "the current deployment manager does not support HTTPS setup"})
		return
	}
	u.queueMaintenance(w, "https-setup", "Configuring HTTPS", func() error {
		args := []string{filepath.Join(u.workspace, "scripts", "setup-https.sh"), payload.Domain}
		if payload.Email != "" {
			args = append(args, payload.Email)
		}
		if err := u.execCommand("sh", args...); err != nil {
			return err
		}
		u.writeAutoRenewalStatus("updater", "pending", true)
		return nil
	})
}

func (u *updater) renewHTTPS(w http.ResponseWriter, r *http.Request) {
	if !u.composeManaged(r.Context()) {
		writeJSON(w, http.StatusConflict, job{Status: "failed", Message: "the current deployment manager does not support HTTPS renewal"})
		return
	}
	if _, err := os.Stat(filepath.Join(u.workspace, "data", "nginx", "gamepanel-https.conf")); err != nil {
		writeJSON(w, http.StatusConflict, job{Status: "failed", Message: "HTTPS is not configured"})
		return
	}
	u.queueMaintenance(w, "https-renew", "Checking HTTPS certificate renewal", func() error {
		return u.runHTTPSRenewal()
	})
}

func (u *updater) queueMaintenance(w http.ResponseWriter, kind, message string, operation func() error) {
	current, err := u.startMaintenance(kind, message, operation)
	if err != nil {
		writeJSON(w, http.StatusConflict, current)
		return
	}
	writeJSON(w, http.StatusAccepted, current)
}

func (u *updater) startMaintenance(kind, message string, operation func() error) (job, error) {
	u.mu.Lock()
	if u.job.Status == "running" {
		current := u.job
		u.mu.Unlock()
		return current, errors.New("another maintenance operation is running")
	}
	now := time.Now().UTC()
	u.job = job{ID: fmt.Sprintf("deployment-%d", now.UnixNano()), Kind: kind, Status: "running", Stage: "queued", Message: message, StartedAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339)}
	current := u.job
	u.persistLocked()
	u.mu.Unlock()
	go func() {
		if err := u.setStage("applying", message); err != nil {
			u.fail(err)
			return
		}
		if err := operation(); err != nil {
			u.fail(err)
			return
		}
		u.complete()
	}()
	return current, nil
}

func (u *updater) runRenewalScheduler(ctx context.Context) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			https := u.readHTTPSStatus()
			if !https.Configured || https.AutoRenewal.Method == "systemd" {
				continue
			}
			_, _ = u.startMaintenance("https-renew", "Running automatic HTTPS certificate renewal check", u.runHTTPSRenewal)
		}
	}
}

func (u *updater) runHTTPSRenewal() error {
	if err := u.execCommand("sh", filepath.Join(u.workspace, "scripts", "renew-https.sh")); err != nil {
		u.writeAutoRenewalStatus("updater", "failed", true)
		return err
	}
	u.writeAutoRenewalStatus("updater", "success", true)
	return nil
}

func (u *updater) writeAutoRenewalStatus(method, status string, enabled bool) {
	value := autoRenewalStatus{Enabled: enabled, Method: method, LastCheckedAt: time.Now().UTC().Format(time.RFC3339), LastStatus: status}
	data, err := json.Marshal(value)
	if err != nil {
		return
	}
	path := filepath.Join(u.workspace, "data", "certbot", "renewal-status.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return
	}
	_ = os.WriteFile(path, append(data, '\n'), 0o644)
}

func (u *updater) apply(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&payload); err != nil || !versionPattern.MatchString(payload.Version) {
		writeJSON(w, http.StatusBadRequest, job{Status: "failed", Message: "invalid target version"})
		return
	}
	u.mu.Lock()
	if u.job.Status == "running" {
		current := u.job
		u.mu.Unlock()
		writeJSON(w, http.StatusConflict, current)
		return
	}
	now := time.Now().UTC()
	u.job = job{ID: fmt.Sprintf("panel-update-%d", now.UnixNano()), Kind: "update", Version: payload.Version, Status: "running", Stage: "queued", Message: "Update queued", StartedAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339)}
	current := u.job
	u.persistLocked()
	u.mu.Unlock()
	go u.runUpdate(payload.Version)
	writeJSON(w, http.StatusAccepted, current)
}

func (u *updater) runUpdate(version string) {
	if err := u.setStage("preparing", "Preparing panel update"); err != nil {
		u.fail(err)
		return
	}
	if err := updateEnvFile(filepath.Join(u.workspace, ".env"), "GAMEPANEL_IMAGE_TAG", normalizeVersion(version)); err != nil {
		u.fail(err)
		return
	}
	args := []string{"compose", "-f", "compose.prod.yaml"}
	if _, err := os.Stat(filepath.Join(u.workspace, "data/nginx/gamepanel-https.conf")); err == nil {
		args = append(args, "-f", "compose.https.yaml")
	}
	if err := u.setStage("pulling", "Downloading panel images"); err != nil {
		u.fail(err)
		return
	}
	if err := u.command(args, "pull", "api", "web", "gamepanel-exporter", "updater"); err != nil {
		u.fail(err)
		return
	}
	if err := u.setStage("applying", "Restarting panel services"); err != nil {
		u.fail(err)
		return
	}
	if err := u.command(args, "up", "-d", "--remove-orphans", "--pull", "never", "api", "web", "gamepanel-exporter"); err != nil {
		u.fail(err)
		return
	}
	u.complete()
}

func (u *updater) composePrefix() []string {
	args := []string{"compose", "-f", "compose.prod.yaml"}
	if _, err := os.Stat(filepath.Join(u.workspace, "data", "nginx", "gamepanel-https.conf")); err == nil {
		args = append(args, "-f", "compose.https.yaml")
	}
	return args
}

func (u *updater) readDeploymentStatus(ctx context.Context) (deploymentStatus, error) {
	services, _ := u.readComposeServices(ctx)
	byName := make(map[string]deploymentService, len(services))
	for _, service := range services {
		byName[service.Name] = service
	}
	if byName["updater"].State != "running" {
		return u.readStandaloneStatus(ctx), nil
	}

	ordered := make([]deploymentService, 0, len(controlPlaneServices))
	healthy := true
	for _, name := range controlPlaneServices {
		service, ok := byName[name]
		if !ok {
			service = deploymentService{Name: name, State: "missing"}
		}
		if service.State != "running" || service.Health == "unhealthy" {
			healthy = false
		}
		ordered = append(ordered, service)
	}
	return u.deploymentSnapshot("docker-compose", deploymentCapabilities{Reconcile: true, Restart: true, HTTPSSetup: true, HTTPSRenew: true}, healthy, ordered), nil
}

func (u *updater) readComposeServices(ctx context.Context) ([]deploymentService, error) {
	args := append(u.composePrefix(), "ps", "--format", "json")
	args = append(args, controlPlaneServices...)
	output, err := u.commandOutput(ctx, nil, args...)
	if err != nil {
		return nil, err
	}
	services, err := parseComposeServices(output)
	if err != nil {
		return nil, err
	}
	return services, nil
}

func (u *updater) composeManaged(ctx context.Context) bool {
	services, err := u.readComposeServices(ctx)
	if err != nil {
		return false
	}
	for _, service := range services {
		if service.Name == "updater" && service.State == "running" {
			return true
		}
	}
	return false
}

func (u *updater) readStandaloneStatus(ctx context.Context) deploymentStatus {
	services := []deploymentService{{Name: "updater", State: "running"}}
	healthy := true
	probes := []struct {
		name string
		url  string
	}{
		{name: "api", url: env("GAMEPANEL_API_HEALTH_URL", "http://127.0.0.1:4000/healthz")},
		{name: "web", url: env("GAMEPANEL_WEB_HEALTH_URL", "")},
	}
	for _, probe := range probes {
		if probe.url == "" {
			continue
		}
		service := probeHTTPService(ctx, probe.name, probe.url)
		if service.State != "running" {
			healthy = false
		}
		services = append(services, service)
	}
	return u.deploymentSnapshot("standalone", deploymentCapabilities{}, healthy, services)
}

func probeHTTPService(ctx context.Context, name, target string) deploymentService {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return deploymentService{Name: name, State: "unavailable"}
	}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return deploymentService{Name: name, State: "unavailable"}
	}
	_ = resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 500 {
		return deploymentService{Name: name, State: "unavailable"}
	}
	return deploymentService{Name: name, State: "running"}
}

func (u *updater) deploymentSnapshot(manager string, capabilities deploymentCapabilities, healthy bool, services []deploymentService) deploymentStatus {
	u.mu.RLock()
	currentJob := u.job
	u.mu.RUnlock()
	https := u.readHTTPSStatus()
	mode := "http"
	if https.Configured {
		mode = "https"
	}
	return deploymentStatus{Mode: mode, Manager: manager, CheckedAt: time.Now().UTC().Format(time.RFC3339), Capabilities: capabilities, Healthy: healthy, Services: services, HTTPS: https, Job: currentJob}
}

func (u *updater) readHTTPSStatus() httpsStatus {
	domain := readEnvValue(filepath.Join(u.workspace, ".env"), "GAMEPANEL_DOMAIN")
	configured := false
	if _, err := os.Stat(filepath.Join(u.workspace, "data", "nginx", "gamepanel-https.conf")); err == nil {
		configured = true
	}
	autoRenewal := u.readAutoRenewalStatus()
	if configured && autoRenewal.LastStatus == "unknown" {
		autoRenewal = autoRenewalStatus{Enabled: true, Method: "updater", LastStatus: "pending"}
	}
	status := httpsStatus{Configured: configured, Domain: domain, Certificate: "missing", AutoRenewal: autoRenewal}
	if !configured || domain == "" {
		return status
	}
	data, err := os.ReadFile(filepath.Join(u.workspace, "data", "certbot", "conf", "live", domain, "fullchain.pem"))
	if err != nil {
		return status
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return status
	}
	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return status
	}
	status.Certificate = "valid"
	status.ExpiresAt = certificate.NotAfter.UTC().Format(time.RFC3339)
	status.DaysRemaining = int(time.Until(certificate.NotAfter).Hours() / 24)
	if time.Now().After(certificate.NotAfter) {
		status.Certificate = "expired"
	}
	return status
}

func (u *updater) readAutoRenewalStatus() autoRenewalStatus {
	data, err := os.ReadFile(filepath.Join(u.workspace, "data", "certbot", "renewal-status.json"))
	if err != nil {
		return autoRenewalStatus{LastStatus: "unknown"}
	}
	var status autoRenewalStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return autoRenewalStatus{LastStatus: "unknown"}
	}
	return status
}

func parseComposeServices(data []byte) ([]deploymentService, error) {
	type composeService struct {
		Service string `json:"Service"`
		State   string `json:"State"`
		Health  string `json:"Health"`
		Image   string `json:"Image"`
	}
	var rows []composeService
	if err := json.Unmarshal(data, &rows); err != nil {
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		for _, line := range lines {
			if strings.TrimSpace(line) == "" {
				continue
			}
			var row composeService
			if lineErr := json.Unmarshal([]byte(line), &row); lineErr != nil {
				return nil, fmt.Errorf("invalid docker compose status: %w", err)
			}
			rows = append(rows, row)
		}
	}
	result := make([]deploymentService, 0, len(rows))
	for _, row := range rows {
		if row.Service == "" {
			continue
		}
		result = append(result, deploymentService{Name: row.Service, State: strings.ToLower(row.State), Health: strings.ToLower(row.Health), Image: row.Image})
	}
	return result, nil
}

func readEnvValue(path, key string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, key+"=") {
			continue
		}
		return strings.Trim(strings.TrimSpace(strings.TrimPrefix(trimmed, key+"=")), `"'`)
	}
	return ""
}

func (u *updater) command(prefix []string, args ...string) error {
	commandArgs := append(append([]string{}, prefix...), args...)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", commandArgs...)
	cmd.Dir = u.workspace
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker compose failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func (u *updater) execCommand(name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = u.workspace
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("maintenance command failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func (u *updater) commandOutput(ctx context.Context, prefix []string, args ...string) ([]byte, error) {
	commandArgs := append(append([]string{}, prefix...), args...)
	cmd := exec.CommandContext(ctx, "docker", commandArgs...)
	cmd.Dir = u.workspace
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("docker compose failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return output, nil
}

func (u *updater) setStage(stage, message string) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.job.Status != "running" {
		return errors.New("update job is no longer running")
	}
	u.job.Stage = stage
	u.job.Message = message
	u.job.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	u.persistLocked()
	return nil
}

func (u *updater) fail(err error) {
	u.logger.Error("panel update failed", "error", err)
	u.mu.Lock()
	defer u.mu.Unlock()
	u.job.Status = "failed"
	u.job.Stage = "failed"
	u.job.Message = err.Error()
	u.job.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	u.persistLocked()
}

func (u *updater) complete() {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.job.Status = "completed"
	u.job.Stage = "completed"
	if u.job.Kind == "update" {
		u.job.Message = "Panel update completed"
	} else {
		u.job.Message = "Maintenance operation completed"
	}
	u.job.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	u.persistLocked()
}

func (u *updater) statePath() string {
	return filepath.Join(u.workspace, "data/system-update-job.json")
}

func (u *updater) persistLocked() {
	data, err := json.MarshalIndent(u.job, "", "  ")
	if err != nil {
		return
	}
	path := u.statePath()
	_ = os.MkdirAll(filepath.Dir(path), 0o750)
	_ = os.WriteFile(path+".tmp", data, 0o600)
	_ = os.Rename(path+".tmp", path)
}

func (u *updater) load() {
	data, err := os.ReadFile(u.statePath())
	if err == nil {
		_ = json.Unmarshal(data, &u.job)
		if u.job.Status == "running" {
			u.job.Status = "failed"
			u.job.Stage = "interrupted"
			u.job.Message = "Updater restarted before the previous job completed"
		}
	}
}

func updateEnvFile(path, key, value string) error {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	lines := strings.Split(string(data), "\n")
	found := false
	for index, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), key+"=") {
			lines[index] = key + "=\"" + value + "\""
			found = true
		}
	}
	if !found {
		lines = append(lines, key+"=\""+value+"\"")
	}
	content := strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
	if err := os.WriteFile(path+".tmp", []byte(content), 0o600); err != nil {
		return err
	}
	return os.Rename(path+".tmp", path)
}

func normalizeVersion(version string) string {
	if strings.HasPrefix(version, "v") {
		return version
	}
	return "v" + version
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
