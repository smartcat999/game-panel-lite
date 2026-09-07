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

func TestBackendImportBoundaries(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate repository")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != root && filepath.Dir(path) == root && entry.Name() != "apps" && entry.Name() != "internal" {
				return filepath.SkipDir
			}
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
			t.Errorf("%s imports %s: %s", relative, imported, reason)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

}

func forbiddenImport(file, imported string) string {
	under := func(dir string) bool { return strings.HasPrefix(file, "apps/api/internal/"+dir+"/") }
	if strings.HasPrefix(file, "internal/workload/") && (strings.Contains(imported, ".") || imported == "net/http") {
		return "shared workload protocol must be infrastructure independent"
	}
	if strings.HasPrefix(file, "internal/worker/") && (strings.HasPrefix(imported, api) || strings.Contains(imported, "/internal/runtime/") || imported == "net/http") {
		return "worker coordinates runtime interfaces without transport or adapter dependencies"
	}
	if strings.HasPrefix(imported, "github.com/smartcat999/game-panel-lite/internal/runtime/") && !under("app") && !under("runtime/docker") && file != "apps/agent/main.go" && !strings.HasPrefix(file, "internal/runtime/") {
		return "concrete shared runtime belongs in an adapter or composition root"
	}
	if strings.HasPrefix(file, "internal/runtime/") && strings.HasPrefix(imported, api) {
		return "shared runtimes cannot depend on control-plane internals"
	}

	if strings.HasPrefix(imported, "github.com/docker/") && !under("runtime/docker") && !strings.HasPrefix(file, "internal/runtime/docker/") {
		return "Docker SDK belongs to the Docker RuntimeAdapter"
	}
	if strings.HasPrefix(imported, "gorm.io/") && !under("store") {
		return "GORM belongs to persistence adapters"
	}
	if (under("modcatalog") || under("modruntime")) && (imported == "net/http" || strings.HasPrefix(imported, api+"http") || strings.HasPrefix(imported, api+"server") || strings.HasPrefix(imported, api+"store") || strings.HasPrefix(imported, api+"runtime")) {
		return "shared mod rules must not depend on transport, lifecycle or concrete persistence/runtime"
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
		{"apps/api/internal/modcatalog/metadata.go", api + "store", true},
		{"apps/api/internal/modruntime/dependencies.go", api + "server", true},
		{"apps/api/internal/modruntime/dependencies.go", api + "modcatalog", false},
		{"apps/api/internal/modcatalog/metadata.go", api + "domain", false},
		{"apps/api/internal/http/new.go", api + "provider/terraria", true},
		{"apps/agent/main.go", "github.com/smartcat999/game-panel-lite/internal/runtime/docker", false},
		{"apps/agent/reconcile.go", "github.com/docker/docker/client", true},
		{"internal/runtime/docker/adapter.go", "github.com/docker/docker/client", false},
		{"internal/runtime/docker/adapter.go", api + "store", true},
		{"internal/worker/reconciler.go", "github.com/smartcat999/game-panel-lite/internal/runtime/docker", true},
		{"internal/workload/protocol.go", "net/http", true},
		{"apps/api/internal/http/new.go", "github.com/smartcat999/game-panel-lite/internal/runtime/docker", true},
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
