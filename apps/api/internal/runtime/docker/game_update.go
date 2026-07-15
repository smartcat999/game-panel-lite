package docker

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/runtime"
)

const (
	checkGameUpdateScript = `set -eu
echo GAMEPANEL_UPDATE_PHASE=refreshing_metadata
exec /home/steam/steamcmd/steamcmd.sh +login anonymous +app_info_update 1 +app_info_print "$GAMEPANEL_APP_ID" +quit
`
	applyGameUpdateScript = `set -eu
echo GAMEPANEL_UPDATE_PHASE=refreshing_metadata
/home/steam/steamcmd/steamcmd.sh +login anonymous +app_info_update 1 +app_info_print "$GAMEPANEL_APP_ID" +quit
echo GAMEPANEL_UPDATE_PHASE=applying
manifest="/palworld/steamapps/appmanifest_${GAMEPANEL_APP_ID}.acf"
stale_manifest="${manifest}.gamepanel-stale"
original_manifest="${manifest}.gamepanel-original"
original_manifest_tmp="${original_manifest}.tmp"
mutation_started=0
finish_update() {
  status=$?
  if [ "$status" -ne 0 ] && [ "$mutation_started" -eq 1 ]; then
    rm -f "$manifest"
    if [ -f "$original_manifest" ]; then
      mv "$original_manifest" "$manifest"
    fi
  else
    rm -f "$original_manifest"
  fi
  rm -f "$stale_manifest" "$original_manifest_tmp"
  if [ -f /palworld/PalServer.sh ]; then
    chmod +x /palworld/PalServer.sh || true
  fi
  trap - EXIT
  exit "$status"
}
trap finish_update EXIT
rm -f "$stale_manifest" "$original_manifest_tmp"
if [ -f "$original_manifest" ]; then
  rm -f "$manifest"
  mv "$original_manifest" "$manifest"
fi
if [ -f "$manifest" ]; then
  cp -p "$manifest" "$original_manifest_tmp"
  mv "$original_manifest_tmp" "$original_manifest"
fi
mutation_started=1
run_update() {
  /home/steam/steamcmd/steamcmd.sh +force_install_dir /palworld +login anonymous +app_update "$GAMEPANEL_APP_ID" validate +quit
}
manifest_ready() {
  [ -f "$manifest" ] && grep -Eq '^[[:space:]]*"StateFlags"[[:space:]]+"4"[[:space:]]*$' "$manifest"
}
if ! run_update || ! manifest_ready; then
  echo GAMEPANEL_UPDATE_PHASE=recovering_manifest
  rm -f "$stale_manifest"
  if [ -f "$manifest" ]; then
    mv "$manifest" "$stale_manifest"
  fi
  if ! run_update || ! manifest_ready; then
    echo "Steam manifest recovery failed for app $GAMEPANEL_APP_ID" >&2
    exit 1
  fi
  rm -f "$stale_manifest"
fi
echo GAMEPANEL_UPDATE_PHASE=finalizing
chmod +x /palworld/PalServer.sh
echo GAMEPANEL_UPDATE_PHASE=complete
`
)

var (
	appIDPattern       = regexp.MustCompile(`^[0-9]+$`)
	vdfTokenPattern    = regexp.MustCompile(`"([^"]*)"|[{}]`)
	ansiPattern        = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)
	bracketPercent     = regexp.MustCompile(`\[\s*([0-9]{1,3})%\]`)
	statePercent       = regexp.MustCompile(`(?i)progress:\s*([0-9]+(?:\.[0-9]+)?)`)
	containerNameClean = regexp.MustCompile(`[^a-zA-Z0-9_.-]+`)
)

func (a *Adapter) CheckGameUpdate(ctx context.Context, request runtime.GameUpdateRequest) (runtime.GameUpdateResult, error) {
	dataDir, err := validateGameUpdateRequest(request)
	if err != nil {
		return runtime.GameUpdateResult{}, err
	}
	installed, err := readInstalledBuildID(dataDir, request.AppID)
	if err != nil {
		return runtime.GameUpdateResult{}, err
	}
	output, err := a.runGameUpdater(ctx, request, dataDir, true, checkGameUpdateScript, nil)
	if err != nil {
		return runtime.GameUpdateResult{}, err
	}
	latest, err := parsePublicBuildID(output)
	if err != nil {
		return runtime.GameUpdateResult{}, fmt.Errorf("parse latest Steam build for app %s: %w", request.AppID, err)
	}
	return runtime.GameUpdateResult{InstalledBuildID: installed, LatestBuildID: latest}, nil
}

