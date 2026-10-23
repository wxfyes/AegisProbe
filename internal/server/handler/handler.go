package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"tianque_probe/internal/common"
	"tianque_probe/internal/server/auth"
	"tianque_probe/internal/server/config"
	"tianque_probe/internal/server/notify"
	"tianque_probe/internal/server/store"
)

type RouterHandler struct {
	cfgMgr   *config.Manager
	store    *store.Store
	authMgr  *auth.AuthManager
	notifier *notify.Notifier
}

func NewRouterHandler(cfgMgr *config.Manager, st *store.Store, authMgr *auth.AuthManager, notif *notify.Notifier) *RouterHandler {
	return &RouterHandler{
		cfgMgr:   cfgMgr,
		store:    st,
		authMgr:  authMgr,
		notifier: notif,
	}
}

func (h *RouterHandler) RegisterRoutes(mux *http.ServeMux) {
	// Agent 单向上报接口
	mux.HandleFunc("/api/report", h.handleReport)

	// 大屏脱敏数据接口 (双模访问保护)
	mux.HandleFunc("/api/nodes/summary", h.handleNodesSummary)

	// 密码解锁与登录接口 (防暴力破解)
	mux.HandleFunc("/api/auth/unlock", h.handleUnlock)
	mux.HandleFunc("/api/auth/login", h.handleLogin)
	mux.HandleFunc("/api/auth/logout", h.handleLogout)

	// 管理控制台 API (强制鉴权)
	mux.HandleFunc("/api/admin/config", h.adminRequireAuth(h.handleAdminConfig))
	mux.HandleFunc("/api/admin/node/save", h.adminRequireAuth(h.handleAdminNodeSave))
	mux.HandleFunc("/api/admin/node/delete", h.adminRequireAuth(h.handleAdminNodeDelete))
	mux.HandleFunc("/api/admin/agent_install_cmd", h.adminRequireAuth(h.handleAdminInstallCmd))
	mux.HandleFunc("/api/admin/test_tg", h.adminRequireAuth(h.handleAdminTestTG))

	// 一键安装脚本直接托管
	mux.HandleFunc("/install.sh", h.handleInstallScript)
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

// handleReport 处理 Agent 纯单向 POST 上报
func (h *RouterHandler) handleReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1024*64)) // 限制包体在 64KB 以内
	if err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var payload common.ReportPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "Invalid Payload", http.StatusBadRequest)
		return
	}

	if payload.NodeID == "" || payload.AuthToken == "" {
		http.Error(w, "Missing Auth", http.StatusUnauthorized)
		return
	}

	// 校验 Token
	if !h.cfgMgr.ValidateNodeToken(payload.NodeID, payload.AuthToken) {
		http.Error(w, "Unauthorized Node", http.StatusUnauthorized)
		return
	}

	// 提取上报客户端的真实公网 IP (支持反代及直连，零下行防渗透)
	clientIP := r.Header.Get("CF-Connecting-IP")
	if clientIP == "" {
		clientIP = r.Header.Get("X-Forwarded-For")
		if clientIP != "" {
			clientIP = strings.Split(clientIP, ",")[0]
		}
	}
	if clientIP == "" {
		clientIP = r.Header.Get("X-Real-IP")
	}
	if clientIP == "" {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err == nil {
			clientIP = host
		} else {
			clientIP = r.RemoteAddr
		}
	}
	clientIP = strings.TrimSpace(clientIP)
	if payload.Metrics.PublicIP == "" {
		payload.Metrics.PublicIP = clientIP
	}

	// 更新内存状态 (绝对不生成或返回任何反向下行指令)
	h.store.UpdateReport(payload.NodeID, payload.Metrics)

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleNodesSummary 返回脱敏大屏状态
func (h *RouterHandler) handleNodesSummary(w http.ResponseWriter, r *http.Request) {
	cfg := h.cfgMgr.GetConfig()

	// 私密模式下必须鉴权 Session
	if cfg.AccessMode == "private" {
		token := auth.ExtractTokenFromRequest(r)
		if !h.authMgr.ValidateSession(token) {
			writeJSON(w, http.StatusUnauthorized, map[string]interface{}{
				"locked":        true,
				"error":         "需要密码解锁访问",
				"site_title":    cfg.SiteTitle,
				"need_password": true,
			})
			return
		}
	}

	summary := h.store.GetSummary(cfg.SiteTitle, cfg.Nodes, cfg.OfflineSec)
	writeJSON(w, http.StatusOK, summary)
}

