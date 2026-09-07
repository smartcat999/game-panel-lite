package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/smartcat999/game-panel-lite/internal/workload"
)

func reconcileAssignments(ctx context.Context, client *http.Client, cfg AgentConfig, logger *slog.Logger, runtime agentRuntime) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.MasterURL+"/api/agent/assignments", nil)
	if err != nil {
		return
	}
	req.Header.Set("X-Node-Token", cfg.Token)
	capabilities := workload.ExecutionLeaseCapability
	if cfg.ArtifactsEnabled {
		capabilities += "," + workload.ArtifactCapability
	}
	req.Header.Set("X-Workload-Capabilities", capabilities)
	resp, err := client.Do(req)
	if err != nil {
		logger.Warn("failed to fetch workload assignments", "error", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		logger.Warn("master rejected workload assignment poll", "status", resp.StatusCode)
		return
	}
	var assignments []workload.Assignment
	if err := json.NewDecoder(resp.Body).Decode(&assignments); err != nil {
		logger.Warn("failed to decode workload assignments", "error", err)
		return
	}
	for _, assignment := range assignments {
		if ctx.Err() != nil {
			return
		}
		_, err := reconcileLeasedAssignment(ctx, client, cfg, logger, assignment, runtime)
		if err != nil {
			logger.Warn("leased workload reconciliation failed", "server_id", assignment.ServerID, "error", err)
			continue
		}
	}
}

func reportWorkloadObservation(ctx context.Context, client *http.Client, cfg AgentConfig, assignment workload.Assignment, observation workload.Observation) error {
	payload, err := json.Marshal(observation)
	if err != nil {
		return err
	}
	endpoint := fmt.Sprintf("%s/api/agent/assignments/%s/status", cfg.MasterURL, url.PathEscape(assignment.UID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Node-Token", cfg.Token)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("master returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}
