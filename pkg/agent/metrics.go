package agent

import (
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	dockerclient "github.com/docker/docker/client"
	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/runtime"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/volume"
)

const maxInstanceMetricWalkFiles = 100000

type dockerCPUSample struct {
	totalUsage  uint64
	systemUsage uint64
	rxBytes     uint64
	txBytes     uint64
	sampledAt   time.Time
	rxRate      int64
	txRate      int64
}

type diskUsageCacheEntry struct {
	bytes     int64
	updatedAt time.Time
}

type fileLinesCacheEntry struct {
	modTimeNano int64
	size        int64
	lines       []string
}

var (
	dockerCliOnce      sync.Once
	dockerCliInst      *dockerclient.Client
	dockerCPUMu        sync.Mutex
	dockerCPUMap       = make(map[string]dockerCPUSample)
	diskUsageCacheMu   sync.RWMutex
	diskUsageCacheMap  = make(map[string]diskUsageCacheEntry)
	fileLinesCacheMu   sync.RWMutex
	fileLinesCacheMap  = make(map[string]fileLinesCacheEntry)
	regexCacheMu       sync.RWMutex
	compiledRegexCache = make(map[string]*regexp.Regexp)
	customProbeMu      sync.Mutex
	lastProbeAtMap     = make(map[string]time.Time)
	lastProbeLinesMap  = make(map[string][]string)
	ansiEscapeRegex    = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)
)

func getCompiledRegex(pattern string, caseInsensitive bool) *regexp.Regexp {
	if pattern == "" {
		return nil
	}
	expr := pattern
	if caseInsensitive {
		expr = "(?i)" + pattern
	}
	regexCacheMu.RLock()
	if re, ok := compiledRegexCache[expr]; ok {
		regexCacheMu.RUnlock()
		return re
	}
	regexCacheMu.RUnlock()

	re, err := regexp.Compile(expr)
	if err != nil {
		return nil
	}
	regexCacheMu.Lock()
	if len(compiledRegexCache) >= 1024 {
		compiledRegexCache = make(map[string]*regexp.Regexp, 64)
	}
	compiledRegexCache[expr] = re
	regexCacheMu.Unlock()
	return re
}

func getDockerStatsClient() *dockerclient.Client {
	dockerCliOnce.Do(func() {
		cli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
		if err == nil {
			dockerCliInst = cli
		}
	})
	return dockerCliInst
}

func collectDockerContainerStats(containerID string) (cpuPercent float64, memBytes uint64, rxRate, txRate int64) {
	cli := getDockerStatsClient()
	if cli == nil || strings.TrimSpace(containerID) == "" {
		return 0, 0, 0, 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := cli.ContainerStatsOneShot(ctx, containerID)
	if err != nil {
		return 0, 0, 0, 0
	}
	defer resp.Body.Close()

	var v container.StatsResponse
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return 0, 0, 0, 0
	}

	usage := v.MemoryStats.Usage
	if inactive, ok := v.MemoryStats.Stats["inactive_file"]; ok && inactive < usage {
		usage -= inactive
	}
	memBytes = usage
	cpuPercent, rxRate, txRate = computeDockerStatsRates(containerID, &v)
	return cpuPercent, memBytes, rxRate, txRate
}

func computeDockerStatsRates(containerID string, v *container.StatsResponse) (cpuPercent float64, rxRate, txRate int64) {
	return computeDockerStatsRatesAt(containerID, v, time.Now())
}

