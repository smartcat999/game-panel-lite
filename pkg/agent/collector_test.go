package agent

import (
	"os"
	"path/filepath"
	"testing"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/config"
)

func TestReadKernelMaxMapCount(t *testing.T) {
	dir := t.TempDir()
	missingPath := filepath.Join(dir, "missing")
	if _, ok := readKernelMaxMapCount(missingPath); ok {
		t.Fatalf("expected missing path to return ok=false")
	}

	okPath := filepath.Join(dir, "max_map_count_ok")
	if err := os.WriteFile(okPath, []byte("262144\n"), 0644); err != nil {
		t.Fatalf("write ok file: %v", err)
	}
	if got, ok := readKernelMaxMapCount(okPath); !ok || got != 262144 {
		t.Fatalf("expected (262144, true), got (%d, %v)", got, ok)
	}

	lowPath := filepath.Join(dir, "max_map_count_low")
	if err := os.WriteFile(lowPath, []byte("65530\n"), 0644); err != nil {
		t.Fatalf("write low file: %v", err)
	}
	if got, ok := readKernelMaxMapCount(lowPath); !ok || got != 65530 || got >= MinRecommendedMaxMapCount {
		t.Fatalf("expected (65530, true) below MinRecommendedMaxMapCount, got (%d, %v)", got, ok)
	}
	warnIfLowKernelMaxMapCount(lowPath)
}

func TestHostResourceBudgetWithReservedQuantities(t *testing.T) {
	cfg := config.New()
	cfg.Agent.ReservedCPU = "2"
	cfg.Agent.ReservedMemory = "8Gi"
	cfg.Default()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("validate config: %v", err)
	}
	capRes, allocRes, err := hostResourceBudget(16, 30<<30, 100<<30, *cfg.Agent.ReservedResources)
	if err != nil {
		t.Fatalf("hostResourceBudget: %v", err)
	}
	if capRes[corev1.ResourceCPU] != "16000m" || allocRes[corev1.ResourceCPU] != "14000m" {
		t.Fatalf("unexpected cpu budget: cap=%q alloc=%q", capRes[corev1.ResourceCPU], allocRes[corev1.ResourceCPU])
	}
	if capRes.MemoryBytes() != 30<<30 || allocRes.MemoryBytes() != 22<<30 {
		t.Fatalf("unexpected memory budget: cap=%d alloc=%d", capRes.MemoryBytes(), allocRes.MemoryBytes())
	}
}
