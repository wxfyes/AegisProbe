package common

import "strings"

// MetricsData 定义受控端 Agent 采集到的原始指标
type MetricsData struct {
	CPUPercent    float64 `json:"cpu_percent"`    // CPU 使用率 0-100
	MemoryUsed    uint64  `json:"memory_used"`    // 内存使用量 (bytes)
	MemoryTotal   uint64  `json:"memory_total"`   // 内存总量 (bytes)
	SwapUsed      uint64  `json:"swap_used"`      // SWAP 使用量 (bytes)
	SwapTotal     uint64  `json:"swap_total"`     // SWAP 总量 (bytes)
	DiskUsed      uint64  `json:"disk_used"`      // 磁盘使用量 (bytes)
	DiskTotal     uint64  `json:"disk_total"`     // 磁盘总量 (bytes)
	NetInSpeed    uint64  `json:"net_in_speed"`   // 瞬时下行速率 (bytes/s)
	NetOutSpeed   uint64  `json:"net_out_speed"`  // 瞬时上行速率 (bytes/s)
	NetInTotal    uint64  `json:"net_in_total"`   // 累计接收流量 (bytes)
	NetOutTotal   uint64  `json:"net_out_total"`  // 累计发送流量 (bytes)
	Load1         float64 `json:"load_1"`         // 系统 1分钟负载
	Load5         float64 `json:"load_5"`         // 系统 5分钟负载
	Load15        float64 `json:"load_15"`        // 系统 15分钟负载
	UptimeSeconds uint64  `json:"uptime_seconds"` // 系统开机运行秒数
	CPUCores      int     `json:"cpu_cores"`      // CPU 物理核心数
	Arch          string  `json:"arch"`           // 架构 (如 amd64, arm64)
	OS            string  `json:"os"`             // 发行版 (如 debian, ubuntu, alpine, centos, windows)
	PublicIP      string  `json:"public_ip,omitempty"`   // 探测到的公网 IP
	IPv4          string  `json:"ipv4,omitempty"`        // 探测到的 IPv4 地址
	IPv6          string  `json:"ipv6,omitempty"`        // 探测到的 IPv6 地址
	CountryCode   string  `json:"country_code,omitempty"` // 探测到的国家代码 (如 HK, TW, US)
}

// ReportPayload 是 Agent 纯单向 POST 的请求体（无任何反向控制参数）
type ReportPayload struct {
	NodeID    string      `json:"node_id"`
	AuthToken string      `json:"auth_token"`
	Timestamp int64       `json:"timestamp"`
	Metrics   MetricsData `json:"metrics"`
}

// NodeConfig 代表由服务端后台管理的受控节点元数据
type NodeConfig struct {
	ID         string `json:"id"`               // 节点唯一ID (如 node_01)
	Token      string `json:"token"`            // 鉴权 Token
	Name       string `json:"name"`             // 节点展示名 (如 "香港-30M-122")
	Region     string `json:"region"`           // 地区代码 (如 "HK", "TW", "US", "JP")
	Flag       string `json:"flag"`             // 国旗 Emoji (如 "🇭🇰")
	OS         string `json:"os"`               // 发行版图标 (如 "debian", "ubuntu", "alpine", "centos", "windows")
	Group      string `json:"group"`            // 所属分组 (如 "中转", "亚洲", "直连", "北美洲", "欧洲", "DNS", "站点")
	Price      string `json:"price"`            // 计费标签 (如 "$29.99/月")
	ExpireDate string `json:"expire_date"`      // 到期日期 (格式 "YYYY/MM/DD" 或 "YYYY-MM-DD")
	SortOrder  int    `json:"sort_order"`       // 排序权重
	ProbePort  int    `json:"probe_port,omitempty"` // 测活探测端口 (默认 22)
	Remark     string `json:"remark,omitempty"` // 内部备注 (绝不返回给大屏)
}

// ServerConfig 服务端全局配置模型
type ServerConfig struct {
	ListenPort       int          `json:"listen_port"`        // 监听端口，默认 8989
	AdminPassword    string       `json:"admin_password"`     // 管理员密码
	AccessMode       string       `json:"access_mode"`        // 大屏访问模式: "public" (公开) 或 "private" (密码锁屏)
	SiteTitle        string       `json:"site_title"`         // 大屏站点标题
	TGEnabled        bool         `json:"tg_enabled"`         // 是否启用 Telegram 通知
	TGBotToken       string       `json:"tg_bot_token"`       // Telegram Bot Token
	TGChatID         string       `json:"tg_chat_id"`         // Telegram 目标 Chat ID
	TGProxy          string       `json:"tg_proxy"`           // Telegram Socks5 / HTTP 代理 (如 socks5://127.0.0.1:10808)
	OfflineSec       int          `json:"offline_sec"`        // 判定掉线超时阈值 (秒)，默认 60
	WarnDays         []int        `json:"warn_days"`          // 到期提醒阶梯天数，默认 [7, 3, 1, 0]
	GFWCheckURL      string       `json:"gfw_check_url"`      // 外部测活 API URL 模板 (支持 {ip} 和 {port} 占位符)
	GFWCheckInterval int          `json:"gfw_check_interval"` // 测活轮询间隔 (秒)，默认 180
	Nodes            []NodeConfig `json:"nodes"`              // 节点列表
}

