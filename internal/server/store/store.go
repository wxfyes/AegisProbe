package store

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"tianque_probe/internal/common"
)

type NodeState struct {
	NodeID          string
	LastSeen        time.Time
	LatestMetrics   common.MetricsData
	ConsecutiveHits int       // 连续成功上报次数 (用于防抖恢复)
	WasOnline       bool      // 记录上一次扫描时的在线状态
	IPv4            string    // 记忆的最新公网 IPv4
	IPv6            string    // 记忆的最新公网 IPv6
	GFWStatus       string    // 国内测活状态: "ok" (畅通), "blocked" (被墙), "" (未知)
	GFWChecked      time.Time // 上次测活时间
}

type EventListener interface {
	OnNodeOffline(node common.NodeConfig, lastSeen time.Time)
	OnNodeOnline(node common.NodeConfig, metrics common.MetricsData)
}

type Store struct {
	mu        sync.RWMutex
	states    map[string]*NodeState
	listeners []EventListener
}

func NewStore() *Store {
	return &Store{
		states:    make(map[string]*NodeState),
		listeners: make([]EventListener, 0),
	}
}

func (s *Store) AddListener(l EventListener) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listeners = append(s.listeners, l)
}

// UpdateReport 接收到 Agent 单向推送 (增量合并双栈 IP，永远不丢失)
func (s *Store) UpdateReport(nodeID string, metrics common.MetricsData) {
	s.mu.Lock()
	defer s.mu.Unlock()

	st, exists := s.states[nodeID]
	now := time.Now()
	if !exists {
		st = &NodeState{
			NodeID:          nodeID,
			LastSeen:        now,
			ConsecutiveHits: 1,
			WasOnline:       false,
		}
		s.states[nodeID] = st
	} else {
		st.LastSeen = now
		st.ConsecutiveHits++
	}

	// 增量合并 IPv4 与 IPv6
	if metrics.IPv4 != "" {
		st.IPv4 = metrics.IPv4
	}
	if metrics.IPv6 != "" {
		st.IPv6 = metrics.IPv6
	}
	if metrics.PublicIP != "" {
		if strings.Contains(metrics.PublicIP, ":") && st.IPv6 == "" {
			st.IPv6 = metrics.PublicIP
		} else if strings.Contains(metrics.PublicIP, ".") && st.IPv4 == "" {
			st.IPv4 = metrics.PublicIP
		}
	}

	metrics.IPv4 = st.IPv4
	metrics.IPv6 = st.IPv6
	if metrics.PublicIP == "" {
		if st.IPv4 != "" {
			metrics.PublicIP = st.IPv4
		} else {
			metrics.PublicIP = st.IPv6
		}
	}

	st.LatestMetrics = metrics
}

// UpdateGFWStatus 更新节点国内测活状态
func (s *Store) UpdateGFWStatus(nodeID string, status string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if st, exists := s.states[nodeID]; exists {
		st.GFWStatus = status
		st.GFWChecked = time.Now()
	}
}

// GetNodeRealInfo 获取节点的双栈 IP (IPv4, IPv6)、真实操作系统以及在线状态
func (s *Store) GetNodeRealInfo(nodeID string) (ipv4 string, ipv6 string, os string, isOnline bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st, exists := s.states[nodeID]
	if !exists {
		return "", "", "", false
	}
	isOnline = time.Since(st.LastSeen) <= 60*time.Second
	return st.IPv4, st.IPv6, st.LatestMetrics.OS, isOnline
}

// CheckHeartbeats 定时扫描节点心跳状态，触发掉线或上线恢复
func (s *Store) CheckHeartbeats(nodes []common.NodeConfig, offlineThreshold time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	nodeMap := make(map[string]common.NodeConfig, len(nodes))
	for _, n := range nodes {
		nodeMap[n.ID] = n
	}

	for _, node := range nodes {
		st, exists := s.states[node.ID]
		if !exists {
			continue
		}

		isAlive := now.Sub(st.LastSeen) <= offlineThreshold

		// 触发掉线
		if !isAlive && st.WasOnline {
			st.WasOnline = false
			st.ConsecutiveHits = 0
			for _, l := range s.listeners {
				l.OnNodeOffline(node, st.LastSeen)
			}
		}

		// 触发上线 (连续上报 >= 2 次才算稳定上线，防止抖动)
		if isAlive && !st.WasOnline && st.ConsecutiveHits >= 2 {
			st.WasOnline = true
			for _, l := range s.listeners {
				l.OnNodeOnline(node, st.LatestMetrics)
			}
		}
	}
}