func (a *Adapter) ApplyGameUpdate(ctx context.Context, request runtime.GameUpdateRequest, onProgress runtime.GameUpdateProgressFunc) (runtime.GameUpdateResult, error) {
	dataDir, err := validateGameUpdateRequest(request)
	if err != nil {
		return runtime.GameUpdateResult{}, err
	}
	inspected, err := a.client.ContainerInspect(ctx, request.RuntimeID)
	if err == nil && inspected.State != nil && inspected.State.Running {
		return runtime.GameUpdateResult{}, fmt.Errorf("game server container must be stopped before applying an update")
	}
	if err != nil && !client.IsErrNotFound(err) {
		return runtime.GameUpdateResult{}, fmt.Errorf("inspect game server before update: %w", err)
	}
	output, err := a.runGameUpdater(ctx, request, dataDir, false, applyGameUpdateScript, onProgress)
	if err != nil {
		return runtime.GameUpdateResult{}, err
	}
	installed, err := readInstalledBuildID(dataDir, request.AppID)
	if err != nil {
		return runtime.GameUpdateResult{}, err
	}
	if installed == "" {
		return runtime.GameUpdateResult{}, fmt.Errorf("Steam manifest for app %s was not created after update", request.AppID)
	}
	latest, parseErr := parsePublicBuildID(output)
	if parseErr != nil {
		latest = installed
	} else if err := validateInstalledBuild(installed, latest); err != nil {
		return runtime.GameUpdateResult{}, err
	}
	return runtime.GameUpdateResult{InstalledBuildID: installed, LatestBuildID: latest}, nil
}

func validateInstalledBuild(installed, target string) error {
	if installed == target {
		return nil
	}
	installedNumber, installedErr := strconv.ParseUint(installed, 10, 64)
	targetNumber, targetErr := strconv.ParseUint(target, 10, 64)
	if installedErr == nil && targetErr == nil && installedNumber > targetNumber {
		return nil
	}
	return fmt.Errorf("installed Steam build %s did not reach target build %s", installed, target)
}

