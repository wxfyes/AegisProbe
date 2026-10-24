package main

// =============================================================================
// 🛡️ Aegis-Ping — 天阙·神盾家宽自建国内探活微服务
// 专为家庭宽带软路由 / 公网出口机设计。
// 体积 < 2MB，内存 < 2MB，零外部依赖，纯静态 Go 编译。
// 接口设计：
//   GET /ping?address={ip}&token={token}     → 纯 ICMP Ping（免端口依赖）
//   GET /tcping?address={ip}&port={port}&token={token} → TCP 三次握手
//   GET /health                              → 健康探活（无需 Token）
// =============================================================================

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// ─── 版本 ───────────────────────────────────────────────────────────────────
const version = "v1.0.0"

// ─── 全局配置（由命令行 flag 注入）─────────────────────────────────────────
var (
	listenAddr string
	token      string
	verbose    bool
)

// ─── 统一响应结构 ────────────────────────────────────────────────────────────
type Result struct {
	Code    int         `json:"code"` // 200 = 成功，-1 = 失败，-2 = 被拒/超时
	Msg     string      `json:"msg"`
	Data    interface{} `json:"data,omitempty"`
}

type PingData struct {
	Address string `json:"address"`
	Latency string `json:"ping"`
}

func writeJSON(w http.ResponseWriter, code int, r Result) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(r)
}

// ─── Token 校验中间件 ────────────────────────────────────────────────────────
func checkToken(r *http.Request) bool {
	if token == "" {
		return true // 未设置 token，开放访问（内网模式）
	}
	t := r.URL.Query().Get("token")
	if t == "" {
		t = r.Header.Get("X-Token")
	}
	return t == token
}

// ─── /health ─────────────────────────────────────────────────────────────────
func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, Result{Code: 200, Msg: "Aegis-Ping OK", Data: map[string]string{
		"version": version,
		"time":    time.Now().Format("2006-01-02 15:04:05"),
	}})
}

// ─── /ping  —  纯 ICMP Ping ─────────────────────────────────────────────────
// Linux 软路由通常允许 root / CAP_NET_RAW 进程直接发 ICMP 原始套接字。
// 此处采用"特权 ICMP 拨号"方式，走系统内核 ping，对 OpenWrt/iStoreOS 完全兼容。
func handlePing(w http.ResponseWriter, r *http.Request) {
	if !checkToken(r) {
		writeJSON(w, 403, Result{Code: 403, Msg: "Unauthorized"})
		return
	}

	addr := strings.TrimSpace(r.URL.Query().Get("address"))
	if addr == "" {
		writeJSON(w, 400, Result{Code: -1, Msg: "missing address parameter"})
		return
	}
	// 基础 IP 合法性校验
	if net.ParseIP(addr) == nil {
		// 尝试解析域名
		ips, err := net.LookupHost(addr)
		if err != nil || len(ips) == 0 {
			writeJSON(w, 400, Result{Code: -1, Msg: "invalid or unresolvable address"})
			return
		}
		addr = ips[0]
	}

	if verbose {
		log.Printf("[ping] probing %s", addr)
	}

	start := time.Now()
	ok, latencyMs := icmpPing(addr, 4*time.Second)
	elapsed := time.Since(start)
	_ = elapsed

	if ok {
		writeJSON(w, 200, Result{
			Code: 200,
			Msg:  "成功",
			Data: PingData{
				Address: addr,
				Latency: fmt.Sprintf("%dms", latencyMs),
			},
		})
		log.Printf("📡 [ICMP Ping] 目标: %-15s | 结果: 畅通 ✓ | 延迟: %dms", addr, latencyMs)
	} else {
		writeJSON(w, 200, Result{Code: -2, Msg: "失败", Data: []struct{}{}})
		log.Printf("⚠️ [ICMP Ping] 目标: %-15s | 结果: 超时/阻断 ✕", addr)
	}
}

// ─── /tcping  —  TCP 三次握手 ────────────────────────────────────────────────
func handleTCPing(w http.ResponseWriter, r *http.Request) {
	if !checkToken(r) {
		writeJSON(w, 403, Result{Code: 403, Msg: "Unauthorized"})
		return
	}

	addr := strings.TrimSpace(r.URL.Query().Get("address"))
	portStr := strings.TrimSpace(r.URL.Query().Get("port"))

	if addr == "" || portStr == "" {
		writeJSON(w, 400, Result{Code: -1, Msg: "missing address or port"})
		return
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		writeJSON(w, 400, Result{Code: -1, Msg: "invalid port"})
		return
	}

	target := net.JoinHostPort(addr, portStr)

	start := time.Now()
	conn, err := net.DialTimeout("tcp", target, 6*time.Second)
	latencyMs := time.Since(start).Milliseconds()

	if err != nil {
		writeJSON(w, 200, Result{Code: -2, Msg: "失败", Data: []struct{}{}})
		log.Printf("⚠️ [TCPing] 目标: %-21s | 结果: 失败 ✕ (%v)", target, err)
		return
	}
	conn.Close()

	writeJSON(w, 200, Result{
		Code: 200,
		Msg:  "成功",
		Data: PingData{
			Address: addr,
			Latency: fmt.Sprintf("%dms", latencyMs),
		},
	})
	log.Printf("📡 [TCPing] 目标: %-21s | 结果: 畅通 ✓ | 延迟: %dms", target, latencyMs)
}

// ─── 入口 ────────────────────────────────────────────────────────────────────
func main() {
	flag.StringVar(&listenAddr, "addr", ":38921", "监听地址与端口 (默认 :38921)")
	flag.StringVar(&token, "token", "", "访问 Token（为空则无鉴权，适合纯内网）")
	flag.BoolVar(&verbose, "v", false, "开启详细日志（调试模式）")
	showVersion := flag.Bool("version", false, "显示版本号")
	flag.Parse()

	if *showVersion {
		fmt.Printf("Aegis-Ping %s (TianQue Security Lab)\n", version)
		os.Exit(0)
	}

	if token == "" {
		log.Println("⚠️  [警告] 未设置 Token，任何人均可调用探测接口！建议生产环境加上 -token=你的密钥")
	} else {
		log.Printf("✅ Token 鉴权已启用（Token: %s...）", token[:min(4, len(token))])
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/ping", handlePing)
	mux.HandleFunc("/tcping", handleTCPing)
	// 兼容 xxapi 风格 URL 路径
	mux.HandleFunc("/api/ping", handlePing)
	mux.HandleFunc("/api/tcping", handleTCPing)

	srv := &http.Server{
		Addr:         listenAddr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	log.Printf("🛡️  Aegis-Ping %s 启动中...", version)
	log.Printf("👉 监听地址: %s", listenAddr)
	if token != "" {
		log.Printf("💡 ICMP Ping 接口: http://[公网IP]%s/ping?address={ip}&token=%s", listenAddr, token)
		log.Printf("💡 TCPing 接口  : http://[公网IP]%s/tcping?address={ip}&port={port}&token=%s", listenAddr, token)
	} else {
		log.Printf("💡 ICMP Ping 接口: http://[公网IP]%s/ping?address={ip}", listenAddr)
		log.Printf("💡 TCPing 接口  : http://[公网IP]%s/tcping?address={ip}&port={port}", listenAddr)
	}
	log.Printf("💡 兼容 AegisProbe 主控测活 URL 模板: http://[软路由公网IP]%s/ping?address={ip}&token=%s", listenAddr, token)

	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("启动失败: %v", err)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