func computeDockerStatsRatesAt(containerID string, v *container.StatsResponse, now time.Time) (cpuPercent float64, rxRate, txRate int64) {
	curTotal := v.CPUStats.CPUUsage.TotalUsage
	curSys := v.CPUStats.SystemUsage
	preTotal := v.PreCPUStats.CPUUsage.TotalUsage
	preSys := v.PreCPUStats.SystemUsage

	var rxTotal, txTotal uint64
	for _, nw := range v.Networks {
		rxTotal += nw.RxBytes
		txTotal += nw.TxBytes
	}

	dockerCPUMu.Lock()
	if len(dockerCPUMap) >= 512 {
		dockerCPUMap = make(map[string]dockerCPUSample, 32)
	}
	prev, hasPrev := dockerCPUMap[containerID]
	if hasPrev && (preSys == 0 || preTotal == 0) {
		preTotal = prev.totalUsage
		preSys = prev.systemUsage
	}
	rxRate, txRate, netRxBytes, netTxBytes, netSampledAt := nextDockerNetSample(prev, hasPrev, rxTotal, txTotal, now)
	dockerCPUMap[containerID] = dockerCPUSample{
		totalUsage:  curTotal,
		systemUsage: curSys,
		rxBytes:     netRxBytes,
		txBytes:     netTxBytes,
		sampledAt:   netSampledAt,
		rxRate:      rxRate,
		txRate:      txRate,
	}
	dockerCPUMu.Unlock()

	if curTotal > preTotal && curSys > preSys {
		cpuDelta := float64(curTotal - preTotal)
		sysDelta := float64(curSys - preSys)
		onlineCPUs := float64(v.CPUStats.OnlineCPUs)
		if onlineCPUs <= 0 {
			onlineCPUs = float64(len(v.CPUStats.CPUUsage.PercpuUsage))
		}
		if onlineCPUs <= 0 {
			onlineCPUs = 1
		}
		cpuPercent = (cpuDelta / sysDelta) * onlineCPUs * 100.0
	}
	return cpuPercent, rxRate, txRate
}

func computeNetworkRateDelta(prev dockerCPUSample, hasPrev bool, rxTotal, txTotal uint64, now time.Time) (int64, int64) {
	rxRate, txRate, _, _, _ := nextDockerNetSample(prev, hasPrev, rxTotal, txTotal, now)
	return rxRate, txRate
}

func nextDockerNetSample(prev dockerCPUSample, hasPrev bool, rxTotal, txTotal uint64, now time.Time) (rxRate, txRate int64, baseRx, baseTx uint64, sampledAt time.Time) {
	if !hasPrev || prev.sampledAt.IsZero() {
		return 0, 0, rxTotal, txTotal, now
	}
	elapsed := now.Sub(prev.sampledAt).Seconds()
	if elapsed < 0.2 {
		return prev.rxRate, prev.txRate, prev.rxBytes, prev.txBytes, prev.sampledAt
	}
	if rxTotal >= prev.rxBytes {
		rxRate = int64(float64(rxTotal-prev.rxBytes) / elapsed)
	}
	if txTotal >= prev.txBytes {
		txRate = int64(float64(txTotal-prev.txBytes) / elapsed)
	}
	return rxRate, txRate, rxTotal, txTotal, now
}

// measureDirBytes computes the total byte size of regular files inside dirPath using os.Root.
// Missing directories or unreadable entries are safely ignored.
func measureDirBytes(dirPath string) int64 {
	if strings.TrimSpace(dirPath) == "" {
		return 0
	}
	root, err := os.OpenRoot(dirPath)
	if err != nil {
		return 0
	}
	defer func() { _ = root.Close() }()

	var total int64
	var visited int
	_ = fs.WalkDir(root.FS(), ".", func(_ string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		visited++
		if visited > maxInstanceMetricWalkFiles {
			return fs.SkipAll
		}
		if !d.Type().IsRegular() {
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return nil
		}
		if fi.Size() > 0 {
			total += fi.Size()
		}
		return nil
	})
	return total
}

func measureDirBytesCached(dirPath string) int64 {
	if strings.TrimSpace(dirPath) == "" {
		return 0
	}
	info, err := os.Stat(dirPath)
	if err != nil || !info.IsDir() {
		return 0
	}
	now := time.Now()
	diskUsageCacheMu.RLock()
	if cached, ok := diskUsageCacheMap[dirPath]; ok && now.Sub(cached.updatedAt) < 15*time.Second {
		diskUsageCacheMu.RUnlock()
		return cached.bytes
	}
	diskUsageCacheMu.RUnlock()

	total := measureDirBytes(dirPath)
	diskUsageCacheMu.Lock()
	if len(diskUsageCacheMap) >= 512 {
		diskUsageCacheMap = make(map[string]diskUsageCacheEntry, 32)
	}
	diskUsageCacheMap[dirPath] = diskUsageCacheEntry{
		bytes:     total,
		updatedAt: now,
	}
	diskUsageCacheMu.Unlock()
	return total
}

