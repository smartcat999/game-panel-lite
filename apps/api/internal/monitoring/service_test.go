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

func TestHealthOverallIncludesResourceAlerts(t *testing.T) {
	ds := DataSource{Connected: true}
	if got := healthOverall(ds, "healthy", 0, 0, nil); got != "healthy" {
		t.Fatalf("expected healthy, got %q", got)
	}
	if got := healthOverall(ds, "healthy", 0, 0, []ResourceAlert{{Severity: "warning"}}); got != "warning" {
		t.Fatalf("expected warning, got %q", got)
	}
	if got := healthOverall(ds, "healthy", 0, 0, []ResourceAlert{{Severity: "critical"}}); got != "critical" {
		t.Fatalf("expected critical, got %q", got)
	}
}

func TestNodeMemoryPercentQuery(t *testing.T) {
	want := `100 * (1 - (sum(node_memory_MemAvailable_bytes) / sum(node_memory_MemTotal_bytes)))`
	if got := nodeMemoryPercentQuery(); got != want {
		t.Fatalf("unexpected node memory percent query %q", got)
	}
}
