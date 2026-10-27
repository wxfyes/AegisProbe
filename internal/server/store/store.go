package store

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
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
	mu               sync.RWMutex
	states           map[string]*NodeState
	nodeStatePath    string
	nodeStateDirty   bool
	routeQuality     map[string]map[string]common.CarrierQuality
	routeHistory     map[string]map[string][]common.RouteQualityHistoryPoint
	routeHistoryPath string
	listeners        []EventListener
}

type persistedNodeState struct {
	LastSeen time.Time          `json:"last_seen"`
	Metrics  common.MetricsData `json:"metrics"`
}

func NewStore() *Store {
	return &Store{
		states:       make(map[string]*NodeState),
		routeQuality: make(map[string]map[string]common.CarrierQuality),
		routeHistory: make(map[string]map[string][]common.RouteQualityHistoryPoint),
		listeners:    make([]EventListener, 0),
	}
}

// SetNodeStatePath restores the latest Agent snapshots and configures their durable cache.
func (s *Store) SetNodeStatePath(path string, offlineSec int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nodeStatePath = path
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("读取节点状态快照失败: %v", err)
		}
		return
	}
	var saved map[string]persistedNodeState
	if err := json.Unmarshal(data, &saved); err != nil {
		log.Printf("解析节点状态快照失败: %v", err)
		return
	}
	if offlineSec <= 0 {
		offlineSec = 60
	}
	for nodeID, snapshot := range saved {
		if strings.TrimSpace(nodeID) == "" || snapshot.LastSeen.IsZero() {
			continue
		}
		wasOnline := time.Since(snapshot.LastSeen) <= time.Duration(offlineSec)*time.Second
		consecutiveHits := 0
		if wasOnline {
			consecutiveHits = 2
		}
		s.states[nodeID] = &NodeState{
			NodeID:          nodeID,
			LastSeen:        snapshot.LastSeen,
			LatestMetrics:   snapshot.Metrics,
			ConsecutiveHits: consecutiveHits,
			WasOnline:       wasOnline,
			IPv4:            snapshot.Metrics.IPv4,
			IPv6:            snapshot.Metrics.IPv6,
		}
	}
}

// SaveNodeStates writes dirty last-report snapshots at a low frequency, not on every heartbeat.
func (s *Store) SaveNodeStates() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.nodeStatePath == "" || !s.nodeStateDirty {
		return nil
	}
	snapshots := make(map[string]persistedNodeState, len(s.states))
	for nodeID, state := range s.states {
		snapshots[nodeID] = persistedNodeState{LastSeen: state.LastSeen, Metrics: state.LatestMetrics}
	}
	data, err := json.Marshal(snapshots)
	if err != nil {
		return fmt.Errorf("序列化节点状态快照失败: %w", err)
	}
	if err = os.MkdirAll(filepath.Dir(s.nodeStatePath), 0755); err != nil {
		return fmt.Errorf("创建节点状态目录失败: %w", err)
	}
	tmp := s.nodeStatePath + ".tmp"
	if err = os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("写入节点状态快照失败: %w", err)
	}
	if err = os.Rename(tmp, s.nodeStatePath); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("保存节点状态快照失败: %w", err)
	}
	s.nodeStateDirty = false
	return nil
}

// SetRouteQualityHistoryPath loads and persists the rolling 24-hour route history.
func (s *Store) SetRouteQualityHistoryPath(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.routeHistoryPath = path
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("读取三网质量历史失败: %v", err)
		}
		return
	}
	if err := json.Unmarshal(data, &s.routeHistory); err != nil {
		log.Printf("解析三网质量历史失败: %v", err)
		s.routeHistory = make(map[string]map[string][]common.RouteQualityHistoryPoint)
	}
	if s.routeHistory == nil {
		s.routeHistory = make(map[string]map[string][]common.RouteQualityHistoryPoint)
	}
	s.pruneRouteHistoryLocked(time.Now())
}

// UpdateRouteQuality stores one cached carrier measurement without touching agent heartbeat state.
func (s *Store) UpdateRouteQuality(nodeID string, quality common.CarrierQuality) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.routeQuality[nodeID] == nil {
		s.routeQuality[nodeID] = make(map[string]common.CarrierQuality)
	}
	s.routeQuality[nodeID][quality.Carrier] = quality
	if s.routeHistory[nodeID] == nil {
		s.routeHistory[nodeID] = make(map[string][]common.RouteQualityHistoryPoint)
	}
	s.routeHistory[nodeID][quality.Carrier] = append(s.routeHistory[nodeID][quality.Carrier], common.RouteQualityHistoryPoint{
		CheckedAt: quality.CheckedAt,
		LatencyMS: quality.LatencyMS,
		LossPct:   quality.LossPct,
		Status:    quality.Status,
	})
	s.pruneRouteHistoryLocked(time.Now())
	s.persistRouteHistoryLocked()
}

// GetRouteQualityHistory returns cloned samples from the last 24 hours by carrier.
func (s *Store) GetRouteQualityHistory(nodeID string) map[string][]common.RouteQualityHistoryPoint {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make(map[string][]common.RouteQualityHistoryPoint, 3)
	for carrier, points := range s.routeHistory[nodeID] {
		result[carrier] = append([]common.RouteQualityHistoryPoint(nil), points...)
	}
	return result
}

