//go:build linux || darwin

package main

// ICMP Ping 实现（Linux / macOS / OpenWrt 原生支持）
// 使用特权原始 ICMP 套接字（需要 root 或 CAP_NET_RAW），
// 这在软路由上几乎是标配运行环境。

import (
	"encoding/binary"
	"math/rand"
	"net"
	"time"
)

// icmpPing 向目标 IP 发送 ICMP Echo Request，返回是否成功与延迟毫秒数
func icmpPing(addr string, timeout time.Duration) (ok bool, latencyMs int64) {
	// 先尝试特权 ICMP 原始套接字（root 环境，如 OpenWrt）
	if ok, ms := icmpPingRaw(addr, timeout); ok {
		return true, ms
	}
	// 若权限不足（普通用户环境），降级为 TCP Probe 到 80 端口作为连通性替代
	return icmpFallbackTCP(addr, timeout)
}

func icmpPingRaw(addr string, timeout time.Duration) (bool, int64) {
	ip := net.ParseIP(addr)
	if ip == nil {
		return false, 0
	}

	network := "ip4:icmp"
	if ip.To4() == nil {
		network = "ip6:ipv6-icmp"
	}

	conn, err := net.DialTimeout(network, addr, timeout)
	if err != nil {
		return false, 0
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(timeout))

	id := uint16(rand.Intn(0xffff))
	seq := uint16(1)

	// 构造 ICMP Echo Request 包
	msg := buildICMPEcho(id, seq)

	start := time.Now()
	if _, err := conn.Write(msg); err != nil {
		return false, 0
	}

	// 读取回复
	buf := make([]byte, 1500)
	n, err := conn.Read(buf)
	if err != nil || n < 8 {
		return false, 0
	}

	latency := time.Since(start).Milliseconds()

	// 校验 ICMP 类型 0 (Echo Reply) 且 ID 匹配
	if ip.To4() != nil {
		// IPv4: buf 从 IP 头部开始（20 字节），ICMP 从第 20 字节起
		offset := 0
		if n >= 28 {
			offset = 20
		}
		if buf[offset] != 0 { // type == 0 Echo Reply
			return false, 0
		}
		replyID := binary.BigEndian.Uint16(buf[offset+4 : offset+6])
		if replyID != id {
			return false, 0
		}
	} else {
		// IPv6: 直接从 ICMPv6 开始（type == 129 Echo Reply）
		if buf[0] != 129 {
			return false, 0
		}
	}

	return true, latency
}

// buildICMPEcho 构造 ICMP Echo Request 消息（类型 8，代码 0）
func buildICMPEcho(id, seq uint16) []byte {
	msg := make([]byte, 16)
	msg[0] = 8 // Type: Echo Request
	msg[1] = 0 // Code
	binary.BigEndian.PutUint16(msg[4:6], id)
	binary.BigEndian.PutUint16(msg[6:8], seq)
	copy(msg[8:], []byte("aegis-ping"))

	// 计算校验和
	cs := checksum(msg)
	binary.BigEndian.PutUint16(msg[2:4], cs)
	return msg
}

func checksum(data []byte) uint16 {
	var sum uint32
	for i := 0; i+1 < len(data); i += 2 {
		sum += uint32(binary.BigEndian.Uint16(data[i : i+2]))
	}
	if len(data)%2 != 0 {
		sum += uint32(data[len(data)-1]) << 8
	}
	for sum>>16 != 0 {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	return ^uint16(sum)
}

// icmpFallbackTCP 当无 raw socket 权限时，用 TCP 连通性探测替代（连常见端口）
func icmpFallbackTCP(addr string, timeout time.Duration) (bool, int64) {
	ports := []string{"80", "443", "22", "8080"}
	for _, p := range ports {
		target := net.JoinHostPort(addr, p)
		start := time.Now()
		conn, err := net.DialTimeout("tcp", target, timeout/time.Duration(len(ports)))
		if err == nil {
			conn.Close()
			return true, time.Since(start).Milliseconds()
		}
	}
	return false, 0
}
