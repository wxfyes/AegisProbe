package reporter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"tianque_probe/internal/agent/collector"
	"tianque_probe/internal/common"
)

type Config struct {
	ServerURL string        // 探针服务端地址 (如 http://1.2.3.4:8989)
	NodeID    string        // 节点 ID
	Token     string        // 接入认证 Token
	Interval  time.Duration // 上报周期，默认 2 秒
}

type Reporter struct {
	cfg       Config
	client    *http.Client
	collector collector.SystemCollector
}

func NewReporter(cfg Config) *Reporter {
	if cfg.Interval <= 0 {
		cfg.Interval = 2 * time.Second
	}
	cfg.ServerURL = strings.TrimRight(cfg.ServerURL, "/")

	return &Reporter{
		cfg: cfg,
		client: &http.Client{
			Timeout: 4 * time.Second,
		},
		collector: collector.NewCollector(),
	}
}

// Start 启动常驻单向汇报引擎
func (r *Reporter) Start(ctx context.Context) {
	log.Printf("[Agent] 启动天阙·神盾安全探针被控端...")
	log.Printf("[Agent] 节点ID: %s, 目标主控: %s, 采集频率: %v", r.cfg.NodeID, r.cfg.ServerURL, r.cfg.Interval)
	log.Printf("[Agent] 【安全铁律生效】单向推送模式已锁定，本地未开启任何入站端口，无远程执行通道。")

	ticker := time.NewTicker(r.cfg.Interval)
	defer ticker.Stop()

	// 启动立即初次上报
	r.reportOnce()

	for {
		select {
		case <-ctx.Done():
			log.Printf("[Agent] 接收到退出信号，探针停止运行。")
			return
		case <-ticker.C:
			r.reportOnce()
		}
	}
}

func (r *Reporter) reportOnce() {
	metrics, err := r.collector.Collect()
	if err != nil {
		log.Printf("[Agent] 指标采集异常: %v", err)
		return
	}

	payload := common.ReportPayload{
		NodeID:    r.cfg.NodeID,
		AuthToken: r.cfg.Token,
		Timestamp: time.Now().Unix(),
		Metrics:   *metrics,
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return
	}

	targetURL := fmt.Sprintf("%s/api/report", r.cfg.ServerURL)
	req, err := http.NewRequest(http.MethodPost, targetURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "TianQue-AegisProbe/1.0")

	resp, err := r.client.Do(req)
	if err != nil {
		log.Printf("[Agent] 上报通信警告: 无法连接至主控端 %s (%v)", targetURL, err)
		return
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body) // 纯粹丢弃响应，彻底不接收任何远程指令

	if resp.StatusCode != http.StatusOK {
		log.Printf("[Agent] 上报状态码异常: %d (请检查 NodeID 与 Token 是否匹配)", resp.StatusCode)
	}
}
