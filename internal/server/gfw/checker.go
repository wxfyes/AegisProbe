package gfw

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"tianque_probe/internal/server/config"
	"tianque_probe/internal/server/store"
)

type Checker struct {
	cfgMgr *config.Manager
	store  *store.Store
	client *http.Client
}

func NewChecker(cfgMgr *config.Manager, st *store.Store) *Checker {
	return &Checker{
		cfgMgr: cfgMgr,
		store:  st,
		client: &http.Client{Timeout: 8 * time.Second},
	}
}

// Start 启动后台国内测活循环 (完全异步轻量，零阻塞)
func (c *Checker) Start(ctx context.Context) {
	go func() {
		log.Println("[国内测活] 国内连通性巡检引擎已就绪...")
		// 启动后延迟 5 秒执行初次检测，确保 Agent 先完成上线汇报
		select {
		case <-time.After(5 * time.Second):
			c.checkAll()
		case <-ctx.Done():
			return
		}

		for {
			cfg := c.cfgMgr.GetConfig()
			intervalSec := cfg.GFWCheckInterval
			if intervalSec <= 0 {
				intervalSec = 14400 // 默认 4 小时 (14400 秒)
			}

			select {
			case <-time.After(time.Duration(intervalSec) * time.Second):
				c.checkAll()
			case <-ctx.Done():
				return
			}
		}
	}()
}

// TriggerCheckNow 手动立即触发一轮国内连通性测活
func (c *Checker) TriggerCheckNow() {
	go c.checkAll()
}

// checkAll 执行一轮测活
func (c *Checker) checkAll() {
	cfg := c.cfgMgr.GetConfig()
	templateURL := strings.TrimSpace(cfg.GFWCheckURL)
	if templateURL == "" {
		return
	}

	for _, node := range cfg.Nodes {
		ipv4, ipv6, _, _, isOnline := c.store.GetNodeRealInfo(node.ID)
		if !isOnline {
			continue
		}

		targetIP := ipv4
		if targetIP == "" {
			targetIP = ipv6
		}
		if targetIP == "" {
			continue
		}

		port := node.ProbePort
		if port <= 0 {
			port = 22
		}

		// 执行单点探测 (平滑请求节奏，防止触发外部 API 频率限制)
		go c.probeNode(node.ID, targetIP, port, templateURL)
		time.Sleep(600 * time.Millisecond)
	}
}

var reBracedNum = regexp.MustCompile(`\{(\d+)\}`)
var rePingLatency = regexp.MustCompile(`(?i)^\s*\d+(?:\.\d+)?\s*(?:ms|毫秒)?\s*$`)

func (c *Checker) probeNode(nodeID string, ip string, port int, templateURL string) {
	// 容错处理：若用户手写了类似 {80} 或 {22}，自动移除大括号纠正为数字
	cleanedTemplate := reBracedNum.ReplaceAllString(templateURL, "$1")

	url := strings.ReplaceAll(cleanedTemplate, "{ip}", ip)
	url = strings.ReplaceAll(url, "{port}", strconv.Itoa(port))

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		log.Printf("[国内测活] 节点 %s 构建请求失败: %v", nodeID, err)
		c.store.UpdateGFWStatus(nodeID, "blocked")
		return
	}
	req.Header.Set("User-Agent", "TianQue-Probe/1.0")

	resp, err := c.client.Do(req)
	if err != nil {
		log.Printf("[国内测活] 节点 %s 请求接口失败: %v", nodeID, err)
		c.store.UpdateGFWStatus(nodeID, "blocked")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[国内测活] 节点 %s 接口返回非200状态码: %d", nodeID, resp.StatusCode)
		c.store.UpdateGFWStatus(nodeID, "blocked")
		return
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*32))
	if err != nil {
		c.store.UpdateGFWStatus(nodeID, "blocked")
		return
	}

	isPass := parseAPIResult(body)
	log.Printf("[国内测活] 节点: %s | 目标: %s:%d | 判定: %v | 接口响应: %s", nodeID, ip, port, isPass, string(body))

	if isPass {
		c.store.UpdateGFWStatus(nodeID, "ok")
	} else {
		c.store.UpdateGFWStatus(nodeID, "blocked")
	}
}

// parseAPIResult 只在响应明确表示 TCP 端口连通时返回 true。
// API 的 code=200 通常只表示 API 请求成功，不能用来判断目标端口开放。
func parseAPIResult(body []byte) bool {
	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		// 非结构化响应无法可靠区分 API 成功与 TCP 探测成功，按未确认处理。
		return false
	}

	// xxapi 的 code=200 / msg=数据请求成功只代表 API 调用成功；
	// 只有 data.ping 返回有效延迟值，才代表 TCP 建连成功。
	if dataVal, exists := raw["data"]; exists {
		if dataMap, ok := dataVal.(map[string]interface{}); ok {
			if ping, hasPing := dataMap["ping"]; hasPing {
				if latency, ok := ping.(string); ok && rePingLatency.MatchString(latency) {
					return true
				}
				if latency, ok := ping.(float64); ok && latency >= 0 {
					return true
				}
				return false
			}
			for _, key := range []string{"tcp_open", "port_open", "connected", "open"} {
				if value, exists := dataMap[key]; exists {
					if result, ok := value.(bool); ok {
						return result
					}
				}
			}
			if status, ok := dataMap["status"].(string); ok {
				switch strings.ToLower(strings.TrimSpace(status)) {
				case "open", "connected", "reachable":
					return true
				}
			}
		}
	}

	// 自建探测接口可用明确的 TCP 状态字段；不要把通用 success/code 当作端口结果。
	for _, key := range []string{"tcp_open", "port_open", "connected"} {
		if value, exists := raw[key]; exists {
			if result, ok := value.(bool); ok {
				return result
			}
		}
	}
	if status, ok := raw["status"].(string); ok {
		switch strings.ToLower(strings.TrimSpace(status)) {
		case "open", "connected", "reachable":
			return true
		}
	}

	return false
}
