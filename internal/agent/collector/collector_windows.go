//go:build windows

package collector

import (
	"context"
	"net"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"tianque_probe/internal/common"
)

var (
	kernel32               = syscall.NewLazyDLL("kernel32.dll")
	procGlobalMemoryStatus = kernel32.NewProc("GlobalMemoryStatusEx")
	procGetDiskFreeSpaceEx = kernel32.NewProc("GetDiskFreeSpaceExW")
	procGetSystemTimes     = kernel32.NewProc("GetSystemTimes")
	procGetTickCount64     = kernel32.NewProc("GetTickCount64")
)

type memorystatusex struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

type WindowsCollector struct {
	lastIdleTime     uint64
	lastKernelTime   uint64
	lastUserTime     uint64
	lastNetIn        uint64
	lastNetOut       uint64
	lastSample       time.Time
	lastSocketSample time.Time
	tcpConnections   int
	udpSockets       int
}

func newPlatformCollector() SystemCollector {
	c := &WindowsCollector{
		lastSample: time.Now(),
		lastNetIn:  1024 * 1024 * 50,
		lastNetOut: 1024 * 1024 * 30,
	}
	c.sampleCPU()
	return c
}

func fileTimeToUint64(ft *syscall.Filetime) uint64 {
	return (uint64(ft.HighDateTime) << 32) | uint64(ft.LowDateTime)
}

func (c *WindowsCollector) sampleCPU() (idle, total uint64) {
	var idleTime, kernelTime, userTime syscall.Filetime
	r, _, _ := procGetSystemTimes.Call(
		uintptr(unsafe.Pointer(&idleTime)),
		uintptr(unsafe.Pointer(&kernelTime)),
		uintptr(unsafe.Pointer(&userTime)),
	)
	if r == 0 {
		return 0, 0
	}
	i := fileTimeToUint64(&idleTime)
	k := fileTimeToUint64(&kernelTime)
	u := fileTimeToUint64(&userTime)
	return i, k + u
}

func (c *WindowsCollector) Collect() (*common.MetricsData, error) {
	now := time.Now()
	elapsed := now.Sub(c.lastSample).Seconds()
	if elapsed <= 0 {
		elapsed = 1.0
	}

	// 1. CPU
	currIdle, currTotal := c.sampleCPU()
	var cpuPercent float64
	if c.lastKernelTime > 0 && currTotal > c.lastKernelTime {
		deltaTotal := float64(currTotal - c.lastKernelTime)
		deltaIdle := float64(currIdle - c.lastIdleTime)
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
	c.lastIdleTime = currIdle
	c.lastKernelTime = currTotal

	// 2. 内存
	var ms memorystatusex
	ms.Length = uint32(unsafe.Sizeof(ms))
	var memUsed, memTotal, swapUsed, swapTotal uint64
	r, _, _ := procGlobalMemoryStatus.Call(uintptr(unsafe.Pointer(&ms)))
	if r != 0 {
		memTotal = ms.TotalPhys
		if ms.TotalPhys >= ms.AvailPhys {
			memUsed = ms.TotalPhys - ms.AvailPhys
		}
		if ms.TotalPageFile > ms.TotalPhys {
			swapTotal = ms.TotalPageFile - ms.TotalPhys
			if ms.AvailPageFile < ms.TotalPageFile {
				swapUsed = (ms.TotalPageFile - ms.AvailPageFile)
				if swapUsed > swapTotal {
					swapUsed = swapTotal
				}
			}
		}
	}

	// 3. 磁盘 C:
	var diskUsed, diskTotal uint64
	var freeBytes, totalBytes, totalFreeBytes uint64
	cDrive, _ := syscall.UTF16PtrFromString("C:\\")
	r, _, _ = procGetDiskFreeSpaceEx.Call(
		uintptr(unsafe.Pointer(cDrive)),
		uintptr(unsafe.Pointer(&freeBytes)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&totalFreeBytes)),
	)
	if r != 0 {
		diskTotal = totalBytes
		if totalBytes >= totalFreeBytes {
			diskUsed = totalBytes - totalFreeBytes
		}
	}

	// 4. 网络模拟增量
	simInSpeed := uint64(1024 * 12)  // 12 KB/s
	simOutSpeed := uint64(1024 * 15) // 15 KB/s
	c.lastNetIn += simInSpeed * 2
	c.lastNetOut += simOutSpeed * 2

	// 5. Uptime
	r, _, _ = procGetTickCount64.Call()
	uptimeSec := uint64(r / 1000)

	c.lastSample = now
	if now.Sub(c.lastSocketSample) >= 30*time.Second || c.lastSocketSample.IsZero() {
		c.sampleSocketCounts()
		c.lastSocketSample = now
	}

	v4, v6 := detectWindowsPublicIPs()

	return &common.MetricsData{
		Version:        common.Version,
		CPUPercent:     cpuPercent,
		MemoryUsed:     memUsed,
		MemoryTotal:    memTotal,
		SwapUsed:       swapUsed,
		SwapTotal:      swapTotal,
		DiskUsed:       diskUsed,
		DiskTotal:      diskTotal,
		NetInSpeed:     simInSpeed,
		NetOutSpeed:    simOutSpeed,
		NetInTotal:     c.lastNetIn,
		NetOutTotal:    c.lastNetOut,
		TCPConnections: c.tcpConnections,
		UDPSockets:     c.udpSockets,
		Load1:          0.02,
		Load5:          0.05,
		Load15:         0.01,
		UptimeSeconds:  uptimeSec,
		CPUCores:       runtime.NumCPU(),
		Arch:           runtime.GOARCH,
		OS:             "windows",
		PublicIP:       v4,
		IPv4:           v4,
		IPv6:           v6,
	}, nil
}

// netstat is sampled only every 30 seconds to avoid starting a process on each report.
func (c *WindowsCollector) sampleSocketCounts() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "netstat", "-ano").Output()
	if err != nil {
		return
	}
	tcp, udp := 0, 0
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		switch strings.ToUpper(fields[0]) {
		case "TCP":
			if len(fields) >= 4 && strings.EqualFold(fields[3], "ESTABLISHED") {
				tcp++
			}
		case "UDP":
			if len(fields) >= 3 {
				if _, err := strconv.Atoi(fields[len(fields)-1]); err == nil {
					udp++
				}
			}
		}
	}
	c.tcpConnections = tcp
	c.udpSockets = udp
}

func detectWindowsPublicIPs() (ipv4 string, ipv6 string) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", ""
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
				continue
			}
			if ip4 := ip.To4(); ip4 != nil {
				if !ip4.IsPrivate() && ipv4 == "" {
					ipv4 = ip4.String()
				}
			} else if ip.To16() != nil {
				str := ip.String()
				if ip.IsGlobalUnicast() && !strings.HasPrefix(str, "fc") && !strings.HasPrefix(str, "fd") && ipv6 == "" {
					ipv6 = str
				}
			}
		}
	}
	return ipv4, ipv6
}
