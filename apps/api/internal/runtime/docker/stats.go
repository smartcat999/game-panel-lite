package docker

import (
	"context"
	"encoding/json"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/runtime"
)

func (a *Adapter) StatsWorkload(ctx context.Context, runtimeID string) (runtime.WorkloadStats, error) {
	resp, err := a.client.ContainerStats(ctx, runtimeID, false)
	if err != nil {
		return runtime.WorkloadStats{}, err
	}
	defer resp.Body.Close()
	var data types.StatsJSON
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return runtime.WorkloadStats{}, err
	}
	cpuPercent := 0.0
	cpuDelta := float64(data.CPUStats.CPUUsage.TotalUsage - data.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(data.CPUStats.SystemUsage - data.PreCPUStats.SystemUsage)
	onlineCPUs := float64(data.CPUStats.OnlineCPUs)
	if onlineCPUs == 0 {
		onlineCPUs = float64(len(data.CPUStats.CPUUsage.PercpuUsage))
	}
	if systemDelta > 0 && onlineCPUs > 0 {
		cpuPercent = (cpuDelta / systemDelta) * onlineCPUs * 100
	}
	return runtime.WorkloadStats{
		CPUPercent:    cpuPercent,
		MemoryMB:      int64(data.MemoryStats.Usage) / 1024 / 1024,
		MemoryLimitMB: int64(data.MemoryStats.Limit) / 1024 / 1024,
	}, nil
}

func (a *Adapter) HostStats(ctx context.Context) (runtime.HostStats, error) {
	containers, err := a.client.ContainerList(ctx, types.ContainerListOptions{
		Filters: filters.NewArgs(filters.Arg("label", "gamepanel.instance")),
	})
	if err != nil {
		return runtime.HostStats{}, err
	}
	result := runtime.HostStats{RunningWorkloads: len(containers)}
	for _, c := range containers {
		resp, err := a.client.ContainerStats(ctx, c.ID, false)
		if err != nil {
			continue
		}
		var data types.StatsJSON
		decodeErr := json.NewDecoder(resp.Body).Decode(&data)
		resp.Body.Close()
		if decodeErr != nil {
			continue
		}
		cpuDelta := float64(data.CPUStats.CPUUsage.TotalUsage - data.PreCPUStats.CPUUsage.TotalUsage)
		systemDelta := float64(data.CPUStats.SystemUsage - data.PreCPUStats.SystemUsage)
		onlineCPUs := float64(data.CPUStats.OnlineCPUs)
		if onlineCPUs == 0 {
			onlineCPUs = float64(len(data.CPUStats.CPUUsage.PercpuUsage))
		}
		if systemDelta > 0 && onlineCPUs > 0 {
			result.TotalCPUPercent += (cpuDelta / systemDelta) * onlineCPUs * 100
		}
		result.TotalMemoryMB += int64(data.MemoryStats.Usage) / 1024 / 1024
		if int64(data.MemoryStats.Limit)/1024/1024 > result.MemoryLimitMB {
			result.MemoryLimitMB = int64(data.MemoryStats.Limit) / 1024 / 1024
		}
	}
	return result, nil
}
