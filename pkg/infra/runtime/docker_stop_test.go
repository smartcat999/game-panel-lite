package runtime

import (
	"testing"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
)

func TestBuildDockerHostConfigUlimitsAndPids(t *testing.T) {
	resList := corev1.ResourceList{
		corev1.ResourceCPU:     "1",
		corev1.ResourceMemory:  "1Gi",
		corev1.ResourceStorage: "5Gi",
	}
	in := &corev1.Instance{
		Spec: corev1.InstanceSpec{
			Image:     "gamepanel/runtime:latest",
			Resources: corev1.ResourceRequirements{Limits: resList},
		},
		Status: corev1.InstanceStatus{
			ResourceControl: &corev1.InstanceResourceControl{ApprovedResources: resList},
		},
	}
	hostCfg, err := buildDockerHostConfig("/tmp/host", "/data", nil, in, nil)
	if err != nil {
		t.Fatalf("buildDockerHostConfig default: %v", err)
	}
	if hostCfg.PidsLimit == nil || *hostCfg.PidsLimit != defaultDockerPidsLimit {
		t.Fatalf("expected PidsLimit=%d, got %v", defaultDockerPidsLimit, hostCfg.PidsLimit)
	}
	if len(hostCfg.Ulimits) != 1 || hostCfg.Ulimits[0].Name != "nofile" ||
		hostCfg.Ulimits[0].Soft != defaultDockerNofileLimit || hostCfg.Ulimits[0].Hard != defaultDockerNofileLimit {
		t.Fatalf("expected default nofile ulimit %d, got %+v", defaultDockerNofileLimit, hostCfg.Ulimits)
	}

	customRC := &corev1.RuntimeClass{Handler: HandlerDocker, Parameters: map[string]string{"nofileLimit": "262144"}}
	customCfg, err := buildDockerHostConfig("/tmp/host", "/data", nil, in, customRC)
	if err != nil {
		t.Fatalf("buildDockerHostConfig custom nofileLimit: %v", err)
	}
	if len(customCfg.Ulimits) != 1 || customCfg.Ulimits[0].Soft != 262144 || customCfg.Ulimits[0].Hard != 262144 {
		t.Fatalf("expected custom nofile ulimit 262144, got %+v", customCfg.Ulimits)
	}

	drv := NewDockerDriver("")
	badRC := &corev1.RuntimeClass{Handler: HandlerDocker, Parameters: map[string]string{"nofileLimit": "0"}}
	if err := drv.Validate(in, badRC); err == nil {
		t.Fatalf("expected Validate error for non-positive nofileLimit")
	}
}

func TestResolvePreStopHook(t *testing.T) {
	if resolvePreStopHook(nil) != "" {
		t.Fatalf("expected empty hook for nil instance")
	}
	empty := &corev1.Instance{
		Metadata: metav1.ObjectMeta{Labels: map[string]string{corev1.LabelInstanceApp: corev1.AppPalworldDedicated}},
	}
	if got := resolvePreStopHook(empty); got != "" {
		t.Fatalf("expected empty hook without %s env, got %q", corev1.EnvPreStopHook, got)
	}
	configured := &corev1.Instance{
		Metadata: metav1.ObjectMeta{Labels: map[string]string{corev1.LabelInstanceApp: corev1.AppPalworldDedicated}},
		Spec:     corev1.InstanceSpec{Env: map[string]string{corev1.EnvPreStopHook: "echo custom-stop"}},
	}
	if got := resolvePreStopHook(configured); got != "echo custom-stop" {
		t.Fatalf("expected %s env hook, got %q", corev1.EnvPreStopHook, got)
	}
}

