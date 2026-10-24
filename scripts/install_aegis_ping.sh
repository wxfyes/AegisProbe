#!/usr/bin/env bash
# ==============================================================================
# 🛡️ Aegis-Ping — 天阙·神盾家宽测活微探针 一键安装/管理脚本
# 适用系统: Linux (x86_64 / arm64 / armv7 / 386)、OpenWrt、iStoreOS、Alpine
# ==============================================================================

set -e

REPO="wxfyes/AegisProbe"
INSTALL_DIR="/opt/aegis-ping"
BIN_NAME="aegis_ping"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
PLAIN='\033[0m'

check_root() {
    if [ "$(id -u)" != "0" ]; then
        echo -e "${RED}❌ 此脚本需要 root 权限！${PLAIN}"
        exit 1
    fi
}

detect_arch() {
    ARCH=$(uname -m)
    case "$ARCH" in
        x86_64|amd64)  GOARCH="amd64" ;;
        aarch64|arm64) GOARCH="arm64" ;;
        armv7*|armhf)  GOARCH="arm" ;;
        i386|i686)     GOARCH="386" ;;
        *)
            echo -e "${RED}❌ 暂不支持的架构: $ARCH${PLAIN}"
            exit 1
            ;;
    esac
}

detect_init() {
    if command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; then
        INIT="systemd"
    elif [ -f /etc/alpine-release ] || command -v rc-service >/dev/null 2>&1; then
        INIT="openrc"
    elif [ -f /etc/openwrt_release ] || command -v procd >/dev/null 2>&1; then
        INIT="openwrt"
    else
        INIT="generic"
    fi
}

install_aegis_ping() {
    check_root
    detect_arch
    detect_init

    # ── 交互式配置 ──
    echo -e "${CYAN}=================================================================${PLAIN}"
    echo -e "${CYAN}🛡️  Aegis-Ping 天阙·神盾家宽测活微探针 安装向导${PLAIN}"
    echo -e "${CYAN}=================================================================${PLAIN}"

    DEFAULT_PORT="38921"
    read -rp "监听端口 [默认 ${DEFAULT_PORT}]: " PORT
    PORT="${PORT:-$DEFAULT_PORT}"

    read -rp "访问 Token（留空则无鉴权，建议设置强密码）: " TOKEN

    echo ""
    echo -e "${YELLOW}📡 架构: ${GOARCH} | 系统管理器: ${INIT} | 端口: ${PORT}${PLAIN}"
    echo ""

    # ── 下载二进制 ──
    mkdir -p "$INSTALL_DIR"
    BIN_URL="https://github.com/${REPO}/releases/latest/download/aegis_ping_linux_${GOARCH}"
    BIN_PROXY="https://mirror.ghproxy.com/${BIN_URL}"
    TMP_BIN="/tmp/aegis_ping_${GOARCH}"

    echo -e "${CYAN}⬇️  正在下载 Aegis-Ping (${GOARCH})...${PLAIN}"
    if ! curl -fsSL --connect-timeout 8 "$BIN_URL" -o "$TMP_BIN" 2>/dev/null; then
        echo -e "${YELLOW}⚠️  直连下载慢，切换国内加速镜像...${PLAIN}"
        curl -fsSL --connect-timeout 15 "$BIN_PROXY" -o "$TMP_BIN"
    fi

    chmod +x "$TMP_BIN"
    mv -f "$TMP_BIN" "${INSTALL_DIR}/${BIN_NAME}"

    # ── 生成启动配置文件 ──
    cat > "${INSTALL_DIR}/config.env" << EOF
AEGIS_PING_ADDR=:${PORT}
AEGIS_PING_TOKEN=${TOKEN}
EOF

    # ── 注册系统服务 ──
    if [ "$INIT" = "systemd" ]; then
        cat > /etc/systemd/system/aegis-ping.service << SEOF
[Unit]
Description=Aegis-Ping TianQue Domestic Probe Service
After=network.target

[Service]
Type=simple
ExecStart=${INSTALL_DIR}/${BIN_NAME} -addr=:${PORT} -token=${TOKEN}
Restart=always
RestartSec=3
User=root

[Install]
WantedBy=multi-user.target
SEOF
        systemctl daemon-reload
        systemctl enable aegis-ping
        systemctl restart aegis-ping

    elif [ "$INIT" = "openrc" ]; then
        cat > /etc/init.d/aegis-ping << SEOF
#!/sbin/openrc-run
name="aegis-ping"
description="Aegis-Ping Domestic Probe"
command="${INSTALL_DIR}/${BIN_NAME}"
command_args="-addr=:${PORT} -token=${TOKEN}"
command_background="true"
pidfile="/run/aegis-ping.pid"
depend() { need net; }
SEOF
        chmod +x /etc/init.d/aegis-ping
        rc-update add aegis-ping default
        rc-service aegis-ping restart

    elif [ "$INIT" = "openwrt" ]; then
        cat > /etc/init.d/aegis-ping << SEOF
#!/bin/sh /etc/rc.common
START=99
USE_PROCD=1
PROG=${INSTALL_DIR}/${BIN_NAME}
start_service() {
    procd_open_instance
    procd_set_param command "\$PROG" -addr=:${PORT} -token=${TOKEN}
    procd_set_param respawn 3600 5 0
    procd_set_param stdout 1
    procd_set_param stderr 1
    procd_close_instance
}
SEOF
        chmod +x /etc/init.d/aegis-ping
        /etc/init.d/aegis-ping enable
        /etc/init.d/aegis-ping restart

    else
        nohup "${INSTALL_DIR}/${BIN_NAME}" -addr=":${PORT}" -token="${TOKEN}" >/dev/null 2>&1 &
        echo "$!" > /var/run/aegis-ping.pid
    fi

    echo ""
    echo -e "${GREEN}=================================================================${PLAIN}"
    echo -e "${GREEN}✅ Aegis-Ping 已成功安装并启动！${PLAIN}"
    echo ""
    echo -e "${CYAN}📡 接口测试（直接点击或在软路由局域网内验证）：${PLAIN}"
    LOCAL_IP=$(ip route get 1 2>/dev/null | awk '{print $NF; exit}' || echo "你的公网IP")
    if [ -n "$TOKEN" ]; then
        echo -e "  ${GREEN}健康检查  :${PLAIN} http://${LOCAL_IP}:${PORT}/health"
        echo -e "  ${GREEN}ICMP Ping :${PLAIN} http://${LOCAL_IP}:${PORT}/ping?address=1.1.1.1&token=${TOKEN}"
        echo -e "  ${GREEN}TCPing    :${PLAIN} http://${LOCAL_IP}:${PORT}/tcping?address=1.1.1.1&port=80&token=${TOKEN}"
        echo ""
        echo -e "${YELLOW}💡 在 AegisProbe 主控后台填入的 API 模板：${PLAIN}"
        echo -e "   http://[你家公网IP]:${PORT}/ping?address={ip}&token=${TOKEN}"
    else
        echo -e "  ${GREEN}健康检查  :${PLAIN} http://${LOCAL_IP}:${PORT}/health"
        echo -e "  ${GREEN}ICMP Ping :${PLAIN} http://${LOCAL_IP}:${PORT}/ping?address=1.1.1.1"
        echo -e "  ${GREEN}TCPing    :${PLAIN} http://${LOCAL_IP}:${PORT}/tcping?address=1.1.1.1&port=80"
        echo ""
        echo -e "${YELLOW}💡 在 AegisProbe 主控后台填入的 API 模板：${PLAIN}"
        echo -e "   http://[你家公网IP]:${PORT}/ping?address={ip}"
    fi
    echo -e "${GREEN}=================================================================${PLAIN}"
    echo -e "${YELLOW}⚠️  请在软路由防火墙放行 TCP ${PORT} 端口！${PLAIN}"
}

