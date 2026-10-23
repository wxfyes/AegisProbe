package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"tianque_probe/internal/common"
)

type Notifier struct {
	mu           sync.Mutex
	msgChan      chan string
	client       *http.Client
	botToken     string
	chatID       string
	enabled      bool
	warnedDates  map[string]bool // 防重复提醒映射表: "nodeID_warnDay_YYYYMMDD"
}

func NewNotifier() *Notifier {
	n := &Notifier{
		msgChan:     make(chan string, 128),
		warnedDates: make(map[string]bool),
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
	return n
}

// UpdateConfig 动态更新 TG 推送参数
func (n *Notifier) UpdateConfig(enabled bool, token, chatID, proxyURLStr string) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.enabled = enabled
	n.botToken = strings.TrimSpace(token)
	n.chatID = strings.TrimSpace(chatID)

	transport := &http.Transport{}
	if proxyURLStr != "" {
		if pURL, err := url.Parse(proxyURLStr); err == nil {
			transport.Proxy = http.ProxyURL(pURL)
		} else {
			log.Printf("[TG告警] 代理地址解析失败: %v", err)
		}
	}
	n.client = &http.Client{
		Timeout:   10 * time.Second,
		Transport: transport,
	}
}

// Start 启动后台异步消费循环
func (n *Notifier) Start(ctx context.Context) {
	log.Printf("[TG告警] 异步告警消息中枢就绪...")
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-n.msgChan:
			n.sendMessage(msg)
		}
	}
}

func (n *Notifier) Push(msg string) {
	n.mu.Lock()
	enabled := n.enabled
	token := n.botToken
	chatID := n.chatID
	n.mu.Unlock()

	if !enabled || token == "" || chatID == "" {
		return
	}

	select {
	case n.msgChan <- msg:
	default:
		log.Printf("[TG告警] 队列已满，丢弃告警消息: %s", msg)
	}
}

func (n *Notifier) sendMessage(text string) {
	n.mu.Lock()
	token := n.botToken
	chatID := n.chatID
	client := n.client
	n.mu.Unlock()

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
	payload := map[string]interface{}{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "HTML",
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return
	}

	req, err := http.NewRequest(http.MethodPost, apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[TG告警] 发送失败: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[TG告警] 发送响应异常 HTTP: %d", resp.StatusCode)
	}
}

// OnNodeOffline 触发掉线报警
func (n *Notifier) OnNodeOffline(node common.NodeConfig, lastSeen time.Time) {
	nowStr := time.Now().Format("2006-01-02 15:04:05")
	msg := fmt.Sprintf(
		"🔴 <b>【节点掉线报警】</b>\n"+
			"<b>节点名称</b>：%s\n"+
			"<b>节点分组</b>：%s\n"+
			"<b>离线时间</b>：%s\n"+
			"<b>告警说明</b>：已超过 60 秒未收到单向上报心跳，请尽快排查服务器网络或状态！",
		node.Name, node.Group, nowStr,
	)
	n.Push(msg)
}

// OnNodeOnline 触发上线恢复通知
func (n *Notifier) OnNodeOnline(node common.NodeConfig, metrics common.MetricsData) {
	nowStr := time.Now().Format("2006-01-02 15:04:05")
	var ramPct float64
	if metrics.MemoryTotal > 0 {
		ramPct = float64(metrics.MemoryUsed) / float64(metrics.MemoryTotal) * 100.0
	}
	msg := fmt.Sprintf(
		"🟢 <b>【节点上线恢复】</b>\n"+
			"<b>节点名称</b>：%s\n"+
			"<b>节点分组</b>：%s\n"+
			"<b>恢复时间</b>：%s\n"+
			"<b>系统状态</b>：负载 %.2f | 内存 %.1f%%",
		node.Name, node.Group, nowStr, metrics.Load1, ramPct,
	)
	n.Push(msg)
}

// CheckExpiringNodes 定时检查节点账单到期
func (n *Notifier) CheckExpiringNodes(nodes []common.NodeConfig, warnDays []int) {
	n.mu.Lock()
	defer n.mu.Unlock()

	today := time.Now()
	todayKey := today.Format("20060102")

	for _, node := range nodes {
		if node.ExpireDate == "" {
			continue
		}
		cleanDate := strings.ReplaceAll(node.ExpireDate, "-", "/")
		t, err := time.Parse("2006/01/02", cleanDate)
		if err != nil {
			continue
		}

		todayZero := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())
		targetZero := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, today.Location())
		diffDays := int(targetZero.Sub(todayZero).Hours() / 24)

		for _, wDay := range warnDays {
			if diffDays == wDay {
				warnKey := fmt.Sprintf("%s_%d_%s", node.ID, wDay, todayKey)
				if !n.warnedDates[warnKey] {
					n.warnedDates[warnKey] = true
					
					var dayDesc string
					if diffDays > 0 {
						dayDesc = fmt.Sprintf("仅剩 %d 天", diffDays)
					} else {
						dayDesc = "⚠️ 今日到期！"
					}

					msg := fmt.Sprintf(
						"⚠️ <b>【节点到期提醒】</b>\n"+
							"<b>节点名称</b>：%s\n"+
							"<b>当前计费</b>：%s\n"+
							"<b>到期时间</b>：%s (%s)\n"+
							"<b>友情提示</b>：请注意及时续费，防止机器停机造成代理中转业务中断！",
						node.Name, node.Price, node.ExpireDate, dayDesc,
					)
					go n.Push(msg)
				}
			}
		}
	}
}