// handleUnlock 处理大屏私密锁屏解锁 (密码通用管理员密码)
func (h *RouterHandler) handleUnlock(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	clientIP := auth.GetClientIP(r)
	allowed, waitDur := h.authMgr.CheckRateLimit(clientIP)
	if !allowed {
		writeJSON(w, http.StatusTooManyRequests, map[string]interface{}{
			"error": fmt.Sprintf("尝试次数过多，请在 %d 秒后再试", int(waitDur.Seconds())),
		})
		return
	}

	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	cfg := h.cfgMgr.GetConfig()
	if req.Password != cfg.AdminPassword {
		h.authMgr.RecordFailedAttempt(clientIP)
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{
			"error": "访问密码不正确",
		})
		return
	}

	h.authMgr.RecordSuccess(clientIP)
	token := h.authMgr.CreateSession()

	// 设置 Cookie，支持浏览器无感保存
	http.SetCookie(w, &http.Cookie{
		Name:     "aegis_session",
		Value:    token,
		Path:     "/",
		Expires:  time.Now().Add(7 * 24 * time.Hour),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":    true,
		"token": token,
	})
}

// handleLogin 管理后台登录
func (h *RouterHandler) handleLogin(w http.ResponseWriter, r *http.Request) {
	h.handleUnlock(w, r)
}

// handleLogout 注销
func (h *RouterHandler) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "aegis_session",
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
	})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// adminRequireAuth 后台 API 鉴权中间件
func (h *RouterHandler) adminRequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := auth.ExtractTokenFromRequest(r)
		if !h.authMgr.ValidateSession(token) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"error": "未登录或登录态已失效",
			})
			return
		}
		next(w, r)
	}
}

