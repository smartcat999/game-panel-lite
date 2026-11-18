package agent

import (
	"context"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
	gnet "github.com/shirou/gopsutil/v4/net"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/config"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/log"
)

const (
	// MinRecommendedMaxMapCount is the minimum vm.max_map_count recommended by LinuxGSM for UE5/Unity game servers.
	MinRecommendedMaxMapCount int64 = 262144
	defaultMaxMapCountPath          = "/proc/sys/vm/max_map_count"
)

// ResourceCollector collects dynamic compute host resources (CPU, Memory, Storage).
type ResourceCollector interface {
	Collect(ctx context.Context) (capacity corev1.ResourceList, allocatable corev1.ResourceList, err error)
}

// UsageCollector is an optional extension of ResourceCollector that collects real-time physical host resource usage.
type UsageCollector interface {
	CollectUsage(ctx context.Context) (*corev1.NodeUsage, error)
}

// HostResourceCollector collects actual host resources using gopsutil v4.
type HostResourceCollector struct {
	workDir    string
	reserved   config.AgentResourceReservation
	netMu      sync.Mutex
	prevRx     uint64
	prevTx     uint64
	prevNetAt  time.Time
	lastRxRate int64
	lastTxRate int64
}

// NewHostResourceCollector creates a new HostResourceCollector targeting the specified workDir.
func NewHostResourceCollector(workDir string, reservation ...config.AgentResourceReservation) *HostResourceCollector {
	if workDir == "" {
		workDir = "/tmp/gamepanel/instances"
	}
	reserved := config.DefaultAgentResourceReservation()
	if len(reservation) > 0 {
		reserved = reservation[0]
	}
	return &HostResourceCollector{workDir: workDir, reserved: reserved}
}

// Collect queries host CPU, memory, and disk capacity and allocatable values.
func (c *HostResourceCollector) Collect(ctx context.Context) (corev1.ResourceList, corev1.ResourceList, error) {
	// 1. CPU cores
	cores, err := cpu.CountsWithContext(ctx, true)
	if err != nil {
		return nil, nil, fmt.Errorf("collect cpu cores: %w", err)
	}
	if cores <= 0 {
		return nil, nil, fmt.Errorf("invalid cpu core count: %d", cores)
	}

	// 2. Memory
	vm, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("collect virtual memory: %w", err)
	}
	if vm == nil {
		return nil, nil, fmt.Errorf("virtual memory statistics are missing")
	}

	// 3. Disk (ensure workDir exists before querying disk usage)
	if mkdirErr := os.MkdirAll(c.workDir, 0755); mkdirErr != nil {
		return nil, nil, fmt.Errorf("create workDir %q: %w", c.workDir, mkdirErr)
	}
	diskStat, err := disk.UsageWithContext(ctx, c.workDir)
	if err != nil {
		return nil, nil, fmt.Errorf("collect disk usage for %q: %w", c.workDir, err)
	}
	if diskStat == nil {
		return nil, nil, fmt.Errorf("disk statistics are missing")
	}
	return hostResourceBudget(int64(cores), vm.Total, diskStat.Total, c.reserved)
}

// hostResourceBudget computes the admission ceiling from physical totals. Live
// free space is usage, already covered by allocations, and is not subtracted a
// second time. Every emitted quantity uses exact integer millicores or bytes.
func hostResourceBudget(cores int64, memory, storage uint64, reserved config.AgentResourceReservation) (corev1.ResourceList, corev1.ResourceList, error) {
	if err := reserved.Validate(); err != nil {
		return nil, nil, err
	}
	if cores <= 0 || cores > math.MaxInt64/1000 {
		return nil, nil, fmt.Errorf("CPU core count exceeds supported accounting range: %d", cores)
	}
	if memory == 0 || storage == 0 || memory > math.MaxInt64 || storage > math.MaxInt64 {
		return nil, nil, fmt.Errorf("host memory/storage totals must be positive signed 64-bit bytes")
	}
	cpu := cores * 1000
	capacity := exactHostResources(cpu, int64(memory), int64(storage))
	remaining := func(total, overhead int64) int64 {
		if overhead >= total {
			return 0
		}
		return total - overhead
	}
	allocatable := exactHostResources(remaining(cpu, reserved.CPUMillicores), remaining(int64(memory), reserved.MemoryBytes), remaining(int64(storage), reserved.StorageBytes))
	return capacity, allocatable, nil
}

func exactHostResources(cpu, memory, storage int64) corev1.ResourceList {
	return corev1.ResourceList{
		corev1.ResourceCPU:     corev1.Quantity(strconv.FormatInt(cpu, 10) + "m"),
		corev1.ResourceMemory:  corev1.Quantity(strconv.FormatInt(memory, 10)),
		corev1.ResourceStorage: corev1.Quantity(strconv.FormatInt(storage, 10)),
	}
}

// CollectUsage queries live physical CPU, memory, and disk usage on the host.
func (c *HostResourceCollector) CollectUsage(ctx context.Context) (*corev1.NodeUsage, error) {
	var cpuMillicores int64
	if cores, err := cpu.CountsWithContext(ctx, true); err == nil && cores > 0 {
		if pcts, pErr := cpu.PercentWithContext(ctx, 0, false); pErr == nil && len(pcts) > 0 {
			cpuMillicores = int64(math.Round((pcts[0] / 100.0) * float64(cores) * 1000))
		}
	}

	var memUsed int64
	if vm, err := mem.VirtualMemoryWithContext(ctx); err == nil && vm != nil {
		memUsed = safeInt64(vm.Used)
	}

	var diskUsed int64
	_ = os.MkdirAll(c.workDir, 0755)
	if diskStat, err := disk.UsageWithContext(ctx, c.workDir); err == nil && diskStat != nil {
		diskUsed = safeInt64(diskStat.Used)
	}

	rxRate, txRate := c.collectNetworkRates(ctx)

	return &corev1.NodeUsage{
		CPUMillicores:        cpuMillicores,
		MemoryBytes:          memUsed,
		StorageBytes:         diskUsed,
		NetworkRxBytesPerSec: rxRate,
		NetworkTxBytesPerSec: txRate,
	}, nil
}

