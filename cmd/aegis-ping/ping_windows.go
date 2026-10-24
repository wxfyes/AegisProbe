//go:build windows

package main

// Windows 平台降级实现：Windows 下普通程序无法直接创建原始 ICMP 套接字，
// 改用多端口 TCP 连通性探测代替（效果等同于"tcping"）。
// 软路由通常运行 Linux，此文件仅为保持 Windows 编译兼容性存在。

import (
	"net"
	"time"
)

func icmpPing(addr string, timeout time.Duration) (ok bool, latencyMs int64) {
	ports := []string{"80", "443", "22", "8080", "8443"}
	perTimeout := timeout / time.Duration(len(ports))
	for _, p := range ports {
		target := net.JoinHostPort(addr, p)
		start := time.Now()
		conn, err := net.DialTimeout("tcp", target, perTimeout)
		if err == nil {
			conn.Close()
			return true, time.Since(start).Milliseconds()
		}
	}
	return false, 0
}
