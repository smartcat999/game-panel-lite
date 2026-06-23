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

func TestNodeDiskQueryUsesRootFilesystem(t *testing.T) {
	got := nodeDiskQuery()
	want := `max(100 * (1 - (node_filesystem_avail_bytes{mountpoint="/",fstype!~"tmpfs|overlay|squashfs|aufs|fuse.*"} / node_filesystem_size_bytes{mountpoint="/",fstype!~"tmpfs|overlay|squashfs|aufs|fuse.*"})))`
	if got != want {
		t.Fatalf("unexpected node disk query %q", got)
	}
}
