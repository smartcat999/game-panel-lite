package dst

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"

	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror"
	"github.com/smartcat999/game-panel-cloud/pkg/registryworker/driver"
)

func TestDSTModInfoSchemaParsing(t *testing.T) {
	insp, ok := driver.LookupWorkshopInspector(addonmirror.SteamAppDontStarveTogether)
	if !ok {
		t.Fatalf("expected DST workshop inspector to be registered")
	}
	root := t.TempDir()
	baseLua := `name = "Demo Mod"
local zh = locale == "zh" or locale == "zhr"
configuration_options = {
  { name = "div", label = "Divider", options = {{ description = "", data = "" }}, default = "" },
  { name = "lang", label = zh and "语言" or "Language", hover = "Choose", options = {
    { description = zh and "自动" or "Auto", data = "auto" },
    { description = "EN", data = "en" },
  }, default = "auto" },
  { name = "mult", label = "Rate", options = { { description = "1.5x", data = 1.5 } }, default = 1.5 },
  { name = "show", label = "Show", default = true },
  { name = "step", label = "Step", default = 3 },
}`
	if err := os.WriteFile(filepath.Join(root, "modinfo.lua"), []byte(baseLua), 0o600); err != nil {
		t.Fatal(err)
	}
	chsLua := `name = "Demo Mod"
configuration_options = {
  { name = "lang", label = "界面语言", options = {{ description = "简体中文", data = "zh" }}, default = "zh" },
  { name = "show", label = "显示开关", default = false },
}`
	if err := os.WriteFile(filepath.Join(root, "modinfo_chs.lua"), []byte(chsLua), 0o600); err != nil {
		t.Fatal(err)
	}
	res := insp.InspectDir(root)
	if len(res.ConfigSchema) != 2 {
		t.Fatalf("expected modinfo_chs.lua 2 fields, got %+v", res.ConfigSchema)
	}
	if res.ConfigSchema[0].Label != "界面语言" || res.ConfigSchema[0].Type != registryv1.AddonConfigTypeSelect {
		t.Fatalf("unexpected first field: %+v", res.ConfigSchema[0])
	}
	if res.ConfigSchema[1].Type != registryv1.AddonConfigTypeBoolean || res.ConfigSchema[1].Default != "false" {
		t.Fatalf("unexpected second field: %+v", res.ConfigSchema[1])
	}
}

func TestDSTModInfoTernaryFallbackAndNestedZip(t *testing.T) {
	src := `configuration_options = {
  { name = "mode", label = (zh and "模式" or "Mode"), options = {
    { description = zh and "标准" or "Normal", data = 1 },
  }, default = 1 },
}`
	fields := parseDSTModInfoSchema(src)
	if len(fields) != 1 || fields[0].Label != "模式" || fields[0].Options[0].ValueType != registryv1.AddonConfigValueNumber {
		t.Fatalf("unexpected ternary parse: %+v", fields)
	}

	root := t.TempDir()
	zipPath := filepath.Join(root, "mod_legacy.bin")
	createTestZipFile(t, zipPath, map[string]string{"modinfo.lua": src})
	res := workshopInspector{}.InspectDir(root)
	if len(res.ConfigSchema) != 1 || res.ConfigSchema[0].Name != "mode" {
		t.Fatalf("expected InspectDir to inspect nested zip, got %+v", res.ConfigSchema)
	}
	arcRes := workshopInspector{}.InspectArchive(registryv1.ArchiveFormatZip, zipPath)
	if len(arcRes.ConfigSchema) != 1 || arcRes.ConfigSchema[0].Name != "mode" {
		t.Fatalf("expected InspectArchive result, got %+v", arcRes.ConfigSchema)
	}
	tarGzPath := filepath.Join(root, "workshop.tar.gz")
	wrapFileInTarGz(t, tarGzPath, "123_legacy.bin", zipPath)
	nestedTarRes := workshopInspector{}.InspectArchive(registryv1.ArchiveFormatTarGz, tarGzPath)
	if len(nestedTarRes.ConfigSchema) != 1 || nestedTarRes.ConfigSchema[0].Name != "mode" {
		t.Fatalf("expected nested legacy zip inside tar.gz result, got %+v", nestedTarRes.ConfigSchema)
	}
}

func wrapFileInTarGz(t *testing.T, tarGzPath, entryName, srcPath string) {
	t.Helper()
	raw, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(tarGzPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)
	if err := tw.WriteHeader(&tar.Header{Name: entryName, Mode: 0o644, Size: int64(len(raw))}); err != nil {
		t.Fatal(err)
	}
	if _, err := io.Writer(tw).Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}
}

func createTestZipFile(t *testing.T, zipPath string, files map[string]string) {
	t.Helper()
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	zw := zip.NewWriter(f)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}
