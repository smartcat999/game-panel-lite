package steam

import (
	"slices"
	"testing"

	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
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