func validateGameUpdateRequest(request runtime.GameUpdateRequest) (string, error) {
	if strings.TrimSpace(request.JobID) == "" || containerNameClean.ReplaceAllString(request.JobID, "") != request.JobID {
		return "", fmt.Errorf("invalid game update job ID")
	}
	if strings.TrimSpace(request.RuntimeID) == "" {
		return "", fmt.Errorf("game update runtime ID is required")
	}
	if strings.TrimSpace(request.Image) == "" {
		return "", fmt.Errorf("game update image is required")
	}
	if !appIDPattern.MatchString(request.AppID) {
		return "", fmt.Errorf("invalid Steam app ID %q", request.AppID)
	}
	if strings.TrimSpace(request.DataDir) == "" {
		return "", fmt.Errorf("game update data directory is required")
	}
	dataDir, err := filepath.Abs(request.DataDir)
	if err != nil {
		return "", fmt.Errorf("resolve game update data directory: %w", err)
	}
	info, err := os.Stat(dataDir)
	if err != nil {
		return "", fmt.Errorf("inspect game update data directory: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("game update data path is not a directory")
	}
	return dataDir, nil
}

func (a *Adapter) runGameUpdater(
	ctx context.Context,
	request runtime.GameUpdateRequest,
	dataDir string,
	readOnly bool,
	script string,
	onProgress runtime.GameUpdateProgressFunc,
) (outputText string, returnErr error) {
	if err := a.ensureImage(ctx, request.Image); err != nil {
		return "", err
	}
	namePart := containerNameClean.ReplaceAllString(request.RuntimeID, "-")
	if len(namePart) > 48 {
		namePart = namePart[:48]
	}
	containerName := fmt.Sprintf("gamepanel-updater-%s-%d", namePart, time.Now().UnixNano())
	memoryLimit := int64(1536 * 1024 * 1024)
	if readOnly {
		memoryLimit = 512 * 1024 * 1024
	}
	resp, err := a.client.ContainerCreate(ctx, &container.Config{
		Image:      request.Image,
		Entrypoint: []string{"/bin/sh", "-lc"},
		Cmd:        []string{script},
		Env:        []string{"GAMEPANEL_APP_ID=" + request.AppID},
		User:       "0:0",
		Tty:        true,
		Labels: map[string]string{
			"gamepanel.update-instance": request.RuntimeID,
			"gamepanel.job":             "game-update",
			"gamepanel.update-job":      request.JobID,
		},
	}, &container.HostConfig{
		AutoRemove:  false,
		SecurityOpt: []string{"no-new-privileges:true"},
		Resources: container.Resources{
			NanoCPUs: 2_000_000_000,
			Memory:   memoryLimit,
		},
		Mounts: []mount.Mount{{
			Type:     mount.TypeBind,
			Source:   dataDir,
			Target:   "/palworld",
			ReadOnly: readOnly,
		}},
	}, nil, nil, containerName)
	if err != nil {
		return "", fmt.Errorf("create controlled game updater: %w", err)
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cleanupErr := a.client.ContainerRemove(cleanupCtx, resp.ID, types.ContainerRemoveOptions{Force: true, RemoveVolumes: true})
		if cleanupErr == nil || client.IsErrNotFound(cleanupErr) {
			return
		}
		if returnErr != nil {
			returnErr = fmt.Errorf("%w; cleanup controlled game updater: %v", returnErr, cleanupErr)
			return
		}
		returnErr = fmt.Errorf("cleanup controlled game updater: %w", cleanupErr)
	}()
	if err := a.client.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		return "", fmt.Errorf("start controlled game updater: %w", err)
	}
	waitCh, waitErrCh := a.client.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	logs, err := a.client.ContainerLogs(ctx, resp.ID, types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
	})
	if err != nil {
		return "", fmt.Errorf("stream controlled game updater logs: %w", err)
	}
	defer logs.Close()

	tracker := gameUpdateProgressTracker{callback: onProgress}
	var output strings.Builder
	scanner := bufio.NewScanner(logs)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(ansiPattern.ReplaceAllString(scanner.Text(), ""))
		if line == "" {
			continue
		}
		if output.Len() < 8*1024*1024 {
			output.WriteString(line)
			output.WriteByte('\n')
		}
		tracker.consume(line)
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, context.Canceled) {
		return output.String(), fmt.Errorf("read controlled game updater logs: %w", err)
	}

	for waitCh != nil || waitErrCh != nil {
		select {
		case waitErr, ok := <-waitErrCh:
			if !ok {
				waitErrCh = nil
				continue
			}
			if waitErr != nil {
				return output.String(), fmt.Errorf("wait for controlled game updater: %w", waitErr)
			}
		case result, ok := <-waitCh:
			if !ok {
				waitCh = nil
				continue
			}
			if result.Error != nil {
				return output.String(), fmt.Errorf("controlled game updater failed: %s", result.Error.Message)
			}
			if result.StatusCode != 0 {
				return output.String(), fmt.Errorf("controlled game updater exited with code %d: %s", result.StatusCode, outputTail(output.String(), 8))
			}
			return output.String(), nil
		case <-ctx.Done():
			return output.String(), ctx.Err()
		}
	}
	return output.String(), fmt.Errorf("controlled game updater exited without a status")
}

func (a *Adapter) CleanupGameUpdate(ctx context.Context, jobID string) error {
	if strings.TrimSpace(jobID) == "" || containerNameClean.ReplaceAllString(jobID, "") != jobID {
		return fmt.Errorf("invalid game update job ID")
	}
	containers, err := a.client.ContainerList(ctx, types.ContainerListOptions{
		All: true,
		Filters: filters.NewArgs(
			filters.Arg("label", "gamepanel.job=game-update"),
			filters.Arg("label", "gamepanel.update-job="+jobID),
		),
	})
	if err != nil {
		return fmt.Errorf("list interrupted game updater containers: %w", err)
	}
	for _, item := range containers {
		if err := a.client.ContainerRemove(ctx, item.ID, types.ContainerRemoveOptions{Force: true, RemoveVolumes: true}); err != nil && !client.IsErrNotFound(err) {
			return fmt.Errorf("remove interrupted game updater container %s: %w", item.ID, err)
		}
	}
	return nil
}

func readInstalledBuildID(dataDir, appID string) (string, error) {
	path := filepath.Join(dataDir, "steamapps", "appmanifest_"+appID+".acf")
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read installed Steam manifest: %w", err)
	}
	buildID, err := parseManifestBuildID(string(content))
	if err != nil {
		return "", fmt.Errorf("parse installed Steam manifest: %w", err)
	}
	return buildID, nil
}

func parseManifestBuildID(input string) (string, error) {
	if value := findVDFScalar(input, []string{"AppState"}, "buildid"); value != "" {
		return value, nil
	}
	return "", fmt.Errorf("buildid was not found")
}

func parsePublicBuildID(input string) (string, error) {
	input = ansiPattern.ReplaceAllString(input, "")
	if value := findVDFScalar(input, []string{"branches", "public"}, "buildid"); value != "" {
		return value, nil
	}
	return "", fmt.Errorf("public branch buildid was not found")
}

type vdfToken struct {
	value string
	brace byte
}

