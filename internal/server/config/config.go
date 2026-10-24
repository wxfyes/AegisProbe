package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"tianque_probe/internal/common"
)

type Manager struct {
	mu       sync.RWMutex
	filePath string
	cfg      common.ServerConfig
}

func NewManager(filePath string) (*Manager, error) {
	if filePath == "" {
		filePath = filepath.Join("data", "config.json")
	}

	m := &Manager{
		filePath: filePath,
	}

	if err := m.loadOrInit(); err != nil {
		return nil, err
	}

	return m, nil
}

func (m *Manager) loadOrInit() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	dir := filepath.Dir(m.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}

	data, err := os.ReadFile(m.filePath)
	if err == nil {
		var cfg common.ServerConfig
		if err := json.Unmarshal(data, &cfg); err == nil {
			m.cfg = cfg
			m.ensureDefaults()
			return nil
		}
	}

	// 初始化默认配置
	m.cfg = common.ServerConfig{
		ListenPort:    8989,
		AdminPassword: generateRandomSecret(10), // 初始随机高强度密码
		AccessMode:    "public",                 // 默认公开模式
		SiteTitle:     "天阙·神盾安全探针",
		TGEnabled:     false,
		TGBotToken:    "",
		TGChatID:      "",
		TGProxy:       "",
		OfflineSec:    60,
		WarnDays:      []int{7, 3, 1, 0},
		Nodes: []common.NodeConfig{
			{
				ID:         "node_01",
				Token:      generateRandomSecret(16),
				Name:       "香港-30M-122",
				Region:     "HK",
				Flag:       "🇭🇰",
				OS:         "debian",
				Group:      "中转",
				Price:      "$29.99/月",
				ExpireDate: time.Now().AddDate(0, 0, 13).Format("2006/01/02"),
				SortOrder:  1,
			},
			{
				ID:         "node_02",
				Token:      generateRandomSecret(16),
				Name:       "台湾-Hinet",
				Region:     "TW",
				Flag:       "🇹🇼",
				OS:         "ubuntu",
				Group:      "亚洲",
				Price:      "$350.00/月",
				ExpireDate: time.Now().AddDate(0, 0, 30).Format("2006/01/02"),
				SortOrder:  2,
			},
			{
				ID:         "node_03",
				Token:      generateRandomSecret(16),
				Name:       "香港转发-225",
				Region:     "HK",
				Flag:       "🇭🇰",
				OS:         "alpine",
				Group:      "中转",
				Price:      "$29.99/月",
				ExpireDate: time.Now().AddDate(0, 0, 30).Format("2006/01/02"),
				SortOrder:  3,
			},
		},
	}

	return m.saveLocked()
}

func (m *Manager) ensureDefaults() {
	if m.cfg.ListenPort <= 0 {
		m.cfg.ListenPort = 8989
	}
	if m.cfg.OfflineSec <= 0 {
		m.cfg.OfflineSec = 60
	}
	if m.cfg.AccessMode != "private" && m.cfg.AccessMode != "public" {
		m.cfg.AccessMode = "public"
	}
	if m.cfg.SiteTitle == "" {
		m.cfg.SiteTitle = "天阙·神盾安全探针"
	}
	if len(m.cfg.WarnDays) == 0 {
		m.cfg.WarnDays = []int{7, 3, 1, 0}
	}
	if m.cfg.GFWCheckInterval <= 0 {
		m.cfg.GFWCheckInterval = 180
	}
}

func (m *Manager) saveLocked() error {
	data, err := json.MarshalIndent(m.cfg, "", "  ")
	if err != nil {
		return err
	}

	tmpFile := m.filePath + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0600); err != nil {
		return err
	}

	return os.Rename(tmpFile, m.filePath)
}

func (m *Manager) GetConfig() common.ServerConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cfg
}

func (m *Manager) UpdateSettings(title, password, accessMode string, port, offlineSec int, tgEnabled bool, tgToken, tgChatID, tgProxy string, gfwURL string, gfwInterval int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if title != "" {
		m.cfg.SiteTitle = title
	}
	if password != "" {
		m.cfg.AdminPassword = password
	}
	if accessMode == "public" || accessMode == "private" {
		m.cfg.AccessMode = accessMode
	}
	if port > 0 {
		m.cfg.ListenPort = port
	}
	if offlineSec > 0 {
		m.cfg.OfflineSec = offlineSec
	}
	m.cfg.TGEnabled = tgEnabled
	m.cfg.TGBotToken = tgToken
	m.cfg.TGChatID = tgChatID
	m.cfg.TGProxy = tgProxy
	m.cfg.GFWCheckURL = strings.TrimSpace(gfwURL)
	if gfwInterval > 0 {
		m.cfg.GFWCheckInterval = gfwInterval
	} else if m.cfg.GFWCheckInterval <= 0 {
		m.cfg.GFWCheckInterval = 14400 // 默认 4 小时 (14400 秒)
	}

	return m.saveLocked()
}

func (m *Manager) ValidateNodeToken(nodeID, token string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, n := range m.cfg.Nodes {
		if n.ID == nodeID && n.Token == token {
			return true
		}
	}
	return false
}

func (m *Manager) GetNode(id string) (*common.NodeConfig, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, n := range m.cfg.Nodes {
		if n.ID == id {
			copy := n
			return &copy, true
		}
	}
	return nil, false
}

