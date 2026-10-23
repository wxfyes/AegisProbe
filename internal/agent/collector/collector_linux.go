//go:build linux

package collector

import (
	"bufio"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"tianque_probe/internal/common"
)

type LinuxCollector struct {
	lastCPUTotal uint64
	lastCPUIdle  uint64
	lastNetIn    uint64
	lastNetOut   uint64
	lastSample   time.Time
	cachedOS     string
}

func newPlatformCollector() SystemCollector {
	c := &LinuxCollector{
		lastSample: time.Now(),
		cachedOS:   detectLinuxOS(),
	}
	// 初次采样以校准 CPU 与网络基准
	c.sampleCPU()
	c.sampleNet()
	return c
}

func detectLinuxOS() string {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return "linux"
	}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "ID=") {
			val := strings.TrimPrefix(line, "ID=")
			val = strings.Trim(val, `"`)
			val = strings.ToLower(val)
			switch {
			case strings.Contains(val, "debian"):
				return "debian"
			case strings.Contains(val, "ubuntu"):
				return "ubuntu"
			case strings.Contains(val, "alpine"):
				return "alpine"
			case strings.Contains(val, "centos"):
				return "centos"
			case strings.Contains(val, "rocky"):
				return "rocky"
			case strings.Contains(val, "alma"):
				return "alma"
			case strings.Contains(val, "fedora"):
				return "fedora"
			case strings.Contains(val, "arch"):
				return "arch"
			default:
				return val
			}
		}
	}
	return "linux"
}

func (c *LinuxCollector) sampleCPU() (uint64, uint64) {
	file, err := os.Open("/proc/stat")
	if err != nil {
		return 0, 0
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "cpu ") {
			fields := strings.Fields(line)
			if len(fields) >= 8 {
				var total uint64
				for i := 1; i < len(fields); i++ {
					v, _ := strconv.ParseUint(fields[i], 10, 64)
					total += v
				}
				idle, _ := strconv.ParseUint(fields[4], 10, 64)
				iowait, _ := strconv.ParseUint(fields[5], 10, 64)
				idleTotal := idle + iowait
				return total, idleTotal
			}
			break
		}
	}
	return 0, 0
}

func (c *LinuxCollector) sampleNet() (uint64, uint64) {
	file, err := os.Open("/proc/net/dev")
	if err != nil {
		return 0, 0
	}
	defer file.Close()

	var totalIn, totalOut uint64
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.Contains(line, ":") {
			parts := strings.SplitN(line, ":", 2)
			ifName := strings.TrimSpace(parts[0])
			// 过滤回环与虚拟容器网卡
			if ifName == "lo" || strings.HasPrefix(ifName, "docker") ||
				strings.HasPrefix(ifName, "veth") || strings.HasPrefix(ifName, "br-") ||
				strings.HasPrefix(ifName, "cali") || strings.HasPrefix(ifName, "flannel") {
				continue
			}
			fields := strings.Fields(parts[1])
			if len(fields) >= 9 {
				rx, _ := strconv.ParseUint(fields[0], 10, 64)
				tx, _ := strconv.ParseUint(fields[8], 10, 64)
				totalIn += rx
				totalOut += tx
			}
		}
	}
	return totalIn, totalOut
}

