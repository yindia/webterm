package server

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/process"
)

type metricsSnapshot struct {
	Timestamp time.Time       `json:"timestamp"`
	CPU       cpuMetrics      `json:"cpu"`
	Memory    memoryMetrics   `json:"memory"`
	Process   processMetrics  `json:"process"`
	TopCPU    []processDetail `json:"top_cpu"`
	TopMemory []processDetail `json:"top_memory"`
	GPU       gpuMetrics      `json:"gpu"`
}

type cpuMetrics struct {
	Cores        int       `json:"cores"`
	UsagePercent float64   `json:"usage_percent"`
	PerCore      []float64 `json:"per_core"`
	Available    bool      `json:"available"`
}

type memoryMetrics struct {
	TotalBytes     uint64 `json:"total_bytes"`
	UsedBytes      uint64 `json:"used_bytes"`
	FreeBytes      uint64 `json:"free_bytes"`
	AvailableBytes uint64 `json:"available_bytes"`
	CachedBytes    uint64 `json:"cached_bytes"`
	SwapTotalBytes uint64 `json:"swap_total_bytes"`
	SwapUsedBytes  uint64 `json:"swap_used_bytes"`
	Available      bool   `json:"available"`
}

type processMetrics struct {
	PID         int     `json:"pid"`
	CPUPercent  float64 `json:"cpu_percent"`
	MemoryBytes uint64  `json:"memory_bytes"`
	Goroutines  int     `json:"goroutines"`
	Available   bool    `json:"available"`
}

type processDetail struct {
	PID         int     `json:"pid"`
	Name        string  `json:"name"`
	CPUPercent  float64 `json:"cpu_percent"`
	MemoryBytes uint64  `json:"memory_bytes"`
}

type gpuMetrics struct {
	Available bool   `json:"available"`
	Note      string `json:"note"`
}

func collectMetrics(limit int, offset int) metricsSnapshot {
	memStats := runtime.MemStats{}
	runtime.ReadMemStats(&memStats)

	metrics := metricsSnapshot{
		Timestamp: time.Now().UTC(),
		CPU: cpuMetrics{
			Cores: runtime.NumCPU(),
		},
		Memory: memoryMetrics{
			TotalBytes:     memStats.Sys,
			UsedBytes:      memStats.Alloc,
			FreeBytes:      0,
			AvailableBytes: 0,
			CachedBytes:    0,
			SwapTotalBytes: 0,
			SwapUsedBytes:  0,
			Available:      true,
		},
		Process: processMetrics{
			PID:         os.Getpid(),
			MemoryBytes: memStats.Alloc,
			Goroutines:  runtime.NumGoroutine(),
			Available:   true,
		},
		GPU: gpuMetrics{
			Available: false,
			Note:      "GPU metrics not available",
		},
	}

	if usage, perCore, err := readCPUUsage(); err == nil {
		metrics.CPU.Available = true
		metrics.CPU.UsagePercent = usage
		metrics.CPU.PerCore = perCore
	}

	if total, used, err := readMemoryUsage(); err == nil {
		metrics.Memory.Available = true
		metrics.Memory.TotalBytes = total
		metrics.Memory.UsedBytes = used
		if free, avail, cached, swapTotal, swapUsed, err := readMemoryDetails(); err == nil {
			metrics.Memory.FreeBytes = free
			metrics.Memory.AvailableBytes = avail
			metrics.Memory.CachedBytes = cached
			metrics.Memory.SwapTotalBytes = swapTotal
			metrics.Memory.SwapUsedBytes = swapUsed
		}
	}

	if cpuPercent, memBytes, err := readProcessUsage(); err == nil {
		metrics.Process.Available = true
		metrics.Process.CPUPercent = cpuPercent
		metrics.Process.MemoryBytes = memBytes
	}

	if topCPU, topMem, err := readTopProcesses(limit, offset); err == nil {
		metrics.TopCPU = topCPU
		metrics.TopMemory = topMem
	}

	return metrics
}

func readProcCPUUsage() (float64, []float64, error) {
	stat, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0, nil, err
	}
	lines := strings.Split(string(stat), "\n")
	var totalUsage float64
	perCore := make([]float64, 0)
	for _, line := range lines {
		if !strings.HasPrefix(line, "cpu") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		idle, _ := strconv.ParseFloat(fields[4], 64)
		var total float64
		for i := 1; i < len(fields); i++ {
			v, _ := strconv.ParseFloat(fields[i], 64)
			total += v
		}
		if total == 0 {
			continue
		}
		usage := (total - idle) / total * 100
		if fields[0] == "cpu" {
			totalUsage = usage
		} else {
			perCore = append(perCore, usage)
		}
	}
	if totalUsage == 0 {
		return 0, nil, errors.New("cpu usage unavailable")
	}
	return totalUsage, perCore, nil
}

func readProcMemUsage() (uint64, uint64, error) {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0, err
	}
	defer file.Close()

	var total, available uint64
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				val, _ := strconv.ParseUint(fields[1], 10, 64)
				total = val * 1024
			}
		}
		if strings.HasPrefix(line, "MemAvailable:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				val, _ := strconv.ParseUint(fields[1], 10, 64)
				available = val * 1024
			}
		}
	}
	if total == 0 {
		return 0, 0, errors.New("memory unavailable")
	}
	used := total - available
	return total, used, nil
}

