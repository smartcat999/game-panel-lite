package docker

import (
	"context"
	"io"
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/docker/docker/client"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/runtime"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestValidateGameUpdateRequestRequiresSafeJobID(t *testing.T) {
	dataDir := t.TempDir()
	valid := runtime.GameUpdateRequest{JobID: "job-123", RuntimeID: "gamepanel-palworld", Image: "palworld:latest", DataDir: dataDir, AppID: "2394010"}
	if got, err := validateGameUpdateRequest(valid); err != nil || got == "" {
		t.Fatalf("expected valid request, path=%q err=%v", got, err)
	}
	invalid := valid
	invalid.JobID = "../unsafe"
	if _, err := validateGameUpdateRequest(invalid); err == nil {
		t.Fatal("expected unsafe update job ID to fail validation")
	}
	invalid = valid
	invalid.DataDir = filepath.Join(dataDir, "missing")
	if _, err := validateGameUpdateRequest(invalid); err == nil {
		t.Fatal("expected missing update data directory to fail validation")
	}
}

func TestValidateGameUpdateCheckRequestDoesNotRequireServerInstance(t *testing.T) {
	request := runtime.GameUpdateRequest{JobID: "provider-check-1", Image: "palworld:latest", AppID: "2394010"}
	if err := validateGameUpdateCheckRequest(request); err != nil {
		t.Fatalf("expected provider-scoped check request to be valid: %v", err)
	}
}

func TestGameUpdateMetadataCheckUsesIsolatedLowResources(t *testing.T) {
	resources := gameUpdaterResources(true)
	if resources.NanoCPUs != 250_000_000 {
		t.Fatalf("expected metadata check to use 0.25 CPU, got %d NanoCPUs", resources.NanoCPUs)
	}
	if resources.Memory != 512*1024*1024 {
		t.Fatalf("expected metadata check to use 512 MiB, got %d bytes", resources.Memory)
	}
	if mounts := gameUpdaterMounts("/srv/palworld", true); len(mounts) != 0 {
		t.Fatalf("expected metadata check not to mount live game data, got %#v", mounts)
	}
}

func TestGameUpdateApplyKeepsGameDataMountAndResources(t *testing.T) {
	resources := gameUpdaterResources(false)
	if resources.NanoCPUs != 2_000_000_000 || resources.Memory != 1536*1024*1024 {
		t.Fatalf("unexpected apply resources: %#v", resources)
	}
	mounts := gameUpdaterMounts("/srv/palworld", false)
	if len(mounts) != 1 || mounts[0].Source != "/srv/palworld" || mounts[0].Target != "/palworld" || mounts[0].ReadOnly {
		t.Fatalf("expected writable game data mount for update apply, got %#v", mounts)
	}
}

func TestCleanupGameUpdateFiltersByJobAndForceRemovesHelpers(t *testing.T) {
	var mu sync.Mutex
	filterValue := ""
	removed := ""
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		status := http.StatusOK
		body := ""
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/containers/json"):
			mu.Lock()
			filterValue = r.URL.Query().Get("filters")
			mu.Unlock()
			body = `[{"Id":"updater-1","Names":["/gamepanel-updater"]}]`
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/containers/updater-1"):
			if r.URL.Query().Get("force") != "1" {
				status = http.StatusBadRequest
				body = "force removal required"
				break
			}
			mu.Lock()
			removed = "updater-1"
			mu.Unlock()
			status = http.StatusNoContent
		default:
			status = http.StatusNotFound
		}
		return &http.Response{
			StatusCode: status,
			Status:     http.StatusText(status),
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    r,
		}, nil
	})}
	cli, err := client.NewClientWithOpts(client.WithHost("http://docker.test"), client.WithVersion("1.47"), client.WithHTTPClient(httpClient))
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close()
	adapter := &Adapter{client: cli, host: "http://docker.test"}
	if err := adapter.CleanupGameUpdate(context.Background(), "job-123"); err != nil {
		t.Fatalf("cleanup game updater: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if removed != "updater-1" || !strings.Contains(filterValue, "gamepanel.job=game-update") || !strings.Contains(filterValue, "gamepanel.update-job=job-123") {
		t.Fatalf("expected labeled helper to be force removed, removed=%q filters=%q", removed, filterValue)
	}
}

func TestParseManifestBuildID(t *testing.T) {
	manifest := `"AppState"
{
	"appid" "2394010"
	"buildid" "24181105"
	"TargetBuildID" "24181106"
}`
	got, err := parseManifestBuildID(manifest)
	if err != nil {
		t.Fatalf("parse manifest build ID: %v", err)
	}
	if got != "24181105" {
		t.Fatalf("expected installed build 24181105, got %q", got)
	}
}

