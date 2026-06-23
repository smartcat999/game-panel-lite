package monitoring

import "testing"

func TestManagedContainerQueriesUseGamePanelServerMetrics(t *testing.T) {
	if got := managedContainersCPUQuery(); got != `sum(gamepanel_server_cpu_percent{status="running"})` {
		t.Fatalf("unexpected managed CPU query %q", got)
	}
	if got := managedContainersMemoryQuery(); got != `sum(gamepanel_server_memory_bytes{status="running"}) / 1024 / 1024` {
		t.Fatalf("unexpected managed memory query %q", got)
	}
}