func readMetricFileLinesCached(workDir, uid, tenant, name, relFilePath string) []string {
	root, instDir, err := volume.OpenInstanceRoot(workDir, uid, tenant, name, false)
	if err != nil {
		return nil
	}
	defer func() { _ = root.Close() }()

	fi, rel, err := volume.StatInRoot(root, relFilePath, false)
	if err != nil || fi.IsDir() {
		return nil
	}
	cacheKey := instDir + "::" + rel
	modNano := fi.ModTime().UnixNano()
	sz := fi.Size()
	fileLinesCacheMu.RLock()
	if cached, ok := fileLinesCacheMap[cacheKey]; ok && cached.modTimeNano == modNano && cached.size == sz {
		fileLinesCacheMu.RUnlock()
		return cached.lines
	}
	fileLinesCacheMu.RUnlock()

	rawFile, readErr := volume.ReadFileInRoot(root, rel)
	if readErr != nil {
		return nil
	}
	lines := splitCleanLines(string(rawFile))
	fileLinesCacheMu.Lock()
	if len(fileLinesCacheMap) >= 512 {
		fileLinesCacheMap = make(map[string]fileLinesCacheEntry, 32)
	}
	fileLinesCacheMap[cacheKey] = fileLinesCacheEntry{
		modTimeNano: modNano,
		size:        sz,
		lines:       lines,
	}
	fileLinesCacheMu.Unlock()
	return lines
}

// collectAndRecordInstanceUsage measures and records per-instance CPU, memory, disk, and declarative custom game metrics.
func collectAndRecordInstanceUsage(nodeName, workDir string, inst *corev1.Instance, drv ...runtime.Driver) {
	if inst == nil {
		return
	}
	tenant := inst.Metadata.Tenant
	name := inst.Metadata.Name
	var activeDrv runtime.Driver
	if len(drv) > 0 {
		activeDrv = drv[0]
	}

	var diskBytes int64
	if strings.TrimSpace(workDir) != "" && tenant != "" && name != "" {
		if instDir, err := volume.ResolveInstanceWorkDir(workDir, inst.Metadata.UID, tenant, name, false); err == nil {
			diskBytes = measureDirBytesCached(instDir)
		}
	}

	if inst.Status.Phase != corev1.InstancePhaseRunning {
		customMetrics := collectCustomGameMetrics(nodeName, workDir, inst, nil, false)
		inst.Status.Usage = &corev1.InstanceUsage{
			CPUMillicores: 0,
			MemoryBytes:   0,
			StorageBytes:  diskBytes,
			CustomMetrics: customMetrics,
		}
		return
	}

	var cpuPercent float64
	var memBytes uint64
	var rxRate, txRate int64

	if strings.HasPrefix(inst.Status.RuntimeID, "docker://") {
		containerID := strings.TrimSpace(strings.TrimPrefix(inst.Status.RuntimeID, "docker://"))
		cpuPercent, memBytes, rxRate, txRate = collectDockerContainerStats(containerID)
	}

	customMetrics := collectCustomGameMetrics(nodeName, workDir, inst, activeDrv, true)

	inst.Status.Usage = &corev1.InstanceUsage{
		CPUMillicores:        int64(math.Round(cpuPercent * 10)),
		MemoryBytes:          int64(memBytes),
		StorageBytes:         diskBytes,
		NetworkRxBytesPerSec: rxRate,
		NetworkTxBytesPerSec: txRate,
		CustomMetrics:        customMetrics,
	}
}

func collectCustomGameMetrics(nodeName, workDir string, inst *corev1.Instance, drv runtime.Driver, running bool) []corev1.InstanceCustomMetric {
	if inst == nil || len(inst.Spec.Metrics) == 0 {
		return nil
	}
	if !running || drv == nil {
		return buildZeroCustomGameMetrics(inst.Spec.Metrics)
	}
	tenant, name := inst.Metadata.Tenant, inst.Metadata.Name
	instKey := tenant + "/" + name
	minInterval, hasProbe, needsContainerLogs := inspectMetricSpecsRequirements(inst.Spec.Metrics)
	var execLinesByMetric map[string][]string
	if hasProbe {
		execLinesByMetric = runOrLoadCustomMetricProbes(inst, drv, instKey, minInterval)
	}
	var lines []string
	if needsContainerLogs {
		lines = readRecentContainerLogLines(drv, inst)
	}
	out := make([]corev1.InstanceCustomMetric, 0, len(inst.Spec.Metrics))
	for _, m := range inst.Spec.Metrics {
		metricLines := lines
		src := strings.ToLower(strings.TrimSpace(m.Source))
		if src == corev1.MetricSourceFile && strings.TrimSpace(m.FilePath) != "" && workDir != "" {
			if fileLines := readMetricFileLinesCached(workDir, inst.Metadata.UID, tenant, name, m.FilePath); len(fileLines) > 0 {
				metricLines = fileLines
			}
		} else if src == corev1.MetricSourceExec {
			metricLines = execLinesByMetric[m.ID]
		} else if extra := execLinesByMetric[m.ID]; len(extra) > 0 {
			metricLines = append(append([]string(nil), lines...), extra...)
		}
		out = append(out, EvaluateDeclarativeMetric(m, metricLines))
	}
	return out
}