func readProcProcessUsage() (float64, uint64, error) {
	stat, err := os.ReadFile("/proc/self/stat")
	if err != nil {
		return 0, 0, err
	}
	fields := strings.Fields(string(stat))
	if len(fields) < 24 {
		return 0, 0, errors.New("process stat unavailable")
	}
	utime, _ := strconv.ParseFloat(fields[13], 64)
	stime, _ := strconv.ParseFloat(fields[14], 64)
	starttime, _ := strconv.ParseFloat(fields[21], 64)
	clkTck := float64(100)
	if clk := os.Getenv("CLK_TCK"); clk != "" {
		if v, err := strconv.ParseFloat(clk, 64); err == nil {
			clkTck = v
		}
	}
	uptime := readProcUptime()
	if uptime <= 0 {
		uptime = 1
	}
	procTime := (utime + stime) / clkTck
	cpuPercent := 100 * procTime / (uptime - (starttime / clkTck))
	if cpuPercent < 0 {
		cpuPercent = 0
	}
	memBytes := uint64(0)
	if v, err := strconv.ParseUint(fields[23], 10, 64); err == nil {
		memBytes = v
	}
	return cpuPercent, memBytes, nil
}

func readProcUptime() float64 {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0
	}
	val, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0
	}
	return val
}

func readCPUUsage() (float64, []float64, error) {
	if total, perCore, err := readGopsutilCPU(); err == nil {
		return total, perCore, nil
	}
	return readProcCPUUsage()
}

func readMemoryUsage() (uint64, uint64, error) {
	if total, used, err := readGopsutilMemory(); err == nil {
		return total, used, nil
	}
	return readProcMemUsage()
}

func readMemoryDetails() (uint64, uint64, uint64, uint64, uint64, error) {
	vm, err := mem.VirtualMemory()
	if err != nil {
		return 0, 0, 0, 0, 0, err
	}
	swap, err := mem.SwapMemory()
	if err != nil {
		swap = &mem.SwapMemoryStat{}
	}
	return vm.Free, vm.Available, vm.Cached, swap.Total, swap.Used, nil
}

func readProcessUsage() (float64, uint64, error) {
	if cpuPercent, memBytes, err := readGopsutilProcess(); err == nil {
		return cpuPercent, memBytes, nil
	}
	return readProcProcessUsage()
}

func readGopsutilCPU() (float64, []float64, error) {
	total, err := cpu.Percent(0, false)
	if err != nil || len(total) == 0 {
		return 0, nil, errors.New("cpu unavailable")
	}
	perCore, err := cpu.Percent(0, true)
	if err != nil {
		perCore = nil
	}
	return total[0], perCore, nil
}

func readGopsutilMemory() (uint64, uint64, error) {
	vm, err := mem.VirtualMemory()
	if err != nil {
		return 0, 0, err
	}
	return vm.Total, vm.Used, nil
}

func readGopsutilProcess() (float64, uint64, error) {
	p, err := process.NewProcess(int32(os.Getpid()))
	if err != nil {
		return 0, 0, err
	}
	cpuPercent, err := p.CPUPercent()
	if err != nil {
		cpuPercent = 0
	}
	memInfo, err := p.MemoryInfo()
	if err != nil {
		return cpuPercent, 0, err
	}
	return cpuPercent, memInfo.RSS, nil
}

func readTopProcesses(limit int, offset int) ([]processDetail, []processDetail, error) {
	procs, err := process.Processes()
	if err != nil {
		return nil, nil, err
	}
	items := make([]processDetail, 0, len(procs))
	for _, p := range procs {
		name, err := p.Name()
		if err != nil {
			continue
		}
		cpuPercent, err := p.CPUPercent()
		if err != nil {
			cpuPercent = 0
		}
		memInfo, err := p.MemoryInfo()
		if err != nil {
			continue
		}
		items = append(items, processDetail{
			PID:         int(p.Pid),
			Name:        name,
			CPUPercent:  cpuPercent,
			MemoryBytes: memInfo.RSS,
		})
	}
	if len(items) == 0 {
		return nil, nil, errors.New("no process data")
	}
	byCPU := append([]processDetail{}, items...)
	byMem := append([]processDetail{}, items...)
	sort.Slice(byCPU, func(i, j int) bool { return byCPU[i].CPUPercent > byCPU[j].CPUPercent })
	sort.Slice(byMem, func(i, j int) bool { return byMem[i].MemoryBytes > byMem[j].MemoryBytes })
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = 10
	}
	endCPU := offset + limit
	if offset > len(byCPU) {
		offset = len(byCPU)
	}
	if endCPU > len(byCPU) {
		endCPU = len(byCPU)
	}
	byCPU = byCPU[offset:endCPU]

	endMem := offset + limit
	if offset > len(byMem) {
		offset = len(byMem)
	}
	if endMem > len(byMem) {
		endMem = len(byMem)
	}
	byMem = byMem[offset:endMem]
	return byCPU, byMem, nil
}

func formatBytes(bytes uint64) string {
	units := []string{"B", "KB", "MB", "GB", "TB"}
	val := float64(bytes)
	idx := 0
	for val >= 1024 && idx < len(units)-1 {
		val /= 1024
		idx++
	}
	return fmt.Sprintf("%.1f %s", val, units[idx])
}