// GetSummary 构造供大屏展示的完全脱敏聚合模型
func (s *Store) GetSummary(siteTitle string, nodes []common.NodeConfig, offlineSec int) common.SummaryResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now()
	timeoutDur := time.Duration(offlineSec) * time.Second

	var totalNetOut, totalNetIn uint64
	var instantSpeedOut, instantSpeedIn uint64
	onlineCount := 0
	regionSet := make(map[string]struct{})
	groupSet := make(map[string]struct{})

	cardViews := make([]common.NodeCardView, 0, len(nodes))

	for _, n := range nodes {
		if n.Region != "" {
			regionSet[strings.ToUpper(n.Region)] = struct{}{}
		}
		if n.Group != "" {
			groupSet[n.Group] = struct{}{}
		}

		st, hasState := s.states[n.ID]
		isOnline := false
		var m common.MetricsData
		var lastSeenSec int64 = 999999

		if hasState {
			lastSeenSec = int64(now.Sub(st.LastSeen).Seconds())
			if now.Sub(st.LastSeen) <= timeoutDur {
				isOnline = true
				m = st.LatestMetrics
			}
		}

		if isOnline {
			onlineCount++
			totalNetOut += m.NetOutTotal
			totalNetIn += m.NetInTotal
			instantSpeedOut += m.NetOutSpeed
			instantSpeedIn += m.NetInSpeed
		}

		// 计算计费到期剩余天数
		daysLeft := calcDaysLeft(n.ExpireDate)

		// 计算规格字符串
		ramStr := formatBytes(m.MemoryTotal)
		diskStr := formatBytes(m.DiskTotal)
		if m.MemoryTotal == 0 {
			ramStr = "--"
		}
		if m.DiskTotal == 0 {
			diskStr = "--"
		}

		// 计算百分比
		var ramPct, diskPct, swapPct float64
		if m.MemoryTotal > 0 {
			ramPct = math.Round(float64(m.MemoryUsed)/float64(m.MemoryTotal)*1000) / 10
		}
		if m.DiskTotal > 0 {
			diskPct = math.Round(float64(m.DiskUsed)/float64(m.DiskTotal)*1000) / 10
		}
		swapOff := (m.SwapTotal == 0)
		if !swapOff {
			swapPct = math.Round(float64(m.SwapUsed)/float64(m.SwapTotal)*1000) / 10
		}

		// 格式化负载
		loadStr := fmt.Sprintf("%.2f | %.2f | %.2f", m.Load1, m.Load5, m.Load15)

		// 格式化在线时长
		uptimeStr := formatUptime(m.UptimeSeconds)

		// 优先使用 Agent 真实采集上报的发行版系统，杜绝写死配置错误
		finalOS := strings.ToLower(strings.TrimSpace(m.OS))
		if finalOS == "" {
			finalOS = strings.ToLower(strings.TrimSpace(n.OS))
		}
		if finalOS == "" {
			finalOS = "linux"
		}

		// 智能识别国旗：自动将两位代码转为真正的国旗 Emoji (如 HK->🇭🇰, TW->🇹🇼, US->🇺🇸)
		finalRegion := strings.ToUpper(strings.TrimSpace(n.Region))
		if m.CountryCode != "" {
			finalRegion = strings.ToUpper(strings.TrimSpace(m.CountryCode))
		}
		if finalRegion == "" {
			finalRegion = "GLOBAL"
		}

		finalFlag := strings.TrimSpace(n.Flag)
		if m.CountryCode != "" {
			finalFlag = common.CountryCodeToFlag(m.CountryCode)
		} else if finalFlag != "" {
			finalFlag = common.CountryCodeToFlag(finalFlag)
		} else if finalRegion != "" {
			finalFlag = common.CountryCodeToFlag(finalRegion)
		} else {
			finalFlag = "🌐"
		}

		card := common.NodeCardView{
			ID:     n.ID,
			Name:   n.Name,
			Region: finalRegion,
			Flag:   finalFlag,
			OS:     finalOS,
			Group:  n.Group,
			Billing: common.NodeBillingView{
				Price:      n.Price,
				DaysLeft:   daysLeft,
				ExpireDate: n.ExpireDate,
			},
			Hardware: common.NodeHardwareView{
				CPUCores: m.CPUCores,
				RAMStr:   ramStr,
				DiskStr:  diskStr,
			},
			Status: common.NodeStatusView{
				IsOnline:       isOnline,
				CPUPct:         math.Round(m.CPUPercent*10) / 10,
				RAMPct:         ramPct,
				SwapPct:        swapPct,
				SwapOff:        swapOff,
				DiskPct:        diskPct,
				SpeedUpStr:     formatSpeed(m.NetOutSpeed),
				SpeedDownStr:   formatSpeed(m.NetInSpeed),
				TrafficUpStr:   formatBytes(m.NetOutTotal),
				TrafficDownStr: formatBytes(m.NetInTotal),
				LoadStr:        loadStr,
				UptimeStr:      uptimeStr,
				LastSeenSec:    lastSeenSec,
			},
			GFWStatus: func() string {
				if hasState && isOnline {
					return st.GFWStatus
				}
				return ""
			}(),
		}

		cardViews = append(cardViews, card)
	}

	// 整理分组列表，保证“所有”排第一
	groups := []string{"所有"}
	var customGroups []string
	for g := range groupSet {
		if g != "所有" && g != "" {
			customGroups = append(customGroups, g)
		}
	}
	sort.Strings(customGroups)
	groups = append(groups, customGroups...)

	return common.SummaryResponse{
		SiteTitle: siteTitle,
		Summary: common.OverviewSummary{
			CurrentTime:      now.Format("15:04:05"),
			OnlineCount:      onlineCount,
			TotalCount:       len(nodes),
			RegionCount:      len(regionSet),
			TotalNetOutBytes: totalNetOut,
			TotalNetInBytes:  totalNetIn,
			InstantSpeedOut:  instantSpeedOut,
			InstantSpeedIn:   instantSpeedIn,
		},
		Groups: groups,
		Nodes:  cardViews,
	}
}