func buildZeroCustomGameMetrics(specs []corev1.AppMetricSpec) []corev1.InstanceCustomMetric {
	out := make([]corev1.InstanceCustomMetric, 0, len(specs))
	for _, m := range specs {
		out = append(out, corev1.InstanceCustomMetric{
			ID: m.ID, Label: m.Label, Unit: m.Unit, Value: 0, Display: "0",
			Max: m.MaxValue, BindParam: m.BindParam, QuickActionIDs: m.QuickActionIDs, ItemActions: m.ItemActions,
		})
	}
	return out
}

func inspectMetricSpecsRequirements(specs []corev1.AppMetricSpec) (time.Duration, bool, bool) {
	minInterval := 15 * time.Second
	hasProbe := false
	needsContainerLogs := false
	for _, m := range specs {
		src := strings.ToLower(strings.TrimSpace(m.Source))
		if src != corev1.MetricSourceFile && src != corev1.MetricSourceExec {
			needsContainerLogs = true
		}
		if strings.TrimSpace(m.ProbeCommand) != "" {
			hasProbe = true
			if m.IntervalSec > 0 && time.Duration(m.IntervalSec)*time.Second < minInterval {
				minInterval = time.Duration(m.IntervalSec) * time.Second
			}
		}
	}
	return minInterval, hasProbe, needsContainerLogs
}

func runOrLoadCustomMetricProbes(inst *corev1.Instance, drv runtime.Driver, instKey string, minInterval time.Duration) map[string][]string {
	var execLinesByMetric map[string][]string
	customProbeMu.Lock()
	shouldProbe := time.Since(lastProbeAtMap[instKey]) >= minInterval
	if shouldProbe {
		if len(lastProbeAtMap) >= 512 {
			lastProbeAtMap = make(map[string]time.Time, 32)
		}
		lastProbeAtMap[instKey] = time.Now()
	} else {
		for _, m := range inst.Spec.Metrics {
			if cachedLines := lastProbeLinesMap[instKey+"/"+m.ID]; len(cachedLines) > 0 {
				if execLinesByMetric == nil {
					execLinesByMetric = make(map[string][]string)
				}
				execLinesByMetric[m.ID] = cachedLines
			}
		}
	}
	customProbeMu.Unlock()
	if !shouldProbe {
		return execLinesByMetric
	}
	return executeCustomMetricProbes(inst, drv, instKey)
}

func instanceConsoleDelivery(inst *corev1.Instance) string {
	if inst != nil && inst.Spec.BackupPolicy != nil {
		return strings.TrimSpace(inst.Spec.BackupPolicy.ConsoleDelivery)
	}
	return ""
}

func executeCustomMetricProbes(inst *corev1.Instance, drv runtime.Driver, instKey string) map[string][]string {
	var execLinesByMetric map[string][]string
	seenCmd := make(map[string]string)
	delivery := instanceConsoleDelivery(inst)
	sentStdinProbe := false
	for _, m := range inst.Spec.Metrics {
		cmd := strings.TrimSpace(m.ProbeCommand)
		if cmd == "" {
			continue
		}
		src := strings.ToLower(strings.TrimSpace(m.Source))
		cacheKey := src + ":" + delivery + ":" + cmd
		out, ok := seenCmd[cacheKey]
		if !ok {
			probeCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			var execReq runtime.ExecRequest
			if src == corev1.MetricSourceExec {
				execReq = runtime.ExecRequest{Command: []string{"sh", "-c", cmd}}
			} else {
				execReq = runtime.ExecRequest{Command: runtime.BuildConsoleCommandArgv(delivery, cmd)}
				sentStdinProbe = true
			}
			out, _ = drv.Exec(probeCtx, inst, execReq)
			cancel()
			seenCmd[cacheKey] = out
		}
		if strings.TrimSpace(out) != "" {
			if execLinesByMetric == nil {
				execLinesByMetric = make(map[string][]string)
			}
			parsed := splitCleanLines(out)
			execLinesByMetric[m.ID] = parsed
			customProbeMu.Lock()
			if len(lastProbeLinesMap) >= 1024 {
				lastProbeLinesMap = make(map[string][]string, 64)
			}
			lastProbeLinesMap[instKey+"/"+m.ID] = parsed
			customProbeMu.Unlock()
		}
	}
	if sentStdinProbe {
		time.Sleep(80 * time.Millisecond)
	}
	return execLinesByMetric
}