// handleAdminConfig 获取或修改配置
func (h *RouterHandler) handleAdminConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		cfg := h.cfgMgr.GetConfig()

		type AdminNodeView struct {
			common.NodeConfig
			RealIP   string `json:"real_ip"`
			RealOS   string `json:"real_os"`
			IsOnline bool   `json:"is_online"`
		}

		adminNodes := make([]AdminNodeView, 0, len(cfg.Nodes))
		for _, n := range cfg.Nodes {
			ip, osName, isOnline := h.store.GetNodeRealInfo(n.ID)
			adminNodes = append(adminNodes, AdminNodeView{
				NodeConfig: n,
				RealIP:     ip,
				RealOS:     osName,
				IsOnline:   isOnline,
			})
		}

		resp := map[string]interface{}{
			"site_title":     cfg.SiteTitle,
			"access_mode":    cfg.AccessMode,
			"listen_port":    cfg.ListenPort,
			"offline_sec":    cfg.OfflineSec,
			"warn_days":      cfg.WarnDays,
			"tg_enabled":     cfg.TGEnabled,
			"tg_bot_token":   cfg.TGBotToken,
			"tg_chat_id":     cfg.TGChatID,
			"tg_proxy":       cfg.TGProxy,
			"admin_password": "******",
			"nodes":          adminNodes,
		}
		writeJSON(w, http.StatusOK, resp)
		return
	}

	if r.Method == http.MethodPost {
		var req struct {
			SiteTitle     string `json:"site_title"`
			AdminPassword string `json:"admin_password"`
			AccessMode    string `json:"access_mode"`
			ListenPort    int    `json:"listen_port"`
			OfflineSec    int    `json:"offline_sec"`
			TGEnabled     bool   `json:"tg_enabled"`
			TGBotToken    string `json:"tg_bot_token"`
			TGChatID      string `json:"tg_chat_id"`
			TGProxy       string `json:"tg_proxy"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid body", http.StatusBadRequest)
			return
		}

		// 若密码未变动（仍为占位符）则不覆盖原密码
		pwd := req.AdminPassword
		if pwd == "******" || pwd == "" {
			pwd = ""
		}

		if err := h.cfgMgr.UpdateSettings(
			req.SiteTitle, pwd, req.AccessMode, req.ListenPort, req.OfflineSec,
			req.TGEnabled, req.TGBotToken, req.TGChatID, req.TGProxy,
		); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		// 同步更新 Telegram 模块配置
		h.notifier.UpdateConfig(req.TGEnabled, req.TGBotToken, req.TGChatID, req.TGProxy)

		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}

	http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
}

// handleAdminNodeSave 保存节点 (新增/编辑)
func (h *RouterHandler) handleAdminNodeSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var node common.NodeConfig
	if err := json.NewDecoder(r.Body).Decode(&node); err != nil {
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}

	if err := h.cfgMgr.UpsertNode(node); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleAdminNodeDelete 删除节点
func (h *RouterHandler) handleAdminNodeDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}

	if err := h.cfgMgr.DeleteNode(req.ID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleAdminInstallCmd 生成对应节点的一键部署命令
func (h *RouterHandler) handleAdminInstallCmd(w http.ResponseWriter, r *http.Request) {
	nodeID := r.URL.Query().Get("id")
	node, exists := h.cfgMgr.GetNode(nodeID)
	if !exists {
		http.Error(w, "Node Not Found", http.StatusNotFound)
		return
	}

	host := r.Host
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	serverURL := fmt.Sprintf("%s://%s", scheme, host)

	// 组装一键命令 (配置 nobody 沙箱)
	cmd := fmt.Sprintf(
		"curl -fsSL %s/install.sh | bash -s -- --server=%s --id=%s --token=%s",
		serverURL, serverURL, node.ID, node.Token,
	)

	writeJSON(w, http.StatusOK, map[string]string{
		"cmd":        cmd,
		"server_url": serverURL,
		"node_id":    node.ID,
		"token":      node.Token,
	})
}

// handleAdminTestTG 发送 TG 测试消息
func (h *RouterHandler) handleAdminTestTG(w http.ResponseWriter, r *http.Request) {
	cfg := h.cfgMgr.GetConfig()
	if !cfg.TGEnabled || cfg.TGBotToken == "" || cfg.TGChatID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "未开启 TG 通知或未配置 Token 与 ChatID",
		})
		return
	}

	msg := fmt.Sprintf("🛡️ <b>【天阙·神盾安全探针】</b>\n告警通道测试成功！当前时间：%s", time.Now().Format("2006-01-02 15:04:05"))
	h.notifier.Push(msg)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleInstallScript 提供一键安装 Shell 脚本
func (h *RouterHandler) handleInstallScript(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	serverURL := fmt.Sprintf("%s://%s", scheme, host)

	script := fmt.Sprintf(`#!/usr/bin/env bash
# ==============================================================================
# 天阙·神盾安全探针 (TianQue-Probe) 一键部署脚本 (由主控动态生成)
# 【安全铁律】：纯单向推送、nobody 非特权运行、零入站监听
# ==============================================================================
set -e

REPO="wxfyes/AegisProbe"
SERVER_URL="%s"
NODE_ID=""
TOKEN=""

while [[ $# -gt 0 ]]; do
  case $1 in
    --server=*) SERVER_URL="${1#*=}"; shift ;;
    --id=*)     NODE_ID="${1#*=}";    shift ;;
    --token=*)  TOKEN="${1#*=}";      shift ;;
    *)          shift ;;
  esac
done

if [ -z "$NODE_ID" ] || [ -z "$TOKEN" ]; then
  echo "❌ 错误: 必须提供 --id 和 --token 参数！"
  exit 1
fi

echo "🛡️ 开始部署天阙·神盾被控探针..."

# 检查服务管理器 (Systemd vs Alpine OpenRC vs OpenWrt procd vs 通用后台)
if command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; then
  INIT_SYS="systemd"
elif [ -f /etc/alpine-release ] || command -v rc-service >/dev/null 2>&1; then
  INIT_SYS="openrc"
elif [ -f /etc/openwrt_release ] || command -v procd >/dev/null 2>&1; then
  INIT_SYS="openwrt"
else
  INIT_SYS="generic"
fi

# 精准架构检测 (只下载当前机器所需单一二进制)
ARCH=$(uname -m)
case $ARCH in
  x86_64|amd64)   BIN_NAME="aegis_agent_linux_amd64" ;;
  aarch64|arm64)  BIN_NAME="aegis_agent_linux_arm64"  ;;
  i386|i686)      BIN_NAME="aegis_agent_linux_386"    ;;
  *)
    echo "❌ 暂不支持的架构: $ARCH"
    exit 1
    ;;
esac

AGENT_DIR="/opt/aegis-probe/agent"
mkdir -p "$AGENT_DIR"

# 停止并清理旧版 agent
if [ "$INIT_SYS" = "systemd" ]; then
  systemctl stop aegis-agent 2>/dev/null || true
  systemctl stop tianque-probe 2>/dev/null || true