func tokenizeVDF(input string) []vdfToken {
	matches := vdfTokenPattern.FindAllStringSubmatch(input, -1)
	tokens := make([]vdfToken, 0, len(matches))
	for _, match := range matches {
		if match[0] == "{" || match[0] == "}" {
			tokens = append(tokens, vdfToken{brace: match[0][0]})
			continue
		}
		tokens = append(tokens, vdfToken{value: match[1]})
	}
	return tokens
}

func findVDFScalar(input string, parentSuffix []string, key string) string {
	tokens := tokenizeVDF(input)
	stack := make([]string, 0, 8)
	for i := 0; i < len(tokens); i++ {
		token := tokens[i]
		if token.brace == '}' {
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			continue
		}
		if token.brace != 0 || token.value == "" || i+1 >= len(tokens) {
			continue
		}
		next := tokens[i+1]
		if next.brace == '{' {
			stack = append(stack, token.value)
			i++
			continue
		}
		if next.brace == 0 && token.value == key && pathHasSuffix(stack, parentSuffix) {
			return next.value
		}
		if next.brace == 0 {
			i++
		}
	}
	return ""
}

func pathHasSuffix(path, suffix []string) bool {
	if len(path) < len(suffix) {
		return false
	}
	start := len(path) - len(suffix)
	for i := range suffix {
		if path[start+i] != suffix[i] {
			return false
		}
	}
	return true
}

type gameUpdateProgressTracker struct {
	callback runtime.GameUpdateProgressFunc
	stage    string
	progress int
}

func (t *gameUpdateProgressTracker) consume(line string) {
	lower := strings.ToLower(line)
	switch line {
	case "GAMEPANEL_UPDATE_PHASE=refreshing_metadata":
		t.emit(runtime.GameUpdateStageRefreshingMetadata, 5, "Refreshing Steam metadata")
		return
	case "GAMEPANEL_UPDATE_PHASE=applying":
		t.emit(runtime.GameUpdateStageValidating, 10, "Validating installed game files")
		return
	case "GAMEPANEL_UPDATE_PHASE=recovering_manifest":
		t.emit(runtime.GameUpdateStageValidating, 12, "Repairing stale Steam manifest and retrying")
		return
	case "GAMEPANEL_UPDATE_PHASE=finalizing":
		t.emit(runtime.GameUpdateStageFinalizing, 90, "Finalizing game update")
		return
	case "GAMEPANEL_UPDATE_PHASE=complete":
		// Leave room for the orchestration layer's optional start and health
		// check phases. It owns the terminal 100% state.
		t.emit(runtime.GameUpdateStageFinalizing, 92, "Game files updated")
		return
	}
	if t.stage == runtime.GameUpdateStageRefreshingMetadata {
		return
	}
	percent := progressPercent(line)
	switch {
	case strings.Contains(lower, "verifying") || strings.Contains(lower, "validating"):
		t.emit(runtime.GameUpdateStageValidating, 10+percent*30/100, line)
	case strings.Contains(lower, "downloading"):
		t.emit(runtime.GameUpdateStageDownloading, 40+percent*40/100, line)
	case strings.Contains(lower, "installing") || strings.Contains(lower, "extracting") || strings.Contains(lower, "fully installed"):
		progress := 82
		if strings.Contains(lower, "fully installed") {
			progress = 88
		}
		t.emit(runtime.GameUpdateStageInstalling, progress, line)
	}
}

func (t *gameUpdateProgressTracker) emit(stage string, progress int, message string) {
	if t.callback == nil || (stage == t.stage && progress == t.progress) {
		return
	}
	t.stage, t.progress = stage, progress
	t.callback(runtime.GameUpdateProgress{Stage: stage, Progress: progress, Message: message})
}

func progressPercent(line string) int {
	match := bracketPercent.FindStringSubmatch(line)
	if len(match) == 2 {
		value, _ := strconv.Atoi(match[1])
		return min(value, 100)
	}
	match = statePercent.FindStringSubmatch(line)
	if len(match) == 2 {
		value, _ := strconv.ParseFloat(match[1], 64)
		return min(int(value), 100)
	}
	return 0
}

func outputTail(output string, lines int) string {
	parts := strings.Split(strings.TrimSpace(output), "\n")
	if len(parts) > lines {
		parts = parts[len(parts)-lines:]
	}
	return strings.Join(parts, " | ")
}

var _ runtime.GameUpdateExecutor = (*Adapter)(nil)
var _ runtime.GameUpdateCleaner = (*Adapter)(nil)
var _ runtime.WorkloadHealthInspector = (*Adapter)(nil)