func readRecentContainerLogLines(drv runtime.Driver, inst *corev1.Instance) []string {
	logCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	rc, err := drv.Logs(logCtx, inst, runtime.LogOptions{TailLines: 200})
	if err != nil || rc == nil {
		return nil
	}
	rawBytes, _ := io.ReadAll(io.LimitReader(rc, 256*1024))
	_ = rc.Close()
	return splitCleanLines(string(rawBytes))
}

func splitCleanLines(raw string) []string {
	var out []string
	for _, line := range strings.Split(raw, "\n") {
		clean := strings.TrimSpace(ansiEscapeRegex.ReplaceAllString(strings.TrimRight(line, "\r"), ""))
		if clean != "" {
			out = append(out, clean)
		}
	}
	return out
}

// EvaluateDeclarativeMetric evaluates a single declarative AppMetricSpec against recent log lines.
func EvaluateDeclarativeMetric(m corev1.AppMetricSpec, lines []string) corev1.InstanceCustomMetric {
	cm := corev1.InstanceCustomMetric{
		ID: m.ID, Label: m.Label, Unit: m.Unit, Value: 0, Display: "0",
		Max: m.MaxValue, BindParam: m.BindParam, QuickActionIDs: m.QuickActionIDs, ItemActions: m.ItemActions,
	}
	incRe := getCompiledRegex(m.IncPattern, true)
	decRe := getCompiledRegex(m.DecPattern, true)
	resetRe := getCompiledRegex(m.ResetPattern, true)
	deltaCount, deltaItems := evaluateMetricLogDelta(lines, incRe, decRe, resetRe)
	switch m.Mode {
	case corev1.MetricModeLogDelta:
		if len(deltaItems) > 0 && deltaCount == 0 {
			deltaCount = len(deltaItems)
		}
		cm.Value, cm.Display, cm.Items = float64(deltaCount), strconv.Itoa(deltaCount), deltaItems
		return cm
	case corev1.MetricModeRegexCount:
		return evaluateRegexCountMetric(m, cm, lines, deltaCount, deltaItems)
	default:
		return evaluateRegexValueMetric(m, cm, lines, incRe != nil || decRe != nil, deltaCount, deltaItems)
	}
}

func evaluateMetricLogDelta(lines []string, incRe, decRe, resetRe *regexp.Regexp) (int, []string) {
	if incRe == nil && decRe == nil {
		return 0, nil
	}
	deltaCount := 0
	var deltaItems []string
	for _, line := range lines {
		if resetRe != nil && resetRe.MatchString(line) {
			deltaCount, deltaItems = 0, nil
			continue
		}
		if incRe != nil {
			if sub := incRe.FindStringSubmatch(line); len(sub) > 0 {
				deltaCount++
				if len(sub) > 1 {
					if item := strings.TrimSpace(sub[1]); item != "" && !containsString(deltaItems, item) {
						deltaItems = append(deltaItems, item)
					}
				}
			}
		}
		if decRe != nil {
			if sub := decRe.FindStringSubmatch(line); len(sub) > 0 {
				if deltaCount > 0 {
					deltaCount--
				}
				if len(sub) > 1 {
					if item := strings.TrimSpace(sub[1]); item != "" {
						deltaItems = removeString(deltaItems, item)
					}
				}
			}
		}
	}
	return deltaCount, deltaItems
}

