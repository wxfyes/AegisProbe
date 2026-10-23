package gfw

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
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
		ipv4, ipv6, _, isOnline := c.store.GetNodeRealInfo(node.ID)
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

		// 执行单点探测 (并发受限，防止打满外部 API)
		go c.probeNode(node.ID, targetIP, port, templateURL)
		time.Sleep(300 * time.Millisecond) // 平滑请求节奏
	}
}

func (c *Checker) probeNode(nodeID string, ip string, port int, templateURL string) {
	url := strings.ReplaceAll(templateURL, "{ip}", ip)
	url = strings.ReplaceAll(url, "{port}", strconv.Itoa(port))

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		c.store.UpdateGFWStatus(nodeID, "blocked")
		return
	}
	req.Header.Set("User-Agent", "TianQue-Probe/1.0")

	resp, err := c.client.Do(req)
	if err != nil {
		c.store.UpdateGFWStatus(nodeID, "blocked")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.store.UpdateGFWStatus(nodeID, "blocked")
		return
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*32))
	if err != nil {
		c.store.UpdateGFWStatus(nodeID, "blocked")
		return
	}

	isPass := parseAPIResult(body)
	if isPass {
		c.store.UpdateGFWStatus(nodeID, "ok")
	} else {
		c.store.UpdateGFWStatus(nodeID, "blocked")
	}
}

// parseAPIResult 通用智能解析外部 API 探测结果 (支持 xxapi、自建 tcping 等各类格式)
func parseAPIResult(body []byte) bool {
	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		// 非 json，如包含成功则判为畅通
		content := strings.ToLower(string(body))
		return strings.Contains(content, "ok") || strings.Contains(content, "success") || strings.Contains(content, "true")
	}

	// 1. 匹配 code 字段
	if codeVal, exists := raw["code"]; exists {
		switch v := codeVal.(type) {
		case float64:
			if int(v) == 200 || int(v) == 0 {
				return true
			}
			return false
		case string:
			if v == "200" || v == "0" || strings.ToLower(v) == "success" {
				return true
			}
			return false
		}
	}

	// 2. 匹配 success / status 字段
	if successVal, exists := raw["success"]; exists {
		if b, ok := successVal.(bool); ok && b {
			return true
		}
	}
	if statusVal, exists := raw["status"]; exists {
		s := fmt.Sprintf("%v", statusVal)
		if strings.ToLower(s) == "ok" || strings.ToLower(s) == "success" {
			return true
		}
	}

	// 3. 匹配 data 内部是否有 ping
	if dataVal, exists := raw["data"]; exists {
		if dataMap, ok := dataVal.(map[string]interface{}); ok {
			if _, hasPing := dataMap["ping"]; hasPing {
				return true
			}
		}
	}

	return false
}