func TestParsePublicBuildIDIgnoresOtherBranches(t *testing.T) {
	appInfo := `AppID : 2394010, change number : 30000000
"2394010"
{
	"depots"
	{
		"branches"
		{
			"public"
			{
				"buildid" "24181105"
				"timeupdated" "1784122262"
			}
			"previous"
			{
				"buildid" "24000000"
			}
		}
	}
}`
	got, err := parsePublicBuildID(appInfo)
	if err != nil {
		t.Fatalf("parse public build ID: %v", err)
	}
	if got != "24181105" {
		t.Fatalf("expected public build 24181105, got %q", got)
	}
}

func TestParsePublicBuildIDStripsANSI(t *testing.T) {
	appInfo := "\x1b[0m\"branches\"\n{\n\"public\"\n{\n\"buildid\"\t\"24181105\"\n}\n}\n"
	got, err := parsePublicBuildID(appInfo)
	if err != nil {
		t.Fatalf("parse ANSI app info build ID: %v", err)
	}
	if got != "24181105" {
		t.Fatalf("expected public build 24181105, got %q", got)
	}
}

func TestParseBuildIDsRejectMissingValues(t *testing.T) {
	if _, err := parseManifestBuildID(`"AppState" { "appid" "2394010" }`); err == nil {
		t.Fatal("expected missing manifest build ID to fail")
	}
	if _, err := parsePublicBuildID(`"branches" { "beta" { "buildid" "1" } }`); err == nil {
		t.Fatal("expected missing public branch build ID to fail")
	}
}

func TestValidateInstalledBuildRequiresTargetOrNewer(t *testing.T) {
	for _, test := range []struct {
		installed string
		target    string
		wantError bool
	}{
		{installed: "24181105", target: "24181105"},
		{installed: "24181106", target: "24181105"},
		{installed: "24180000", target: "24181105", wantError: true},
		{installed: "invalid", target: "24181105", wantError: true},
	} {
		err := validateInstalledBuild(test.installed, test.target)
		if (err != nil) != test.wantError {
			t.Fatalf("validateInstalledBuild(%q, %q) error=%v, wantError=%v", test.installed, test.target, err, test.wantError)
		}
	}
}

func TestGameUpdateProgressTrackerMapsSteamPhases(t *testing.T) {
	var got []runtime.GameUpdateProgress
	tracker := gameUpdateProgressTracker{callback: func(progress runtime.GameUpdateProgress) {
		got = append(got, progress)
	}}
	for _, line := range []string{
		"GAMEPANEL_UPDATE_PHASE=refreshing_metadata",
		"[ 50%] Downloading update (20,000 of 40,000 KB)...",
		"GAMEPANEL_UPDATE_PHASE=applying",
		"GAMEPANEL_UPDATE_PHASE=recovering_manifest",
		"Update state (0x5) verifying install, progress: 50.00",
		"Update state (0x61) downloading, progress: 25.00",
		"[----] Installing update...",
		"Success! App '2394010' fully installed.",
		"GAMEPANEL_UPDATE_PHASE=finalizing",
		"GAMEPANEL_UPDATE_PHASE=complete",
	} {
		tracker.consume(line)
	}

	want := []runtime.GameUpdateProgress{
		{Stage: runtime.GameUpdateStageRefreshingMetadata, Progress: 5, Message: "Refreshing Steam metadata"},
		{Stage: runtime.GameUpdateStageValidating, Progress: 10, Message: "Validating installed game files"},
		{Stage: runtime.GameUpdateStageValidating, Progress: 12, Message: "Repairing stale Steam manifest and retrying"},
		{Stage: runtime.GameUpdateStageValidating, Progress: 25, Message: "Update state (0x5) verifying install, progress: 50.00"},
		{Stage: runtime.GameUpdateStageDownloading, Progress: 50, Message: "Update state (0x61) downloading, progress: 25.00"},
		{Stage: runtime.GameUpdateStageInstalling, Progress: 82, Message: "[----] Installing update..."},
		{Stage: runtime.GameUpdateStageInstalling, Progress: 88, Message: "Success! App '2394010' fully installed."},
		{Stage: runtime.GameUpdateStageFinalizing, Progress: 90, Message: "Finalizing game update"},
		{Stage: runtime.GameUpdateStageFinalizing, Progress: 92, Message: "Game files updated"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected progress events:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestProgressPercentSupportsSteamFormats(t *testing.T) {
	for _, test := range []struct {
		line string
		want int
	}{
		{"[ 42%] Downloading update", 42},
		{"Update state verifying, progress: 73.80", 73},
		{"[150%] invalid oversized value", 100},
		{"no progress", 0},
	} {
		if got := progressPercent(test.line); got != test.want {
			t.Fatalf("progressPercent(%q)=%d, want %d", test.line, got, test.want)
		}
	}
}