func evaluateRegexCountMetric(m corev1.AppMetricSpec, cm corev1.InstanceCustomMetric, lines []string, deltaCount int, deltaItems []string) corev1.InstanceCustomMetric {
	patRe := getCompiledRegex(m.Pattern, false)
	itemRe := getCompiledRegex(m.ItemPattern, false)
	start := max(0, len(lines)-40)
	var items []string
	for _, line := range lines[start:] {
		if itemRe != nil {
			if sub := itemRe.FindStringSubmatch(line); len(sub) > 0 {
				label := strings.TrimSpace(sub[0])
				if len(sub) >= 3 && strings.TrimSpace(sub[2]) != "" {
					label = strings.TrimSpace(sub[2]) + " (" + strings.TrimSpace(sub[1]) + ")"
				} else if len(sub) >= 2 && strings.TrimSpace(sub[1]) != "" {
					label = strings.TrimSpace(sub[1])
				}
				if label != "" && !containsString(items, label) {
					items = append(items, label)
				}
				continue
			}
		}
		if patRe != nil {
			if sub := patRe.FindStringSubmatch(line); len(sub) > 0 {
				label := strings.TrimSpace(sub[0])
				if len(sub) >= 2 && strings.TrimSpace(sub[1]) != "" {
					label = strings.TrimSpace(sub[1])
				}
				if label != "" && !containsString(items, label) {
					items = append(items, label)
				}
			}
		}
	}
	if len(items) > 0 {
		cm.Value, cm.Display, cm.Items = float64(len(items)), strconv.Itoa(len(items)), items
		return cm
	}
	cm.Value, cm.Display, cm.Items = float64(deltaCount), strconv.Itoa(deltaCount), deltaItems
	return cm
}

func evaluateRegexValueMetric(m corev1.AppMetricSpec, cm corev1.InstanceCustomMetric, lines []string, hasDelta bool, deltaCount int, deltaItems []string) corev1.InstanceCustomMetric {
	patRe := getCompiledRegex(m.Pattern, true)
	itemRe := getCompiledRegex(m.ItemPattern, true)
	resetRe := getCompiledRegex(m.ResetPattern, true)
	matchedIdx := -1
	if patRe != nil {
		for i := len(lines) - 1; i >= 0; i-- {
			if resetRe != nil && resetRe.MatchString(lines[i]) {
				cm.Value, cm.Display, cm.Items = 0, "0", nil
				return cm
			}
			if sub := patRe.FindStringSubmatch(lines[i]); len(sub) >= 2 && strings.TrimSpace(sub[1]) != "" {
				rawVal := strings.TrimSpace(sub[1])
				if isZeroKeyword(rawVal) {
					cm.Value, cm.Display = 0, "0"
				} else {
					cm.Display = rawVal
					if f, err := strconv.ParseFloat(rawVal, 64); err == nil {
						cm.Value = f
					}
				}
				matchedIdx = i
				break
			}
		}
	}
	if matchedIdx >= 0 {
		if cm.Value > 0 && itemRe != nil {
			if extracted := extractRegexValueItems(lines, itemRe, matchedIdx); len(extracted) > 0 {
				cm.Items = extracted
			}
		}
		if len(cm.Items) == 0 && cm.Value > 0 && len(deltaItems) > 0 {
			cm.Items = deltaItems
		}
		return cm
	}
	if hasDelta {
		cm.Value, cm.Display, cm.Items = float64(deltaCount), strconv.Itoa(deltaCount), deltaItems
	}
	return cm
}

func isZeroKeyword(s string) bool {
	return strings.EqualFold(s, "no") || strings.EqualFold(s, "none") || strings.EqualFold(s, "false")
}

func extractRegexValueItems(lines []string, itemRe *regexp.Regexp, matchedIdx int) []string {
	var extracted []string
	end := min(matchedIdx+25, len(lines))
	for _, line := range lines[matchedIdx:end] {
		sub := itemRe.FindStringSubmatch(line)
		if len(sub) < 2 || strings.TrimSpace(sub[1]) == "" {
			continue
		}
		rawItem := strings.TrimSpace(sub[1])
		if strings.Contains(rawItem, ",") {
			for _, part := range strings.Split(rawItem, ",") {
				if p := strings.TrimSpace(part); p != "" && !containsString(extracted, p) {
					extracted = append(extracted, p)
				}
			}
		} else if !containsString(extracted, rawItem) {
			extracted = append(extracted, rawItem)
		}
	}
	return extracted
}

func containsString(slice []string, target string) bool {
	for _, s := range slice {
		if s == target {
			return true
		}
	}
	return false
}

// deleteInstanceUsageMetrics clears the instance's metric collection caches.
// Prometheus series removal is centralized in metrics.DeleteInstanceSeries.
func deleteInstanceUsageMetrics(nodeName, tenant, name string) {
	instKey := tenant + "/" + name
	prefix := instKey + "/"
	customProbeMu.Lock()
	delete(lastProbeAtMap, instKey)
	for k := range lastProbeLinesMap {
		if strings.HasPrefix(k, prefix) {
			delete(lastProbeLinesMap, k)
		}
	}
	customProbeMu.Unlock()
}
