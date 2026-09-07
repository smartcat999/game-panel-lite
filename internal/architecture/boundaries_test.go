package architecture

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

const api = "github.com/smartcat999/game-panel-lite/apps/api/internal/"

// These file/import pairs are the existing game-specific integrations scheduled
// for M2 (game integration) and M3 (Agent runtime) in docs/architecture/backend-modularity-plan.md. New exceptions require
// an explicit review; stale exceptions fail so the baseline can only shrink.
var legacyImports = map[string][]string{
	"apps/api/internal/http/terraria_handlers.go":       {api + "provider/terraria"},
	"apps/api/internal/http/world_handlers.go":          {api + "provider/terraria"},
	"apps/api/internal/http/backup_handlers.go":         {api + "provider/terraria"},
	"apps/api/internal/http/types.go":                   {api + "provider/terraria"},
	"apps/api/internal/http/provider_config_helpers.go": {api + "provider/terraria"},
	"apps/api/internal/http/mod_helpers.go":             {api + "provider/terraria"},
	"apps/api/internal/server/mod_planner.go":           {api + "provider/terraria"},
	"apps/agent/reconcile.go":                           {"github.com/docker/docker/api/types", "github.com/docker/docker/client"},
}

func TestBackendImportBoundaries(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate repository")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
	seen := map[string]bool{}
	err := filepath.WalkDir(filepath.Join(root, "apps"), func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == "node_modules" || entry.Name() == ".next" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, spec := range parsed.Imports {
			imported, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			reason := forbiddenImport(relative, imported)
			if reason == "" {
				continue
			}
			excepted := false
			for _, allowed := range legacyImports[relative] {
				if imported == allowed {
					seen[relative+" -> "+imported] = true
					excepted = true
					break
				}
			}
			if excepted {
				continue
			}
			t.Errorf("%s imports %s: %s", relative, imported, reason)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for file, imports := range legacyImports {
		for _, imported := range imports {
			key := file + " -> " + imported
			if !seen[key] {
				t.Errorf("remove stale import exception: %s", key)
			}
		}
	}
}

func forbiddenImport(file, imported string) string {
	under := func(dir string) bool { return strings.HasPrefix(file, "apps/api/internal/"+dir+"/") }
	if strings.HasPrefix(imported, "github.com/docker/") && !under("runtime/docker") {
		return "Docker SDK belongs to the Docker RuntimeAdapter"
	}
	if strings.HasPrefix(imported, "gorm.io/") && !under("store") {
		return "GORM belongs to persistence adapters"
	}
	if under("domain") && (strings.HasPrefix(imported, api) || imported == "net/http" || strings.HasPrefix(imported, "github.com/go-chi/")) {
		return "domain must not depend on transport, services or infrastructure"
	}
	if strings.HasPrefix(imported, api+"provider/") && imported != api+"provider/runtimecatalog" &&
		!under("provider") && !under("app") {
		return "concrete game providers belong in provider implementations or the composition root"
	}
	if under("runtime") && (strings.HasPrefix(imported, api+"provider") || strings.HasPrefix(imported, api+"http")) {
		return "runtime must not depend on providers or HTTP handlers"
	}
	return ""
}

func TestImportRules(t *testing.T) {
	for _, tc := range []struct {
		file, imported string
		forbidden      bool
	}{
		{"apps/api/internal/http/new.go", api + "provider/terraria", true},
		{"apps/api/internal/http/world_handlers.go", api + "provider/minecraft", true},
		{"apps/api/internal/app/app.go", api + "provider/terraria", false},
		{"apps/api/internal/runtime/docker/adapter.go", "github.com/docker/docker/client", false},
		{"apps/api/internal/server/new.go", "github.com/docker/docker/client", true},
		{"apps/api/internal/domain/new.go", "net/http", true},
		{"apps/api/internal/domain/new.go", api + "store", true},
		{"apps/api/internal/server/new.go", "gorm.io/gorm", true},
		{"apps/api/internal/runtime/new.go", api + "provider", true},
	} {
		if got := forbiddenImport(tc.file, tc.imported) != ""; got != tc.forbidden {
			t.Errorf("%s -> %s: forbidden=%v, want %v", tc.file, tc.imported, got, tc.forbidden)
		}
	}
}
