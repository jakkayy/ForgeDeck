// Package host samples resource usage of the machine the Agent runs on.
package host

import "github.com/jakkayy/ForgeDeck/internal/agentproto"

// Sampler reports host resource usage. CPU usage is measured between
// consecutive calls, so the first call reports 0% CPU.
type Sampler struct {
	diskPath string
	prevCPU  cpuTimes
}

// NewSampler measures disk usage of the filesystem holding diskPath.
func NewSampler(diskPath string) *Sampler {
	return &Sampler{diskPath: diskPath}
}

// Sample returns current usage, or nil when it cannot be read on this host.
func (s *Sampler) Sample() *agentproto.Resources {
	cpu, cpuErr := readCPUTimes()
	memory, memErr := memoryPercent()
	disk, diskErr := diskPercent(s.diskPath)
	if cpuErr != nil || memErr != nil || diskErr != nil {
		return nil
	}
	usage := cpu.busyPercentSince(s.prevCPU)
	s.prevCPU = cpu
	return &agentproto.Resources{CPUPercent: usage, MemoryPercent: memory, DiskPercent: disk}
}

// cpuTimes are cumulative jiffies from /proc/stat.
type cpuTimes struct {
	idle, total uint64
}

func (c cpuTimes) busyPercentSince(prev cpuTimes) float32 {
	if prev.total == 0 || c.total <= prev.total {
		return 0
	}
	total := float64(c.total - prev.total)
	idle := float64(c.idle - prev.idle)
	return clampPercent(100 * (total - idle) / total)
}

func clampPercent(v float64) float32 {
	return float32(min(max(v, 0), 100))
}