func calcDaysLeft(dateStr string) int {
	if dateStr == "" {
		return 999
	}
	dateStr = strings.ReplaceAll(dateStr, "-", "/")
	t, err := time.Parse("2006/01/02", dateStr)
	if err != nil {
		return 999
	}
	today := time.Now()
	todayZero := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())
	targetZero := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, today.Location())

	diff := targetZero.Sub(todayZero)
	return int(diff.Hours() / 24)
}

func formatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	val := float64(b) / float64(div)
	switch exp {
	case 0:
		return fmt.Sprintf("%.2f KB", val)
	case 1:
		return fmt.Sprintf("%.2f MB", val)
	case 2:
		return fmt.Sprintf("%.2f GB", val)
	case 3:
		return fmt.Sprintf("%.2f TB", val)
	default:
		return fmt.Sprintf("%.2f PB", val)
	}
}

func formatSpeed(b uint64) string {
	return formatBytes(b) + "/s"
}

func formatUptime(seconds uint64) string {
	if seconds == 0 {
		return "刚刚启动"
	}
	days := seconds / 86400
	hours := (seconds % 86400) / 3600
	mins := (seconds % 3600) / 60
	if days > 0 {
		return fmt.Sprintf("在线: %d天%d小时", days, hours)
	}
	if hours > 0 {
		return fmt.Sprintf("在线: %d小时%d分", hours, mins)
	}
	return fmt.Sprintf("在线: %d分钟", mins)
}
