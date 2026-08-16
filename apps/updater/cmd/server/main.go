package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

var versionPattern = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?$`)

type job struct {
	ID        string `json:"id,omitempty"`
	Version   string `json:"version,omitempty"`
	Status    string `json:"status,omitempty"`
	Stage     string `json:"stage,omitempty"`
	Message   string `json:"message,omitempty"`
	StartedAt string `json:"startedAt,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
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
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.Handle("GET /status", u.authorize(http.HandlerFunc(u.status)))
	mux.Handle("POST /apply", u.authorize(http.HandlerFunc(u.apply)))
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
	u.job = job{ID: fmt.Sprintf("panel-update-%d", now.UnixNano()), Version: payload.Version, Status: "running", Stage: "queued", Message: "Update queued", StartedAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339)}
	current := u.job
	u.persistLocked()
	u.mu.Unlock()
	go u.run(payload.Version)
	writeJSON(w, http.StatusAccepted, current)
}

func (u *updater) run(version string) {
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
	u.job.Message = "Panel update completed"
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
