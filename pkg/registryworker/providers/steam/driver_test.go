package steam

import (
	"os"
	"slices"
	"strings"
	"testing"

	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror"
)

func TestBuildServerUpdateArgs(t *testing.T) {
	tests := []struct {
		name      string
		appID     string
		branch    string
		platforms string
		want      []string
	}{
		{
			name:      "default single linux branch",
			appID:     "2394010",
			branch:    "latest",
			platforms: "",
			want:      []string{"+app_info_update", "1", "+app_update", "2394010", "validate"},
		},
		{
			name:      "multi platform annotation windows then linux",
			appID:     "222860",
			branch:    "latest",
			platforms: "windows,linux",
			want: []string{
				"+app_info_update", "1",
				"+@sSteamCmdForcePlatformType", "windows", "+app_update", "222860",
				"+@sSteamCmdForcePlatformType", "linux", "+app_update", "222860",
				"validate",
			},
		},
		{
			name:      "single platform with explicit beta branch",
			appID:     "294420",
			branch:    "alpha21",
			platforms: "linux",
			want: []string{
				"+app_info_update", "1",
				"+@sSteamCmdForcePlatformType", "linux", "+app_update", "294420",
				"-beta", "alpha21",
				"validate",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := buildServerUpdateArgs(tc.appID, tc.branch, tc.platforms)
			if !slices.Equal(got, tc.want) {
				t.Fatalf("buildServerUpdateArgs(%q, %q, %q) = %v, want %v", tc.appID, tc.branch, tc.platforms, got, tc.want)
			}
		})
	}
}

func TestResolveSteamPlatformsPrecedence(t *testing.T) {
	up := &registryv1.Upstream{
		Metadata: metav1.ObjectMeta{
			Annotations: map[string]string{registryv1.AnnotationSteamPlatforms: "windows,linux"},
		},
	}
	if got := resolveSteamPlatforms(nil, up); got != "windows,linux" {
		t.Fatalf("expected upstream annotation fallback, got %q", got)
	}
	art := &registryv1.Artifact{
		Metadata: metav1.ObjectMeta{
			Annotations: map[string]string{registryv1.AnnotationSteamPlatforms: "linux"},
		},
	}
	if got := resolveSteamPlatforms(art, up); got != "linux" {
		t.Fatalf("expected artifact annotation override, got %q", got)
	}
}

func TestBuildCS2WorkshopManifestPayload(t *testing.T) {
	tmp := t.TempDir()
	art := &registryv1.Artifact{
		Metadata: metav1.ObjectMeta{Name: "cs2-workshop.3070244462.1700"},
		Spec:     registryv1.ArtifactSpec{Repository: "cs2-workshop", Package: "3070244462", Version: "1700"},
	}
	item := &addonmirror.Item{ID: "3070244462", Title: "Aim Botz", UpdatedAt: "2026-11-20T00:00:00Z"}
	rel := &addonmirror.ResolvedRelease{Build: "rev-3", UpstreamURL: "https://steamcommunity.com/sharedfiles/filedetails/?id=3070244462"}
	got, err := buildCS2WorkshopManifestPayload(nil, art, "3070244462", item, rel, tmp)
	if err != nil {
		t.Fatalf("buildCS2WorkshopManifestPayload: %v", err)
	}
	if got.ArchiveFormat != registryv1.ArchiveFormatRaw || got.FileName != "workshop-3070244462.json" ||
		!slices.Equal(got.MapFolders, []string{"3070244462"}) || got.SHA256Hex == "" || got.SizeBytes == 0 {
		t.Fatalf("unexpected cs2 workshop payload: %+v", got)
	}
	raw, err := os.ReadFile(got.LocalPath)
	if err != nil || !strings.Contains(string(raw), `"workshopId":"3070244462"`) || !strings.Contains(string(raw), `"appId":730`) {
		t.Fatalf("unexpected manifest file content: err=%v raw=%q", err, string(raw))
	}
}