func (s *Store) pruneRouteHistoryLocked(now time.Time) {
	cutoff := now.Add(-24 * time.Hour)
	for nodeID, carriers := range s.routeHistory {
		for carrier, points := range carriers {
			kept := points[:0]
			for _, point := range points {
				checked, err := time.Parse(time.RFC3339, point.CheckedAt)
				if err == nil && !checked.Before(cutoff) && !checked.After(now.Add(time.Minute)) {
					kept = append(kept, point)
				}
			}
			if len(kept) == 0 {
				delete(carriers, carrier)
			} else {
				carriers[carrier] = kept
			}
		}
		if len(carriers) == 0 {
			delete(s.routeHistory, nodeID)
		}
	}
}

func (s *Store) persistRouteHistoryLocked() {
	if s.routeHistoryPath == "" {
		return
	}
	data, err := json.Marshal(s.routeHistory)
	if err != nil {
		log.Printf("序列化三网质量历史失败: %v", err)
		return
	}
	if err = os.MkdirAll(filepath.Dir(s.routeHistoryPath), 0755); err != nil {
		log.Printf("创建三网质量历史目录失败: %v", err)
		return
	}
	tmp := s.routeHistoryPath + ".tmp"
	if err = os.WriteFile(tmp, data, 0600); err != nil {
		log.Printf("写入三网质量历史失败: %v", err)
		return
	}
	if err = os.Rename(tmp, s.routeHistoryPath); err != nil {
		_ = os.Remove(tmp)
		log.Printf("保存三网质量历史失败: %v", err)
	}
}

func (s *Store) GetRouteQuality(nodeID string) []common.CarrierQuality {
	s.mu.RLock()
	defer s.mu.RUnlock()
	results := make([]common.CarrierQuality, 0, 3)
	for _, carrier := range []string{"电信", "联通", "移动"} {
		if quality, ok := s.routeQuality[nodeID][carrier]; ok {
			results = append(results, quality)
		}
	}
	return results
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
	s.nodeStateDirty = true
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

// GetNodeRealInfo 获取节点的双栈 IP (IPv4, IPv6)、真实操作系统、版本号以及在线状态
func (s *Store) GetNodeRealInfo(nodeID string) (ipv4 string, ipv6 string, os string, version string, isOnline bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st, exists := s.states[nodeID]
	if !exists {
		return "", "", "", "", false
	}
	isOnline = time.Since(st.LastSeen) <= 60*time.Second
	ver := st.LatestMetrics.Version
	if ver == "" && isOnline {
		ver = "v1.0.0"
	}
	return st.IPv4, st.IPv6, st.LatestMetrics.OS, ver, isOnline
}

func (s *Store) GetNodeCountryCode(nodeID string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if st, exists := s.states[nodeID]; exists {
		return strings.ToUpper(strings.TrimSpace(st.LatestMetrics.CountryCode))
	}
	return ""
}

func (s *Store) GetOnlineNodeIDs(nodes []common.NodeConfig, threshold time.Duration) map[string]bool {
	if threshold <= 0 {
		threshold = 60 * time.Second
	}
	now := time.Now()
	s.mu.RLock()
	defer s.mu.RUnlock()
	online := make(map[string]bool, len(nodes))
	for _, node := range nodes {
		if state, exists := s.states[node.ID]; exists && now.Sub(state.LastSeen) <= threshold {
			online[node.ID] = true
		}
	}
	return online
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
			m = st.LatestMetrics
			if now.Sub(st.LastSeen) <= timeoutDur {
				isOnline = true
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
		speedUpStr := formatSpeed(m.NetOutSpeed)
		speedDownStr := formatSpeed(m.NetInSpeed)

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
				SpeedUpStr:     speedUpStr,
				SpeedDownStr:   speedDownStr,
				TCPConnections: m.TCPConnections,
				UDPSockets:     m.UDPSockets,
				TrafficUpStr:   formatBytes(m.NetOutTotal),
				TrafficDownStr: formatBytes(m.NetInTotal),
				LoadStr:        loadStr,
				UptimeStr:      uptimeStr,
				LastSeenSec:    lastSeenSec,
				LastSeenAt:     lastSeenAt(st, hasState),
			},
			GFWStatus: func() string {
				if hasState && isOnline {
					return st.GFWStatus
				}
				return ""
			}(),
			RouteQuality: s.getRouteQualityLocked(n.ID),
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

func (s *Store) getRouteQualityLocked(nodeID string) []common.CarrierQuality {
	results := make([]common.CarrierQuality, 0, 3)
	for _, carrier := range []string{"电信", "联通", "移动"} {
		if quality, ok := s.routeQuality[nodeID][carrier]; ok {
			results = append(results, quality)
		}
	}
	return results
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

func lastSeenAt(state *NodeState, exists bool) string {
	if !exists || state == nil {
		return ""
	}
	return state.LastSeen.Format(time.RFC3339)
}
