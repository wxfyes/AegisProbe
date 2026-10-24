//go:build !linux && !windows

package collector

import (
	"runtime"
	"time"

	"tianque_probe/internal/common"
)

type OtherCollector struct {
	startTime time.Time
}

func newPlatformCollector() SystemCollector {
	return &OtherCollector{startTime: time.Now()}
}

func (c *OtherCollector) Collect() (*common.MetricsData, error) {
	return &common.MetricsData{
		Version:       common.Version,
		CPUPercent:    1.0,
		MemoryUsed:    1024 * 1024 * 512,
		MemoryTotal:   1024 * 1024 * 2048,
		SwapUsed:      0,
		SwapTotal:     0,
		DiskUsed:      1024 * 1024 * 1024 * 10,
		DiskTotal:     1024 * 1024 * 1024 * 50,
		NetInSpeed:    1024 * 8,
		NetOutSpeed:   1024 * 6,
		NetInTotal:    1024 * 1024 * 100,
		NetOutTotal:   1024 * 1024 * 80,
		Load1:         0.05,
		Load5:         0.02,
		Load15:        0.00,
		UptimeSeconds: uint64(time.Since(c.startTime).Seconds()),
		CPUCores:      runtime.NumCPU(),
		Arch:          runtime.GOARCH,
		OS:            runtime.GOOS,
	}, nil
}