func (m *Manager) UpsertNode(node common.NodeConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if node.ID == "" {
		return fmt.Errorf("节点ID不可为空")
	}
	if node.Token == "" {
		node.Token = generateRandomSecret(16)
	}
	if node.Flag == "" {
		node.Flag = "🌐"
	}
	if node.OS == "" {
		node.OS = "linux"
	}
	if node.ProbePort <= 0 {
		node.ProbePort = 22
	}
	if strings.TrimSpace(node.RenewalPeriod) == "" {
		node.RenewalPeriod = common.InferRenewalPeriod(node.Price)
	} else {
		node.RenewalPeriod = common.NormalizeRenewalPeriod(node.RenewalPeriod)
	}

	found := false
	for i, n := range m.cfg.Nodes {
		if n.ID == node.ID {
			m.cfg.Nodes[i] = node
			found = true
			break
		}
	}
	if !found {
		m.cfg.Nodes = append(m.cfg.Nodes, node)
	}

	return m.saveLocked()
}

type AutoRenewalEvent struct {
	NodeName string
	Previous string
	Next     string
	Period   string
}

// RenewExpiredOnlineNodes extends an expired node only when auto-renew is enabled
// and its agent has reported within the configured online window.
func (m *Manager) RenewExpiredOnlineNodes(onlineNodes map[string]bool, now time.Time) ([]AutoRenewalEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	originalNodes := append([]common.NodeConfig(nil), m.cfg.Nodes...)
	events := make([]AutoRenewalEvent, 0)
	for i := range m.cfg.Nodes {
		node := &m.cfg.Nodes[i]
		if !node.AutoRenew || !onlineNodes[node.ID] || strings.TrimSpace(node.ExpireDate) == "" {
			continue
		}

		expiry, layout, ok := parseExpireDate(node.ExpireDate, now.Location())
		if !ok || !expiry.Before(today) {
			continue
		}

		period := node.RenewalPeriod
		if strings.TrimSpace(period) == "" {
			period = common.InferRenewalPeriod(node.Price)
		}
		period = common.NormalizeRenewalPeriod(period)
		next := addRenewalPeriod(today, period)
		previousDate := node.ExpireDate
		node.ExpireDate = next.Format(layout)
		node.RenewalPeriod = period
		events = append(events, AutoRenewalEvent{
			NodeName: node.Name,
			Previous: previousDate,
			Next:     node.ExpireDate,
			Period:   period,
		})
	}

	if len(events) == 0 {
		return nil, nil
	}
	if err := m.saveLocked(); err != nil {
		m.cfg.Nodes = originalNodes
		return nil, err
	}
	return events, nil
}

func parseExpireDate(value string, location *time.Location) (time.Time, string, bool) {
	for _, layout := range []string{"2006/01/02", "2006-01-02"} {
		parsed, err := time.ParseInLocation(layout, strings.TrimSpace(value), location)
		if err == nil {
			return parsed, layout, true
		}
	}
	return time.Time{}, "", false
}

func addRenewalPeriod(base time.Time, period string) time.Time {
	months := 1
	switch period {
	case common.RenewalPeriodQuarter:
		months = 3
	case common.RenewalPeriodHalfYear:
		months = 6
	case common.RenewalPeriodYear:
		months = 12
	}
	firstOfMonth := time.Date(base.Year(), base.Month()+time.Month(months), 1, 0, 0, 0, 0, base.Location())
	lastDay := firstOfMonth.AddDate(0, 1, -1).Day()
	day := base.Day()
	if day > lastDay {
		day = lastDay
	}
	return time.Date(firstOfMonth.Year(), firstOfMonth.Month(), day, 0, 0, 0, 0, base.Location())
}

func (m *Manager) DeleteNode(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var newNodes []common.NodeConfig
	for _, n := range m.cfg.Nodes {
		if n.ID != id {
			newNodes = append(newNodes, n)
		}
	}
	m.cfg.Nodes = newNodes
	return m.saveLocked()
}

// ReorderNodes 根据传入的节点ID列表重排所有节点的持久化展示顺序
func (m *Manager) ReorderNodes(nodeIDs []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	nodeMap := make(map[string]common.NodeConfig, len(m.cfg.Nodes))
	for _, n := range m.cfg.Nodes {
		nodeMap[n.ID] = n
	}

	var orderedNodes []common.NodeConfig
	seen := make(map[string]bool)
	for _, id := range nodeIDs {
		if n, ok := nodeMap[id]; ok && !seen[id] {
			orderedNodes = append(orderedNodes, n)
			seen[id] = true
		}
	}

	// 补全任何未在传入列表中的遗留节点
	for _, n := range m.cfg.Nodes {
		if !seen[n.ID] {
			orderedNodes = append(orderedNodes, n)
		}
	}

	m.cfg.Nodes = orderedNodes
	return m.saveLocked()
}

// ImportBackup 导入外部备份，完全恢复系统配置与节点列表
func (m *Manager) ImportBackup(newCfg common.ServerConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 保护机制：如果导入的备份中密码为空，保留当前管理员密码
	if newCfg.AdminPassword == "" {
		newCfg.AdminPassword = m.cfg.AdminPassword
	}
	if newCfg.ListenPort <= 0 {
		newCfg.ListenPort = m.cfg.ListenPort
	}
	if newCfg.SiteTitle == "" {
		newCfg.SiteTitle = "天阙·神盾安全探针"
	}
	if newCfg.OfflineSec <= 0 {
		newCfg.OfflineSec = 60
	}
	if len(newCfg.WarnDays) == 0 {
		newCfg.WarnDays = []int{7, 3, 1, 0}
	}

	m.cfg = newCfg
	return m.saveLocked()
}

func generateRandomSecret(length int) string {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "secret123456"
	}
	return hex.EncodeToString(bytes)
}
