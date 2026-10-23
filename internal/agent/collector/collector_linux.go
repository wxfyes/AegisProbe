//go:build linux

package collector

import (
	"bufio"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"tianque_probe/internal/common"
)

type LinuxCollector struct {
	lastCPUTotal  uint64
	lastCPUIdle   uint64
	lastNetIn     uint64
	lastNetOut    uint64
	lastSample    time.Time
	cachedOS      string
	cachedIP      string
	cachedIPv4    string
	cachedIPv6    string
	cachedCountry string
	lastGeoCheck  time.Time
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

	// 1. 本地网卡扫描全局单播双栈 IP (毫秒级，不耗外网流量)
	v4, v6 := detectLocalPublicIPs()
	if v4 != "" {
		c.cachedIPv4 = v4
	}
	if v6 != "" {
		c.cachedIPv6 = v6
	}

	// 2. 异步轻量探测公网 IP 与国家代码 (每小时或初次触发一次)
	if time.Since(c.lastGeoCheck) > 1*time.Hour || c.cachedCountry == "" {
		c.lastGeoCheck = now
		go func() {
			client := &http.Client{Timeout: 4 * time.Second}
			// 获取综合地理信息
			resp, err := client.Get("https://api.ip.sb/geoip")
			if err == nil {
				defer resp.Body.Close()
				var res struct {
					IP          string `json:"ip"`
					CountryCode string `json:"country_code"`
				}
				if json.NewDecoder(resp.Body).Decode(&res) == nil {
					if res.IP != "" {
						c.cachedIP = res.IP
						if strings.Contains(res.IP, ":") && c.cachedIPv6 == "" {
							c.cachedIPv6 = res.IP
						} else if strings.Contains(res.IP, ".") && c.cachedIPv4 == "" {
							c.cachedIPv4 = res.IP
						}
					}
					if res.CountryCode != "" {
						c.cachedCountry = res.CountryCode
					}
				}
			}

			// 若依然缺少 v4，尝试补充 v4
			if c.cachedIPv4 == "" {
				if r4, err := client.Get("https://api-ipv4.ip.sb/ip"); err == nil {
					defer r4.Body.Close()
					if b, _ := io.ReadAll(r4.Body); len(b) > 0 {
						ip := strings.TrimSpace(string(b))
						if strings.Contains(ip, ".") {
							c.cachedIPv4 = ip
						}
					}
				}
			}

			// 若依然缺少 v6，尝试补充 v6
			if c.cachedIPv6 == "" {
				if r6, err := client.Get("https://api-ipv6.ip.sb/ip"); err == nil {
					defer r6.Body.Close()
					if b, _ := io.ReadAll(r6.Body); len(b) > 0 {
						ip := strings.TrimSpace(string(b))
						if strings.Contains(ip, ":") {
							c.cachedIPv6 = ip
						}
					}
				}
			}
		}()
	}

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
		PublicIP:      c.cachedIP,
		IPv4:          c.cachedIPv4,
		IPv6:          c.cachedIPv6,
		CountryCode:   c.cachedCountry,
	}, nil
}

// detectLocalPublicIPs 扫描本机全部网卡提取物理绑定的公网 IPv4 与 IPv6
func detectLocalPublicIPs() (ipv4 string, ipv6 string) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", ""
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}
		name := strings.ToLower(iface.Name)
		if strings.HasPrefix(name, "docker") || strings.HasPrefix(name, "veth") || strings.HasPrefix(name, "br-") || strings.HasPrefix(name, "tun") {
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
			if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
				continue
			}
			if ip4 := ip.To4(); ip4 != nil {
				if !ip4.IsPrivate() && ipv4 == "" {
					ipv4 = ip4.String()
				}
			} else if ip.To16() != nil {
				// 全局单播 IPv6 (通常为 2xxx: / 3xxx:)
				str := ip.String()
				if ip.IsGlobalUnicast() && !strings.HasPrefix(str, "fc") && !strings.HasPrefix(str, "fd") && ipv6 == "" {
					ipv6 = str
				}
			}
		}
	}
	return ipv4, ipv6
}