// OverviewSummary 大屏看板顶部的全盘聚合数据模型
type OverviewSummary struct {
	CurrentTime      string `json:"current_time"`        // "06:07:26"
	OnlineCount      int    `json:"online_count"`        // 在线节点数
	TotalCount       int    `json:"total_count"`         // 总节点数
	RegionCount      int    `json:"region_count"`        // 覆盖地区总数
	TotalNetOutBytes uint64 `json:"total_net_out_bytes"` // 全网历史总出站流量 (bytes)
	TotalNetInBytes  uint64 `json:"total_net_in_bytes"`  // 全网历史总入站流量 (bytes)
	InstantSpeedOut  uint64 `json:"instant_speed_out"`   // 全网瞬时上行速率 (bytes/s)
	InstantSpeedIn   uint64 `json:"instant_speed_in"`    // 全网瞬时下行速率 (bytes/s)
}

// NodeBillingView 大屏单节点计费胶囊视图
type NodeBillingView struct {
	Price      string `json:"price"`       // 如 "$29.99/月"
	DaysLeft   int    `json:"days_left"`   // 剩余天数 (负数表示已过期)
	ExpireDate string `json:"expire_date"` // 规范格式 "YYYY/MM/DD"
}

// NodeHardwareView 大屏单节点硬件规格视图
type NodeHardwareView struct {
	CPUCores int    `json:"cpu_cores"` // CPU 核心数
	RAMStr   string `json:"ram_str"`   // 格式化内存大小 (如 "3.84 GB")
	DiskStr  string `json:"disk_str"`  // 格式化磁盘大小 (如 "29.49 GB")
}

// NodeStatusView 大屏单节点动态状态视图 (绝对脱敏)
type NodeStatusView struct {
	IsOnline       bool    `json:"is_online"`        // 是否在线
	CPUPct         float64 `json:"cpu_pct"`          // CPU 占用百分比 (0-100)
	RAMPct         float64 `json:"ram_pct"`          // 内存占用百分比 (0-100)
	SwapPct        float64 `json:"swap_pct"`         // SWAP 占用百分比
	SwapOff        bool    `json:"swap_off"`         // SWAP 是否未开启 (若未开启前端显示灰色 OFF)
	DiskPct        float64 `json:"disk_pct"`         // 硬盘占用百分比
	SpeedUpStr     string  `json:"speed_up_str"`     // 格式化瞬时上传速率 (如 "12.48 KB/s")
	SpeedDownStr   string  `json:"speed_down_str"`   // 格式化瞬时下载速率 (如 "14.11 KB/s")
	TrafficUpStr   string  `json:"traffic_up_str"`   // 格式化累计上传流量 (如 "95.31 GB")
	TrafficDownStr string  `json:"traffic_down_str"` // 格式化累计下载流量 (如 "98.87 GB")
	LoadStr        string  `json:"load_str"`         // 负载字符串 (如 "0.00 | 0.01 | 0.00")
	UptimeStr      string  `json:"uptime_str"`       // 在线时长 (如 "在线: 6天13小时")
	LastSeenSec    int64   `json:"last_seen_sec"`    // 距离上次上报的秒数
}

// NodeCardView 大屏单节点卡片视图模型 (物理级剔除 IP、MAC、私网地址)
type NodeCardView struct {
	ID       string           `json:"id"`
	Name     string           `json:"name"`
	Region   string           `json:"region"`
	Flag     string           `json:"flag"`
	OS       string           `json:"os"`
	Group    string           `json:"group"`
	Billing   NodeBillingView  `json:"billing"`
	Hardware  NodeHardwareView `json:"hardware"`
	Status    NodeStatusView   `json:"status"`
	GFWStatus string           `json:"gfw_status,omitempty"` // 国内连通性: "ok" (畅通), "blocked" (被墙), "" (未知/未开启)
}

// SummaryResponse 大屏前端聚合查询返回体
type SummaryResponse struct {
	SiteTitle string          `json:"site_title"`
	Summary   OverviewSummary `json:"summary"`
	Groups    []string        `json:"groups"` // 所有存在的分组标签列表 (排重并置前 "所有")
	Nodes     []NodeCardView  `json:"nodes"`
}

// CountryCodeToFlag 将两位国家代码 (如 HK, TW, US, JP, SG) 转换为对应 Unicode 国旗 Emoji
func CountryCodeToFlag(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	// 如果本身已经是 Emoji (长度 > 2 字节)，直接返回
	if len(code) == 0 {
		return "🌐"
	}
	// 处理特殊情况
	if code == "TW" {
		return "🇹🇼"
	}
	if code == "HK" {
		return "🇭🇰"
	}
	if code == "MO" {
		return "🇲🇴"
	}
	if code == "CN" {
		return "🇨🇳"
	}
	if len(code) == 2 && code[0] >= 'A' && code[0] <= 'Z' && code[1] >= 'A' && code[1] <= 'Z' {
		r1 := rune(code[0]) - 'A' + 0x1F1E6
		r2 := rune(code[1]) - 'A' + 0x1F1E6
		return string([]rune{r1, r2})
	}
	// 如果传入的已经是 Emoji 字符
	return code
}