func (c *LinuxCollector) Collect() (*common.MetricsData, error) {
	now := time.Now()
	elapsed := now.Sub(c.lastSample).Seconds()
	if elapsed <= 0 {
		elapsed = 1.0
	}

	// 1. CPU 占用率
	currCPUTotal, currCPUIdle := c.sampleCPU()
	var cpuPercent float64
	if c.lastCPUTotal > 0 && currCPUTotal > c.lastCPUTotal {
		deltaTotal := float64(currCPUTotal - c.lastCPUTotal)
		deltaIdle := float64(currCPUIdle - c.lastCPUIdle)
		if deltaTotal > 0 {
			cpuPercent = (1.0 - (deltaIdle / deltaTotal)) * 100.0
			if cpuPercent < 0 {
				cpuPercent = 0
			}
			if cpuPercent > 100 {
				cpuPercent = 100
			}
		}
	}
	c.lastCPUTotal = currCPUTotal
	c.lastCPUIdle = currCPUIdle

	// 2. 内存与 SWAP
	var memUsed, memTotal, swapUsed, swapTotal uint64
	memFile, err := os.Open("/proc/meminfo")
	if err == nil {
		defer memFile.Close()
		scanner := bufio.NewScanner(memFile)
		var memAvailable, memFree, buffers, cached uint64
		hasAvailable := false

		for scanner.Scan() {
			line := scanner.Text()
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				val, _ := strconv.ParseUint(parts[1], 10, 64)
				valBytes := val * 1024
				switch parts[0] {
				case "MemTotal:":
					memTotal = valBytes
				case "MemFree:":
					memFree = valBytes
				case "MemAvailable:":
					memAvailable = valBytes
					hasAvailable = true
				case "Buffers:":
					buffers = valBytes
				case "Cached:":
					cached = valBytes
				case "SwapTotal:":
					swapTotal = valBytes
				case "SwapFree:":
					swapFree := valBytes
					if swapTotal >= swapFree {
						swapUsed = swapTotal - swapFree
					}
				}
			}
		}
		if hasAvailable {
			if memTotal >= memAvailable {
				memUsed = memTotal - memAvailable
			}
		} else {
			actualFree := memFree + buffers + cached
			if memTotal >= actualFree {
				memUsed = memTotal - actualFree
			}
		}
	}

	// 3. 根磁盘使用率
	var diskUsed, diskTotal uint64
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err == nil {
		bsize := uint64(stat.Bsize)
		diskTotal = stat.Blocks * bsize
		free := stat.Bavail * bsize
		if diskTotal >= free {
			diskUsed = diskTotal - free
		}
	}

	// 4. 网络速率与累计流量
	currNetIn, currNetOut := c.sampleNet()
	var inSpeed, outSpeed uint64
	if c.lastNetIn > 0 && currNetIn >= c.lastNetIn {
		inSpeed = uint64(float64(currNetIn-c.lastNetIn) / elapsed)
	}
	if c.lastNetOut > 0 && currNetOut >= c.lastNetOut {
		outSpeed = uint64(float64(currNetOut-c.lastNetOut) / elapsed)
	}
	c.lastNetIn = currNetIn
	c.lastNetOut = currNetOut

	// 5. 系统负载
	var l1, l5, l15 float64
	loadData, err := os.ReadFile("/proc/loadavg")
	if err == nil {
		fields := strings.Fields(string(loadData))
		if len(fields) >= 3 {
			l1, _ = strconv.ParseFloat(fields[0], 64)
			l5, _ = strconv.ParseFloat(fields[1], 64)
			l15, _ = strconv.ParseFloat(fields[2], 64)
		}
	}

	// 6. 系统 Uptime
	var uptime uint64
	uptimeData, err := os.ReadFile("/proc/uptime")
	if err == nil {
		fields := strings.Fields(string(uptimeData))
		if len(fields) >= 1 {
			upSec, _ := strconv.ParseFloat(fields[0], 64)
			uptime = uint64(upSec)
		}
	}

	c.lastSample = now

	return &common.MetricsData{
		CPUPercent:    cpuPercent,
		MemoryUsed:    memUsed,
		MemoryTotal:   memTotal,
		SwapUsed:      swapUsed,
		SwapTotal:     swapTotal,
		DiskUsed:      diskUsed,
		DiskTotal:     diskTotal,
		NetInSpeed:    inSpeed,
		NetOutSpeed:   outSpeed,
		NetInTotal:    currNetIn,
		NetOutTotal:   currNetOut,
		Load1:         l1,
		Load5:         l5,
		Load15:        l15,
		UptimeSeconds: uptime,
		CPUCores:      runtime.NumCPU(),
		Arch:          runtime.GOARCH,
		OS:            c.cachedOS,
	}, nil
}