func (c *HostResourceCollector) collectNetworkRates(ctx context.Context) (int64, int64) {
	totalRx, totalTx, ok := readHostNetTotals(ctx)
	if !ok {
		return 0, 0
	}
	return c.updateNetworkRates(totalRx, totalTx, time.Now())
}

func (c *HostResourceCollector) updateNetworkRates(totalRx, totalTx uint64, now time.Time) (int64, int64) {
	c.netMu.Lock()
	defer c.netMu.Unlock()
	if c.prevNetAt.IsZero() {
		c.prevRx, c.prevTx, c.prevNetAt = totalRx, totalTx, now
		return 0, 0
	}
	elapsed := now.Sub(c.prevNetAt).Seconds()
	if elapsed < 0.2 {
		return c.lastRxRate, c.lastTxRate
	}
	var rxRate, txRate int64
	if totalRx >= c.prevRx {
		rxRate = int64(float64(totalRx-c.prevRx) / elapsed)
	}
	if totalTx >= c.prevTx {
		txRate = int64(float64(totalTx-c.prevTx) / elapsed)
	}
	c.prevRx, c.prevTx, c.prevNetAt = totalRx, totalTx, now
	c.lastRxRate, c.lastTxRate = rxRate, txRate
	return rxRate, txRate
}

func readHostNetTotals(ctx context.Context) (uint64, uint64, bool) {
	if stats, ok := readProcNetDev("/proc/1/net/dev"); ok {
		if rx, tx, valid := aggregateHostNetTotals(stats); valid {
			return rx, tx, true
		}
	}
	counters, err := gnet.IOCountersWithContext(ctx, true)
	if err != nil {
		return 0, 0, false
	}
	return aggregateHostNetTotals(counters)
}

func readProcNetDev(path string) ([]gnet.IOCountersStat, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var out []gnet.IOCountersStat
	for _, line := range strings.Split(string(raw), "\n") {
		colon := strings.IndexByte(line, ':')
		if colon <= 0 {
			continue
		}
		name := strings.TrimSpace(line[:colon])
		fields := strings.Fields(line[colon+1:])
		if name == "" || len(fields) < 9 {
			continue
		}
		rx, rxErr := strconv.ParseUint(fields[0], 10, 64)
		tx, txErr := strconv.ParseUint(fields[8], 10, 64)
		if rxErr != nil || txErr != nil {
			continue
		}
		out = append(out, gnet.IOCountersStat{Name: name, BytesRecv: rx, BytesSent: tx})
	}
	return out, len(out) > 0
}

func aggregateHostNetTotals(counters []gnet.IOCountersStat) (uint64, uint64, bool) {
	var physRx, physTx, fallbackRx, fallbackTx uint64
	var anyFallback bool
	for _, io := range counters {
		if isVirtualOrLoopbackIface(io.Name) {
			continue
		}
		anyFallback = true
		fallbackRx += io.BytesRecv
		fallbackTx += io.BytesSent
		if isPhysicalHostIface(io.Name) {
			physRx += io.BytesRecv
			physTx += io.BytesSent
		}
	}
	if physRx > 0 || physTx > 0 {
		return physRx, physTx, true
	}
	return fallbackRx, fallbackTx, anyFallback
}

func isPhysicalHostIface(name string) bool {
	if isVirtualOrLoopbackIface(name) {
		return false
	}
	n := strings.ToLower(strings.TrimSpace(name))
	for _, prefix := range []string{"eth", "en", "em", "bond", "team", "wl", "ib"} {
		if strings.HasPrefix(n, prefix) {
			return true
		}
	}
	return false
}

func isVirtualOrLoopbackIface(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" || n == "lo" || strings.HasPrefix(n, "lo") {
		return true
	}
	for _, prefix := range []string{
		"veth", "docker", "br-", "cni", "flannel", "cali", "virbr",
		"tun", "tap", "kube-", "tailscale", "wg", "utun", "meta", "clash", "mihomo", "sit", "vxlan",
	} {
		if strings.HasPrefix(n, prefix) {
			return true
		}
	}
	for _, sub := range []string{"tap", "tun", "veth", "bridge", "dummy"} {
		if strings.Contains(n, sub) {
			return true
		}
	}
	return false
}

func safeInt64(v uint64) int64 {
	if v > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(v)
}

func readKernelMaxMapCount(path string) (int64, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	val, err := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64)
	if err != nil || val < 0 {
		return 0, false
	}
	return val, true
}

func warnIfLowKernelMaxMapCount(path string) {
	val, ok := readKernelMaxMapCount(path)
	if !ok {
		return
	}
	if val < MinRecommendedMaxMapCount {
		log.Warn("host kernel vm.max_map_count below recommended minimum for UE5/Unity game servers; consider running: sysctl -w vm.max_map_count=262144",
			"current", val, "recommended", MinRecommendedMaxMapCount)
	}
}