uninstall_aegis_ping() {
    check_root
    detect_init

    if [ "$INIT" = "systemd" ]; then
        systemctl stop aegis-ping 2>/dev/null || true
        systemctl disable aegis-ping 2>/dev/null || true
        rm -f /etc/systemd/system/aegis-ping.service
        systemctl daemon-reload
    elif [ "$INIT" = "openrc" ]; then
        rc-service aegis-ping stop 2>/dev/null || true
        rc-update del aegis-ping 2>/dev/null || true
        rm -f /etc/init.d/aegis-ping
    elif [ "$INIT" = "openwrt" ]; then
        /etc/init.d/aegis-ping stop 2>/dev/null || true
        /etc/init.d/aegis-ping disable 2>/dev/null || true
        rm -f /etc/init.d/aegis-ping
    fi

    rm -rf "$INSTALL_DIR"
    echo -e "${GREEN}✅ Aegis-Ping 已彻底卸载！${PLAIN}"
}

show_status() {
    detect_init
    echo -e "${CYAN}============ Aegis-Ping 服务状态 ============${PLAIN}"
    if [ "$INIT" = "systemd" ]; then
        systemctl status aegis-ping --no-pager || true
    elif [ "$INIT" = "openrc" ]; then
        rc-service aegis-ping status 2>/dev/null || ps | grep aegis_ping | grep -v grep || true
    else
        ps | grep aegis_ping | grep -v grep || echo "服务未在运行"
    fi
    if [ -f "${INSTALL_DIR}/config.env" ]; then
        echo ""
        echo -e "${CYAN}── 当前配置 ──${PLAIN}"
        cat "${INSTALL_DIR}/config.env"
    fi
    echo -e "${CYAN}=============================================${PLAIN}"
}

# ── 主入口 ────────────────────────────────────────────────────────────────────
case "${1:-}" in
    --install)   install_aegis_ping ;;
    --uninstall) uninstall_aegis_ping ;;
    --status)    show_status ;;
    --restart)
        check_root; detect_init
        if [ "$INIT" = "systemd" ]; then systemctl restart aegis-ping
        elif [ "$INIT" = "openrc" ]; then rc-service aegis-ping restart
        elif [ "$INIT" = "openwrt" ]; then /etc/init.d/aegis-ping restart
        fi
        echo -e "${GREEN}✅ Aegis-Ping 已重启！${PLAIN}"
        ;;
    *)
        echo "用法: bash install_aegis_ping.sh [--install|--uninstall|--status|--restart]"
        install_aegis_ping
        ;;
esac