func TestPrimaryInstancePortAndReadiness(t *testing.T) {
	ports := []corev1.InstancePort{
		{Name: "game", Protocol: corev1.ProtocolUDP, TargetPort: 2456},
		{Name: "query", Protocol: corev1.ProtocolUDP, TargetPort: 2457},
	}
	primary, ok := primaryInstancePort(ports)
	if !ok || primary.Name != "game" || primary.TargetPort != 2456 {
		t.Fatalf("expected primary game port 2456, got ok=%v port=%+v", ok, primary)
	}
	if _, ok := primaryInstancePort(nil); ok {
		t.Fatalf("expected no primary port for empty slice")
	}
	noHook := &corev1.Instance{Spec: corev1.InstanceSpec{Ports: ports}}
	if !isContainerGamePortReady(noHook, 123, time.Now()) {
		t.Fatalf("expected container without %s to be immediately ready", corev1.EnvPreStopHook)
	}
	timedOut := &corev1.Instance{
		Spec: corev1.InstanceSpec{
			Env:   map[string]string{corev1.EnvPreStopHook: "save"},
			Ports: ports,
		},
	}
	if !isContainerGamePortReady(timedOut, 123, time.Now().Add(-2*time.Minute)) {
		t.Fatalf("expected container exceeding 90s warmup window to be ready")
	}
}

func TestCanonicalPortsPreservesPrimaryOrder(t *testing.T) {
	canonical, err := corev1.CanonicalPorts([]corev1.PortRequest{
		{Name: "game", Protocol: "udp", TargetPort: 28015},
		{Name: "query", Protocol: "udp", TargetPort: 28016},
		{Name: "rcon", Protocol: "tcp", TargetPort: 28016},
		{Name: "app", Protocol: "tcp", TargetPort: 28082},
	})
	if err != nil {
		t.Fatalf("CanonicalPorts: %v", err)
	}
	ports := make([]corev1.InstancePort, len(canonical))
	for i, p := range canonical {
		ports[i] = corev1.InstancePort{Name: p.Name, Protocol: p.Protocol, TargetPort: int(p.TargetPort), PublishedPort: int(p.PublishedPort)}
	}
	primary, ok := primaryInstancePort(ports)
	if !ok || primary.Name != "game" || primary.Protocol != corev1.ProtocolUDP || primary.TargetPort != 28015 {
		t.Fatalf("expected primary UDP port preserved at index 0, got ok=%v port=%+v (all=%+v)", ok, primary, ports)
	}
}

func TestHasBoundPortInProcNet(t *testing.T) {
	tcpTable := "  sl  local_address rem_address   st\n" +
		"   0: 00000000:63DD 00000000:0000 0A 00000000:00000000 00:00000000 00000000 1000 0 1 1\n" +
		"   1: 0100007F:63DE 0100007F:9000 01 00000000:00000000 00:00000000 00000000 1000 0 2 1\n"
	if !hasBoundPortInProcNet(tcpTable, 25565, corev1.ProtocolTCP) {
		t.Fatalf("expected TCP port 25565 (0x63DD) in state 0A to match")
	}
	if hasBoundPortInProcNet(tcpTable, 25566, corev1.ProtocolTCP) {
		t.Fatalf("expected non-LISTEN TCP state 01 on port 25566 not to count as ready")
	}
	udpTable := "  sl  local_address rem_address   st\n" +
		"6944: 00000000000000000000000000000000:2013 00000000000000000000000000000000:0000 07 00000000:00000000 00:00000000 00000000 1000 0 1 2\n"
	if !hasBoundPortInProcNet(udpTable, 8211, corev1.ProtocolUDP) {
		t.Fatalf("expected IPv6 UDP port 8211 (0x2013) in state 07 to match")
	}
	if hasBoundPortInProcNet(udpTable, 8212, corev1.ProtocolUDP) {
		t.Fatalf("expected unbound UDP port 8212 not to match")
	}
	if !hasCandidatePortInProcNet(udpTable, 8210, corev1.ProtocolUDP) {
		t.Fatalf("expected UDP query port fallback targetPort+1 (8211) to satisfy readiness")
	}
	if hasCandidatePortInProcNet(tcpTable, 25564, corev1.ProtocolTCP) {
		t.Fatalf("expected TCP readiness not to accept targetPort+1")
	}
}
