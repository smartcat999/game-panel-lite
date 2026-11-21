package registryworker

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/client"
	"github.com/smartcat999/game-panel-cloud/pkg/config"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror"
	"github.com/smartcat999/game-panel-cloud/pkg/registryworker/driver"
	"github.com/smartcat999/game-panel-cloud/pkg/registryworker/providers/steam"
)

func TestReusableArtifactPayload(t *testing.T) {
	const digest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	var blobStatus atomic.Int32
	blobStatus.Store(http.StatusOK)

	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(req.URL.Path, "/apis/registry.gamepanel.io/v1/blobs/") {
			st := int(blobStatus.Load())
			rw.WriteHeader(st)
			if st == http.StatusOK {
				_ = json.NewEncoder(rw).Encode(registryv1.Blob{Digest: digest, SizeBytes: 4294967296, MediaType: "application/gzip"})
			}
			return
		}
		rw.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cli, err := client.New(&config.ClientConfig{ServerURL: srv.URL, Token: "test-token", Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}
	w := New(&config.Config{}, cli)
	art := &registryv1.Artifact{
		Metadata: metav1.ObjectMeta{Name: "palworld-server.palworld.latest"},
		Spec:     registryv1.ArtifactSpec{Repository: "palworld-server", Package: "palworld", Version: "latest", ArchiveFormat: registryv1.ArchiveFormatTarGz},
		Status: registryv1.ArtifactStatus{
			Build:      "25247047",
			Descriptor: &registryv1.BlobDescriptor{Digest: digest, SizeBytes: 4294967296, FileName: "package.tar.gz", ArchiveFormat: registryv1.ArchiveFormatTarGz},
		},
	}

	got := w.ReusableArtifactPayload(context.Background(), nil, art, "25247047", nil, "https://store.steampowered.com/news/app/2394010")
	wantURL := "https://store.steampowered.com/news/app/1623730"
	if got == nil || got.SizeBytes != 4294967296 || got.Build != "25247047" || "sha256:"+got.SHA256Hex != digest || got.UpstreamURL != wantURL {
		t.Fatalf("expected reusable payload for unchanged build with normalized URL, got %#v", got)
	}
	if diff := w.ReusableArtifactPayload(context.Background(), nil, art, "25249999", nil, ""); diff != nil {
		t.Fatalf("expected nil payload when buildID changed, got %#v", diff)
	}
	blobStatus.Store(http.StatusNotFound)
	if missing := w.ReusableArtifactPayload(context.Background(), nil, art, "25247047", nil, ""); missing != nil {
		t.Fatalf("expected nil payload when blob is absent in storage, got %#v", missing)
	}
}

func TestUploadMultipartPartRetryOn5xx(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "chunk.bin")
	if err := os.WriteFile(tmp, []byte("hello-palworld-chunk"), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	f, err := os.Open(tmp)
	if err != nil {
		t.Fatalf("open temp file: %v", err)
	}
	defer func() { _ = f.Close() }()

	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		if string(body) != "hello-palworld-chunk" {
			t.Fatalf("unexpected chunk body on attempt: %q", string(body))
		}
		if attempts.Add(1) == 1 {
			rw.WriteHeader(http.StatusInternalServerError)
			_, _ = rw.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><Error><Code>InternalError</Code></Error>`))
			return
		}
		rw.Header().Set("ETag", `"etag-ok"`)
		rw.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	w := New(&config.Config{}, nil)
	w.uploadRetryDelay = time.Millisecond
	etag, err := w.uploadMultipartPartWithRetry(context.Background(), nil, f, &byteSpeedTracker{}, 0, 1, 20, 0, 20, srv.URL)
	if err != nil {
		t.Fatalf("expected retry to recover from transient 500, got err: %v", err)
	}
	if etag != `"etag-ok"` || attempts.Load() != 2 {
		t.Fatalf("unexpected etag=%q attempts=%d", etag, attempts.Load())
	}
}

func TestLookupSyncDriverMatrix(t *testing.T) {
	cases := []struct {
		name      string
		sourceURL string
		format    string
		provider  string
		want      string
		wantErr   bool
	}{
		{name: "steamcmd-scheme", sourceURL: "steamcmd://108600", want: "steam-server"},
		{name: "steam-server-repo", provider: registryv1.UpstreamProviderSteam, format: registryv1.FormatServerPackage, want: "steam-server"},
		{name: "steam-workshop-with-ugc-url", provider: registryv1.UpstreamProviderSteam, format: registryv1.FormatWorkshopMod, sourceURL: "https://steamusercontent.com/ugc/1", want: "steam-workshop"},
		{name: "modrinth", provider: registryv1.UpstreamProviderModrinth, format: "mod", want: "http-release"},
		{name: "papermc", provider: registryv1.UpstreamProviderPaperMC, format: "jar", want: "http-release"},
		{name: "fabricmc", provider: registryv1.UpstreamProviderFabricMC, format: registryv1.FormatServerPackage, want: "fabricmc-bundle"},
		{name: "http-upstream", provider: registryv1.UpstreamProviderHTTP, format: registryv1.FormatServerPackage, want: "http-release"},
		{name: "hosted-https-no-upstream", sourceURL: "https://example.com/mod.jar", want: "http-release"},
		{name: "missing-source-and-upstream", wantErr: true},
	}
	for _, tc := range cases {
		var up *registryv1.Upstream
		if tc.provider != "" {
			up = &registryv1.Upstream{Provider: tc.provider}
		}
		drv, err := LookupSyncDriver(
			&registryv1.Artifact{Metadata: metav1.ObjectMeta{Name: tc.name}, Spec: registryv1.ArtifactSpec{SourceURL: tc.sourceURL}},
			&registryv1.Repository{Spec: registryv1.RepositorySpec{Format: tc.format}},
			up,
		)
		if (err != nil) != tc.wantErr {
			t.Fatalf("%s: err=%v, wantErr=%v", tc.name, err, tc.wantErr)
		}
		if !tc.wantErr && drv.Name() != tc.want {
			t.Fatalf("%s: got driver %q, want %q", tc.name, drv.Name(), tc.want)
		}
	}
}

func TestLookupWorkshopInspectorAndPZ(t *testing.T) {
	if _, ok := LookupWorkshopInspector(999999); ok {
		t.Fatalf("expected no inspector for unregistered Steam AppID")
	}
	insp, ok := LookupWorkshopInspector(addonmirror.SteamAppProjectZomboid)
	if !ok {
		t.Fatalf("expected PZ workshop inspector to self-register")
	}
	root := t.TempDir()
	modDir := filepath.Join(root, "mods", "DemoMod")
	mapDir := filepath.Join(modDir, "media", "maps", "DemoMap")
	if err := os.MkdirAll(mapDir, 0o755); err != nil {
		t.Fatalf("mkdir mapDir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(modDir, "mod.info"), []byte("name=Demo\nid=DemoMod\n"), 0o600); err != nil {
		t.Fatalf("write mod.info: %v", err)
	}
	res := insp.InspectDir(root)
	if !slices.Equal(res.ModIDs, []string{"DemoMod"}) || !slices.Equal(res.MapFolders, []string{"DemoMap"}) {
		t.Fatalf("unexpected PZ inspect result: mods=%v maps=%v", res.ModIDs, res.MapFolders)
	}
}

func TestL4D2VPKWorkshopInspector(t *testing.T) {
	insp, ok := LookupWorkshopInspector(addonmirror.SteamAppLeft4Dead2)
	if !ok {
		t.Fatalf("expected L4D2 workshop inspector to self-register")
	}
	missionTxt := `"mission" { "Name" "UrbanFlight" "modes" { "coop" { "1" { "Map" "uf1_boulevard" } "2" { "Map" "uf2_rooftops" } } "versus" { "1" { "Map" "uf1_boulevard" } } } }`
	splitVPK := buildTestVPKv2(t, []testVPKFileSpec{
		{ext: "txt", dir: "missions", name: "urbanflight", data: []byte(missionTxt)},
		{ext: "bsp", dir: "maps", name: "uf2_rooftops", data: []byte("VBSP")},
	})
	vpkPath := filepath.Join(t.TempDir(), "urbanflight_part2.vpk")
	if err := os.WriteFile(vpkPath, splitVPK, 0o600); err != nil {
		t.Fatal(err)
	}
	res := insp.InspectArchive(registryv1.ArchiveFormatRaw, vpkPath)
	if !slices.Equal(res.MapFolders, []string{"uf2_rooftops"}) {
		t.Fatalf("split VPK should return only bundled BSP maps: got=%v want=[uf2_rooftops]", res.MapFolders)
	}

	fullVPK := buildTestVPKv2(t, []testVPKFileSpec{
		{ext: "txt", dir: "missions", name: "urbanflight", data: []byte(missionTxt)},
		{ext: "bsp", dir: "maps", name: "uf2_rooftops", data: []byte("VBSP")},
		{ext: "bsp", dir: "maps", name: "uf1_boulevard", data: []byte("VBSP")},
	})
	tgzPayload, _ := buildTestTarGzBytes(t, map[string]string{"urbanflight.vpk": string(fullVPK)})
	tgzPath := filepath.Join(t.TempDir(), "workshop-l4d2.tar.gz")
	if err := os.WriteFile(tgzPath, tgzPayload, 0o600); err != nil {
		t.Fatal(err)
	}
	tgzRes := insp.InspectArchive(registryv1.ArchiveFormatTarGz, tgzPath)
	wantFull := []string{"uf1_boulevard", "uf2_rooftops"}
	if !slices.Equal(tgzRes.MapFolders, wantFull) {
		t.Fatalf("unexpected L4D2 tar.gz VPK maps: got=%v want=%v", tgzRes.MapFolders, wantFull)
	}
}

type testVPKFileSpec struct {
	ext, dir, name string
	data           []byte
}

func buildTestVPKv2(t *testing.T, files []testVPKFileSpec) []byte {
	t.Helper()
	var tree, dataSection bytes.Buffer
	for _, f := range files {
		tree.WriteString(f.ext)
		tree.WriteByte(0)
		tree.WriteString(f.dir)
		tree.WriteByte(0)
		tree.WriteString(f.name)
		tree.WriteByte(0)
		var rec [18]byte
		binary.LittleEndian.PutUint16(rec[4:6], 0)
		binary.LittleEndian.PutUint16(rec[6:8], 0x7fff)
		binary.LittleEndian.PutUint32(rec[8:12], uint32(dataSection.Len()))
		binary.LittleEndian.PutUint32(rec[12:16], uint32(len(f.data)))
		binary.LittleEndian.PutUint16(rec[16:18], 0xffff)
		tree.Write(rec[:])
		tree.WriteByte(0)
		tree.WriteByte(0)
		dataSection.Write(f.data)
	}
	tree.WriteByte(0)
	var hdr [28]byte
	binary.LittleEndian.PutUint32(hdr[0:4], 0x55aa1234)
	binary.LittleEndian.PutUint32(hdr[4:8], 2)
	binary.LittleEndian.PutUint32(hdr[8:12], uint32(tree.Len()))
	binary.LittleEndian.PutUint32(hdr[12:16], uint32(dataSection.Len()))
	out := append([]byte{}, hdr[:]...)
	out = append(out, tree.Bytes()...)
	return append(out, dataSection.Bytes()...)
}

type hookCaptureSteamExec struct {
	sawWait     bool
	sawAcquired bool
	sawLine     bool
}

func (h *hookCaptureSteamExec) Run(_ context.Context, _ string, _ []string, hooks SteamHooks) error {
	h.sawWait = hooks.OnWait != nil
	h.sawAcquired = hooks.OnAcquired != nil
	h.sawLine = hooks.OnLine != nil
	if hooks.OnWait != nil {
		hooks.OnWait()
	}
	if hooks.OnAcquired != nil {
		hooks.OnAcquired()
	}
	if hooks.OnLine != nil {
		hooks.OnLine("Update state (0x61) downloading, progress: 50.00 (500 / 1000)")
	}
	return nil
}

func TestExecSteamAttemptDispatchesHooks(t *testing.T) {
	mock := &hookCaptureSteamExec{}
	w := New(&config.Config{}, nil, WithSteamExecutor(mock))
	if w.steamExec == nil {
		t.Fatalf("expected non-nil steamExec")
	}
	err := steam.ExecAttempt(context.Background(), w, nil, 10, "sync", t.TempDir(), []string{"+app_update", "108600"})
	if err != nil {
		t.Fatalf("execSteamAttempt failed: %v", err)
	}
	if !mock.sawWait || !mock.sawAcquired || !mock.sawLine {
		t.Fatalf("expected all SteamHooks callbacks to be passed: %+v", mock)
	}
}

func TestPZSandboxOptionsSchema(t *testing.T) {
	root := t.TempDir()
	sandboxTxt := `VERSION = 1,
option Demo.Rate { type = double, min = 0.1, max = 5.0, default = 1.5, page = DemoPage, translation = Demo_Rate, }
option Demo.Mode { type = enum, numValues = 2, default = 2, valueTranslation = Demo_Mode, translation = Demo_ModeOpt, }
option Demo.Hard { type = boolean, default = true, translation = Demo_Hard, }`
	enTxt := "Sandbox_Demo_Rate = \"Spawn Rate\"\nSandbox_Demo_ModeOpt = \"Mode\"\n"
	cnTxt := "Sandbox_DemoPage = \"演示设置\"\nSandbox_Demo_Rate = \"生成倍率\"\nSandbox_Demo_Rate_tooltip = \"控制倍率\"\nSandbox_Demo_ModeOpt = \"难度模式\"\nSandbox_Demo_Mode_option1 = \"轻松\"\nSandbox_Demo_Mode_option2 = \"困难\"\n"
	writeTestTreeFile(t, root, "mods/Demo/media/sandbox-options.txt", sandboxTxt)
	writeTestTreeFile(t, root, "mods/Demo/media/lua/shared/Translate/EN/Sandbox_EN.txt", enTxt)
	writeTestTreeFile(t, root, "mods/Demo/media/lua/shared/Translate/CN/Sandbox_CN.txt", cnTxt)

	insp, ok := LookupWorkshopInspector(addonmirror.SteamAppProjectZomboid)
	if !ok {
		t.Fatalf("expected PZ workshop inspector")
	}
	res := insp.InspectDir(root)
	if len(res.ConfigSchema) != 3 {
		t.Fatalf("got %d fields, want 3: %+v", len(res.ConfigSchema), res.ConfigSchema)
	}
	rate := res.ConfigSchema[0]
	if rate.Label != "生成倍率" || rate.Description != "控制倍率" || rate.Type != registryv1.AddonConfigTypeNumber ||
		rate.Min == nil || *rate.Min != 0.1 || rate.Max == nil || *rate.Max != 5.0 {
		t.Fatalf("unexpected rate field: %+v", rate)
	}
	mode := res.ConfigSchema[1]
	if mode.Type != registryv1.AddonConfigTypeSelect || len(mode.Options) != 2 || mode.Options[1].Label != "困难" {
		t.Fatalf("unexpected enum field: %+v", mode)
	}
	if res.ConfigSchema[2].Type != registryv1.AddonConfigTypeBoolean || res.ConfigSchema[2].Default != "true" {
		t.Fatalf("unexpected bool field: %+v", res.ConfigSchema[2])
	}
}

func writeTestTreeFile(t *testing.T, root, rel, body string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestInspectCachedWorkshopPayloadExtractsWithoutSteam(t *testing.T) {
	payload, canonDigest := buildTestTarGzBytes(t, map[string]string{
		"2026.8/CalamityMod.tmod": "tmod-payload",
	})
	srv := newCachedBlobTestServer(t, canonDigest, payload)
	defer srv.Close()

	cli, err := client.New(&config.ClientConfig{ServerURL: srv.URL, Token: "test-token", Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}
	w := New(&config.Config{}, cli)
	art := &registryv1.Artifact{
		Metadata: metav1.ObjectMeta{Name: "tmodloader-workshop.2824688072.rev-1"},
		Spec:     registryv1.ArtifactSpec{Repository: "tmodloader-workshop", Package: "2824688072", ArchiveFormat: registryv1.ArchiveFormatTarGz},
		Status: registryv1.ArtifactStatus{
			Build:      "rev-1",
			Descriptor: &registryv1.BlobDescriptor{Digest: canonDigest, FileName: "workshop-2824688072.tar.gz", ArchiveFormat: registryv1.ArchiveFormatTarGz, MediaType: "application/gzip", SizeBytes: int64(len(payload))},
		},
	}

	got := w.InspectCachedWorkshopPayload(context.Background(), nil, art, addonmirror.SteamAppTModLoader, &addonmirror.ResolvedRelease{Package: "2824688072", Build: "rev-1"}, t.TempDir())
	if got == nil || !slices.Equal(got.ModIDs, []string{"CalamityMod"}) || "sha256:"+got.SHA256Hex != canonDigest || got.LocalPath != "" {
		t.Fatalf("unexpected cached inspection payload: %#v", got)
	}
	if changed := w.InspectCachedWorkshopPayload(context.Background(), nil, art, addonmirror.SteamAppTModLoader, &addonmirror.ResolvedRelease{Package: "2824688072", Build: "rev-2"}, t.TempDir()); changed != nil {
		t.Fatalf("expected nil payload when upstream revision changed, got %#v", changed)
	}
}

func buildTestTarGzBytes(t *testing.T, files map[string]string) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	for name, body := range files {
		hdr := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(body))}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(buf.Bytes())
	return buf.Bytes(), "sha256:" + hex.EncodeToString(sum[:])
}

func newCachedBlobTestServer(t *testing.T, digest string, payload []byte) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/apis/registry.gamepanel.io/v1/blobs/"+digest {
			if req.URL.Query().Get("metadata") == "true" {
				rw.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(rw).Encode(registryv1.Blob{Digest: digest, SizeBytes: int64(len(payload)), MediaType: "application/gzip"})
				return
			}
			rw.Header().Set("Content-Type", "application/gzip")
			_, _ = rw.Write(payload)
			return
		}
		rw.WriteHeader(http.StatusNotFound)
	}))
}

type mockArchiveReporter struct {
	stages    []registryv1.ArtifactSyncStage
	transfers int32
}

func (m *mockArchiveReporter) ResetLogs()          {}
func (m *mockArchiveReporter) Logf(string, ...any) {}
func (m *mockArchiveReporter) LogLine(string)      {}
func (m *mockArchiveReporter) Stage(s registryv1.ArtifactSyncStage, _ int32, _ string) {
	m.stages = append(m.stages, s)
}
func (m *mockArchiveReporter) Transfer(_ registryv1.ArtifactSyncStage, pct int32, _, _, _ int64, _ string) {
	if pct >= 72 && pct <= 79 {
		atomic.AddInt32(&m.transfers, 1)
	}
}

func TestArchiveTreeTarGzWithProgressAndCancel(t *testing.T) {
	root := t.TempDir()
	writeTestTreeFile(t, root, "bin/server.bin", strings.Repeat("A", 4096))
	dst := filepath.Join(t.TempDir(), "ok.tar.gz")
	rep := &mockArchiveReporter{}
	size, shaHex, err := driver.ArchiveTreeTarGzWithProgress(context.Background(), root, dst, rep)
	if err != nil || size == 0 || shaHex == "" || atomic.LoadInt32(&rep.transfers) == 0 {
		t.Fatalf("unexpected ArchiveTreeTarGzWithProgress result: size=%d sha=%q err=%v transfers=%d", size, shaHex, err, rep.transfers)
	}

	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel()
	abortedDst := filepath.Join(t.TempDir(), "aborted.tar.gz")
	if _, _, err := driver.ArchiveTreeTarGzWithProgress(cancelCtx, root, abortedDst, rep); err == nil {
		t.Fatal("expected cancelled ArchiveTreeTarGzWithProgress to fail")
	}
	if _, statErr := os.Stat(abortedDst); !os.IsNotExist(statErr) {
		t.Fatalf("expected cancelled partial archive removed, stat err=%v", statErr)
	}
}

func TestListSyncCandidatesAndReleaseClaimOnShutdown(t *testing.T) {
	expired := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	valid := time.Now().UTC().Add(time.Minute).Format(time.RFC3339)
	var updatedArt, updatedStatus atomic.Value
	srv := newWorkerLeaseTestServer(t, expired, valid, &updatedArt, &updatedStatus)
	defer srv.Close()

	cli, err := client.New(&config.ClientConfig{ServerURL: srv.URL, Token: "t", Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	w := New(&config.Config{Registry: config.RegistryConfig{WorkerWorkDir: t.TempDir()}}, cli, WithWorkerID("w-1"))
	cands, err := w.listSyncCandidates(context.Background())
	if err != nil {
		t.Fatalf("listSyncCandidates: %v", err)
	}
	if len(cands) != 2 || cands[0].Metadata.Name != "a.p.1" || cands[1].Metadata.Name != "b.s.1" {
		t.Fatalf("unexpected sync candidates: %+v", cands)
	}
	art := &registryv1.Artifact{
		Metadata: metav1.ObjectMeta{
			Name:        "b.s.1",
			Annotations: map[string]string{registryv1.AnnotationSyncClaimedBy: "w-1", registryv1.AnnotationSyncLeaseUntil: valid},
		},
		Status: registryv1.ArtifactStatus{Phase: registryv1.ArtifactPhaseSyncing},
	}
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	w.releaseClaimOnShutdown(cancelledCtx, newSyncReporter(cancelledCtx, cli, art), art)
	gotArt, _ := updatedArt.Load().(registryv1.Artifact)
	gotSt, _ := updatedStatus.Load().(registryv1.Artifact)
	if gotArt.Metadata.Annotations[registryv1.AnnotationSyncClaimedBy] != "" || gotSt.Status.Phase != registryv1.ArtifactPhasePending {
		t.Fatalf("expected claim cleared and Pending restored: art=%+v st=%+v", gotArt.Metadata.Annotations, gotSt.Status)
	}
}

func newWorkerLeaseTestServer(t *testing.T, expired, valid string, updatedArt, updatedStatus *atomic.Value) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Set("Content-Type", "application/json")
		switch {
		case req.Method == http.MethodGet && req.URL.Path == "/apis/registry.gamepanel.io/v1/artifacts":
			phase := req.URL.Query().Get("fieldSelector")
			var items []registryv1.Artifact
			if strings.Contains(phase, "Pending") {
				items = []registryv1.Artifact{{Metadata: metav1.ObjectMeta{Name: "a.p.1"}, Status: registryv1.ArtifactStatus{Phase: registryv1.ArtifactPhasePending}}}
			} else {
				items = []registryv1.Artifact{
					{Metadata: metav1.ObjectMeta{Name: "b.s.1", Annotations: map[string]string{registryv1.AnnotationSyncLeaseUntil: expired}}, Status: registryv1.ArtifactStatus{Phase: registryv1.ArtifactPhaseSyncing}},
					{Metadata: metav1.ObjectMeta{Name: "c.s.1", Annotations: map[string]string{registryv1.AnnotationSyncLeaseUntil: valid}}, Status: registryv1.ArtifactStatus{Phase: registryv1.ArtifactPhaseSyncing}},
				}
			}
			_ = json.NewEncoder(rw).Encode(metav1.List[registryv1.Artifact]{Items: items})
		case req.Method == http.MethodGet && req.URL.Path == "/apis/registry.gamepanel.io/v1/artifacts/b.s.1":
			_ = json.NewEncoder(rw).Encode(registryv1.Artifact{Metadata: metav1.ObjectMeta{Name: "b.s.1", Annotations: map[string]string{registryv1.AnnotationSyncClaimedBy: "w-1", registryv1.AnnotationSyncLeaseUntil: valid}}, Status: registryv1.ArtifactStatus{Phase: registryv1.ArtifactPhaseSyncing}})
		case req.Method == http.MethodPut && req.URL.Path == "/apis/registry.gamepanel.io/v1/artifacts/b.s.1":
			var body registryv1.Artifact
			_ = json.NewDecoder(req.Body).Decode(&body)
			updatedArt.Store(body)
			_ = json.NewEncoder(rw).Encode(body)
		case req.Method == http.MethodPut && req.URL.Path == "/apis/registry.gamepanel.io/v1/artifacts/b.s.1/status":
			var body registryv1.Artifact
			_ = json.NewDecoder(req.Body).Decode(&body)
			updatedStatus.Store(body)
			_ = json.NewEncoder(rw).Encode(body)
		default:
			rw.WriteHeader(http.StatusOK)
		}
	}))
}

func TestCleanupPayloadStagingDir(t *testing.T) {
	root := t.TempDir()
	single := filepath.Join(root, "steam-servers", "258550", "latest")
	writeTestTreeFile(t, single, "RustDedicated", "bin")
	cleanupPayloadStagingDir(single)
	if _, err := os.Stat(filepath.Dir(single)); !os.IsNotExist(err) {
		t.Fatalf("expected empty parent directory %q removed, stat err=%v", filepath.Dir(single), err)
	}

	branchA := filepath.Join(root, "steam-servers", "294420", "latest")
	branchB := filepath.Join(root, "steam-servers", "294420", "experimental")
	writeTestTreeFile(t, branchA, "7DaysToDieServer", "a")
	writeTestTreeFile(t, branchB, "7DaysToDieServer", "b")
	cleanupPayloadStagingDir(branchA)
	if _, err := os.Stat(branchA); !os.IsNotExist(err) {
		t.Fatalf("expected staging dir %q removed, stat err=%v", branchA, err)
	}
	if _, err := os.Stat(branchB); err != nil {
		t.Fatalf("expected sibling branch %q preserved, got err=%v", branchB, err)
	}
}

type abortingSteamExec struct {
	onStart func(stagingDir string)
	cause   error
}

func (a *abortingSteamExec) Run(ctx context.Context, installDir string, _ []string, _ SteamHooks) error {
	if a.onStart != nil {
		a.onStart(installDir)
	}
	select {
	case <-ctx.Done():
		a.cause = context.Cause(ctx)
		return ctx.Err()
	case <-time.After(3 * time.Second):
		return errors.New("timed out waiting for artifact abort")
	}
}

func TestWorkerAbortsAndCleansStagingWhenArtifactDeletedMidSync(t *testing.T) {
	workDir := t.TempDir()
	var deleted, sawFailed atomic.Bool
	srv := newAbortMidSyncTestServer(t, &deleted, &sawFailed)
	defer srv.Close()

	cli, err := client.New(&config.ClientConfig{ServerURL: srv.URL, Token: "t", Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	exec := &abortingSteamExec{onStart: func(stagingDir string) {
		writeTestTreeFile(t, stagingDir, "game/cs2.bin", "partial-730")
		deleted.Store(true)
	}}
	cfg := &config.Config{Registry: config.RegistryConfig{WorkerWorkDir: workDir, WorkerLeaseTTL: 60 * time.Millisecond, WorkerConcurrency: 1}}
	w := New(cfg, cli, WithWorkerID("w-abort"), WithSteamExecutor(exec))
	if processed, syncErr := w.SyncOnce(context.Background()); syncErr != nil || processed != 0 {
		t.Fatalf("expected SyncOnce aborted cleanly (processed=0, err=nil), got processed=%d err=%v", processed, syncErr)
	}
	if !errors.Is(exec.cause, driver.ErrArtifactAborted) {
		t.Fatalf("expected ErrArtifactAborted context cause, got %v", exec.cause)
	}
	if _, statErr := os.Stat(filepath.Join(workDir, "steam-servers", "730")); !os.IsNotExist(statErr) {
		t.Fatalf("expected staging dir removed on abort, stat err=%v", statErr)
	}
	if sawFailed.Load() {
		t.Fatal("expected deleted artifact not to receive Failed status update")
	}
}

func newAbortMidSyncTestServer(t *testing.T, deleted, sawFailed *atomic.Bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Set("Content-Type", "application/json")
		if handleAbortTestArtifactRoutes(rw, req, deleted, sawFailed) {
			return
		}
		switch req.URL.Path {
		case "/apis/registry.gamepanel.io/v1/repositories/cs2-server":
			_ = json.NewEncoder(rw).Encode(registryv1.Repository{
				Metadata: metav1.ObjectMeta{Name: "cs2-server"},
				Spec:     registryv1.RepositorySpec{Mode: registryv1.RepositoryModeProxy, Format: registryv1.FormatServerPackage, UpstreamName: "steam-cs2-server"},
			})
		case "/apis/registry.gamepanel.io/v1/upstreams/steam-cs2-server":
			_ = json.NewEncoder(rw).Encode(registryv1.Upstream{Metadata: metav1.ObjectMeta{Name: "steam-cs2-server"}, Provider: registryv1.UpstreamProviderSteam, SteamAppID: 730})
		default:
			rw.WriteHeader(http.StatusOK)
		}
	}))
}

func handleAbortTestArtifactRoutes(rw http.ResponseWriter, req *http.Request, deleted, sawFailed *atomic.Bool) bool {
	const artPath = "/apis/registry.gamepanel.io/v1/artifacts/cs2-server.cs2.latest"
	art := registryv1.Artifact{
		Metadata: metav1.ObjectMeta{Name: "cs2-server.cs2.latest"},
		Spec:     registryv1.ArtifactSpec{Repository: "cs2-server", Package: "cs2", Version: "latest"},
		Status:   registryv1.ArtifactStatus{Phase: registryv1.ArtifactPhasePending},
	}
	switch {
	case req.Method == http.MethodGet && req.URL.Path == "/apis/registry.gamepanel.io/v1/artifacts":
		var items []registryv1.Artifact
		if strings.Contains(req.URL.Query().Get("fieldSelector"), string(registryv1.ArtifactPhasePending)) && !deleted.Load() {
			items = []registryv1.Artifact{art}
		}
		_ = json.NewEncoder(rw).Encode(metav1.List[registryv1.Artifact]{Items: items})
		return true
	case strings.HasPrefix(req.URL.Path, artPath):
		return serveAbortTestSingleArtifact(rw, req, artPath, art, deleted, sawFailed)
	default:
		return false
	}
}

func serveAbortTestSingleArtifact(rw http.ResponseWriter, req *http.Request, artPath string, art registryv1.Artifact, deleted, sawFailed *atomic.Bool) bool {
	var body registryv1.Artifact
	isStatusPut := req.Method == http.MethodPut && req.URL.Path == artPath+"/status"
	if req.Method == http.MethodPut && (req.URL.Path == artPath || isStatusPut) {
		_ = json.NewDecoder(req.Body).Decode(&body)
		if isStatusPut && body.Status.Phase == registryv1.ArtifactPhaseFailed {
			sawFailed.Store(true)
		}
	}
	if deleted.Load() {
		rw.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(rw).Encode(map[string]any{"code": http.StatusNotFound, "message": "not found"})
		return true
	}
	if req.Method == http.MethodPut && (req.URL.Path == artPath || isStatusPut) {
		_ = json.NewEncoder(rw).Encode(body)
		return true
	}
	art.Metadata.Annotations = map[string]string{registryv1.AnnotationSyncClaimedBy: "w-abort"}
	_ = json.NewEncoder(rw).Encode(art)
	return true
}