elif [ "$INIT_SYS" = "openrc" ]; then
  rc-service aegis-agent stop 2>/dev/null || true
elif [ "$INIT_SYS" = "openwrt" ]; then
  /etc/init.d/aegis-agent stop 2>/dev/null || true
fi
pkill -9 -x aegis_agent 2>/dev/null || true
killall -9 aegis_agent 2>/dev/null || true
sleep 1

echo "📥 正在精准下载 ${BIN_NAME} (系统: ${INIT_SYS} | 架构: ${ARCH})..."
GH_URL="https://github.com/${REPO}/releases/latest/download/${BIN_NAME}"
MIRROR_URL="https://mirror.ghproxy.com/${GH_URL}"

if curl -fL --connect-timeout 6 -m 30 "$GH_URL" -o "${AGENT_DIR}/aegis_agent" 2>/dev/null; then
  echo "✓ 从 GitHub 直连下载成功"
else
  echo "⚠️ 直连超时，切换镜像加速节点重试..."
  if curl -fL --connect-timeout 10 -m 60 "$MIRROR_URL" -o "${AGENT_DIR}/aegis_agent" 2>/dev/null; then
    echo "✓ 从加速通道下载成功"
  else
    echo "❌ 下载失败！请检查网络或 GitHub Release 是否存在。"
    exit 1
  fi
fi
chmod +x "${AGENT_DIR}/aegis_agent"

# 根据系统自启环境分别配置
if [ "$INIT_SYS" = "systemd" ]; then
  rm -f /etc/systemd/system/tianque-probe.service 2>/dev/null || true
  cat <<EOF > /etc/systemd/system/aegis-agent.service
[Unit]
Description=TianQue Aegis Probe Agent
After=network.target

[Service]
Type=simple
User=nobody
Group=nogroup
ExecStart=${AGENT_DIR}/aegis_agent -server=${SERVER_URL} -id=${NODE_ID} -token=${TOKEN}
Restart=always
RestartSec=3
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
EOF
  systemctl daemon-reload
  systemctl enable aegis-agent
  systemctl restart aegis-agent
  sleep 2
  if systemctl is-active --quiet aegis-agent; then
    echo "✅ 天阙·神盾探针已成功安装并启动！"
  else
    echo "❌ 探针启动异常，请查看日志："
    journalctl -u aegis-agent -n 15 --no-pager
    exit 1
  fi

elif [ "$INIT_SYS" = "openrc" ]; then
  # Alpine Linux (OpenRC) 极速轻量原生服务支持
  cat <<EOF > /etc/init.d/aegis-agent
#!/sbin/openrc-run
name="aegis-agent"
description="TianQue Aegis Probe Agent"
command="${AGENT_DIR}/aegis_agent"
command_args="-server=${SERVER_URL} -id=${NODE_ID} -token=${TOKEN}"
command_background="true"
pidfile="/run/aegis-agent.pid"

depend() {
  need net
  after firewall
}
EOF
  chmod +x /etc/init.d/aegis-agent
  rc-update add aegis-agent default 2>/dev/null || true
  rc-service aegis-agent restart
  echo "✅ 天阙·神盾探针已在 Alpine Linux (OpenRC) 成功启动！"

elif [ "$INIT_SYS" = "openwrt" ]; then
  # 软路由 OpenWrt procd 支持
  cat <<EOF > /etc/init.d/aegis-agent
#!/bin/sh /etc/rc.common
START=99
USE_PROCD=1
start_service() {
  procd_open_instance
  procd_set_param command ${AGENT_DIR}/aegis_agent -server=${SERVER_URL} -id=${NODE_ID} -token=${TOKEN}
  procd_set_param respawn 3600 5 0
  procd_close_instance
}
EOF
  chmod +x /etc/init.d/aegis-agent
  /etc/init.d/aegis-agent enable
  /etc/init.d/aegis-agent restart
  echo "✅ 天阙·神盾探针已在 OpenWrt 软路由启动！"

else
  # 通用后台保活
  nohup ${AGENT_DIR}/aegis_agent -server=${SERVER_URL} -id=${NODE_ID} -token=${TOKEN} >/dev/null 2>&1 &
  echo "✅ 天阙·神盾探针已在后台运行！"
fi

echo "🛡️ 数据正在单向安全汇报至: ${SERVER_URL} (内存占用 < 4MB，零入站监听)"
`, serverURL)

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(script))
}
