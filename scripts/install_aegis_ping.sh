#!/bin/sh
# ==============================================================================
# 🛡️ Aegis-Ping — 天阙·神盾家宽测活微探针 一键安装/管理脚本
# 适用系统: Linux (x86_64 / arm64 / armv7 / 386)、OpenWrt、iStoreOS、Alpine
# 兼容 sh / bash / busybox ash
# ==============================================================================

REPO="wxfyes/AegisProbe"
INSTALL_DIR="/opt/aegis-ping"
BIN_NAME="aegis_ping"
DEFAULT_PORT="38921"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
PLAIN='\033[0m'

check_root() {
    if [ "$(id -u 2>/dev/null || echo 1)" != "0" ]; then
        echo -e "${RED}❌ 此脚本需要 root 权限！请使用 sudo 或切换至 root 用户运行。${PLAIN}"
        exit 1
    fi
}

detect_arch() {
    ARCH=$(uname -m 2>/dev/null || echo "x86_64")
    case "$ARCH" in
        x86_64|amd64)  GOARCH="amd64" ;;
        aarch64|arm64) GOARCH="arm64" ;;
        armv7*|armhf)  GOARCH="armv7" ;;
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

# 自动放行 OpenWrt / iStoreOS 防火墙端口
configure_firewall() {
    local port="$1"
    if [ "$INIT" = "openwrt" ] && command -v uci >/dev/null 2>&1; then
        if ! uci show firewall 2>/dev/null | grep -q "Allow-Aegis-Ping"; then
            echo -e "${YELLOW}🛡️ 正在自动配置 iStoreOS/OpenWrt 防火墙放行 TCP ${port} 端口...${PLAIN}"
            uci add firewall rule >/dev/null 2>&1 || true
            uci set firewall.@rule[-1].name='Allow-Aegis-Ping'
            uci set firewall.@rule[-1].src='wan'
            uci set firewall.@rule[-1].proto='tcp'
            uci set firewall.@rule[-1].dest_port="${port}"
            uci set firewall.@rule[-1].target='ACCEPT'
            uci commit firewall >/dev/null 2>&1 || true
            /etc/init.d/firewall reload >/dev/null 2>&1 || true
            echo -e "${GREEN}✅ 已自动在防火墙中放行 WAN 侧入站 TCP ${port} 端口！${PLAIN}"
        fi
    fi
}

install_aegis_ping() {
    check_root
    detect_arch
    detect_init

    echo -e "${CYAN}=================================================================${PLAIN}"
    echo -e "${CYAN}🛡️  Aegis-Ping 天阙·神盾家宽测活微探针 安装向导${PLAIN}"
    echo -e "${CYAN}=================================================================${PLAIN}"

    # 1. 端口优先从命令行参数读取；若未传且是交互终端则提示输入，否则使用默认端口
    if [ -z "$PORT" ]; then
        if [ -c /dev/tty ] && [ -t 0 ]; then
            printf "监听端口 [回车默认 %s]: " "$DEFAULT_PORT"
            read -r input_port < /dev/tty 2>/dev/null || input_port=""
            PORT="${input_port:-$DEFAULT_PORT}"
        else
            PORT="$DEFAULT_PORT"
        fi
    fi

    # 2. Token 读取逻辑
    if [ -z "$TOKEN" ]; then
        if [ -c /dev/tty ] && [ -t 0 ]; then
            printf "访问 Token (可选，留空则免密，建议设置密钥): "
            read -r input_token < /dev/tty 2>/dev/null || input_token=""
            TOKEN="$input_token"
        else
            TOKEN=""
        fi
    fi

    echo -e "${YELLOW}📡 硬件架构: ${GOARCH} | 服务管理器: ${INIT} | 监听端口: ${PORT}${PLAIN}"
    if [ -n "$TOKEN" ]; then
        echo -e "${YELLOW}🔑 访问密钥: ${TOKEN}${PLAIN}"
    else
        echo -e "${YELLOW}🔑 访问密钥: 无 (开放模式)${PLAIN}"
    fi
    echo ""

    # ── 下载二进制 (多镜像智能轮询，秒级极速切换) ──
    mkdir -p "$INSTALL_DIR"
    TMP_BIN="/tmp/aegis_ping_${GOARCH}"
    rm -f "$TMP_BIN"

    URLS="
https://ghfast.top/https://github.com/${REPO}/releases/latest/download/aegis_ping_linux_${GOARCH}
https://ghproxy.net/https://github.com/${REPO}/releases/latest/download/aegis_ping_linux_${GOARCH}
https://github.com/${REPO}/releases/latest/download/aegis_ping_linux_${GOARCH}
"

    echo -e "${CYAN}⬇️  正在下载 Aegis-Ping (${GOARCH})...${PLAIN}"
    download_ok=0

    for dl_url in $URLS; do
        [ -z "$dl_url" ] && continue
        echo -e "${YELLOW}⚡ 正在通过线路拉取: $(echo "$dl_url" | cut -d'/' -f3)...${PLAIN}"
        if command -v curl >/dev/null 2>&1; then
            if curl -fL --connect-timeout 5 --max-time 30 --progress-bar "$dl_url" -o "$TMP_BIN"; then
                if [ -s "$TMP_BIN" ]; then
                    download_ok=1
                    break
                fi
            fi
        elif command -v wget >/dev/null 2>&1; then
            if wget --timeout=5 -t 1 -qO "$TMP_BIN" "$dl_url"; then
                if [ -s "$TMP_BIN" ]; then
                    download_ok=1
                    break
                fi
            fi
        fi
        rm -f "$TMP_BIN"
    done

    if [ "$download_ok" -eq 0 ] || [ ! -s "$TMP_BIN" ]; then
        echo -e "${RED}❌ 所有镜像下载均失败，请检查网络连接！${PLAIN}"
        exit 1
    fi

    chmod +x "$TMP_BIN"
    mv -f "$TMP_BIN" "${INSTALL_DIR}/${BIN_NAME}"

    # ── 生成启动配置文件 ──
    cat > "${INSTALL_DIR}/config.env" << EOF
AEGIS_PING_ADDR=:${PORT}
AEGIS_PING_TOKEN=${TOKEN}
EOF

    # ── 注册系统服务并启动 ──
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
        systemctl enable aegis-ping >/dev/null 2>&1 || true
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
        rc-update add aegis-ping default >/dev/null 2>&1 || true
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
        /etc/init.d/aegis-ping enable >/dev/null 2>&1 || true
        /etc/init.d/aegis-ping restart

    else
        killall aegis_ping 2>/dev/null || true
        nohup "${INSTALL_DIR}/${BIN_NAME}" -addr=":${PORT}" -token="${TOKEN}" >/dev/null 2>&1 &
        echo "$!" > /var/run/aegis-ping.pid
    fi

    # 自动放行防火墙
    configure_firewall "${PORT}"

    # 验证运行状态
    sleep 1
    is_running=0
    if ps 2>/dev/null | grep -v grep | grep -q aegis_ping; then
        is_running=1
    elif pgrep aegis_ping >/dev/null 2>&1; then
        is_running=1
    fi

    echo ""
    echo -e "${GREEN}=================================================================${PLAIN}"
    if [ "$is_running" -eq 1 ]; then
        echo -e "${GREEN}✅ Aegis-Ping 家宽微探针已成功安装并运行！${PLAIN}"
    else
        echo -e "${YELLOW}⚠️  Aegis-Ping 已安装，正在后台拉起...${PLAIN}"
    fi
    echo ""
    echo -e "${CYAN}📡 接口说明（支持纯 ICMP Ping 与 TCPing）：${PLAIN}"
    if [ -n "$TOKEN" ]; then
        echo -e "  • ${GREEN}健康检查${PLAIN} : http://[你的公网IP]:${PORT}/health"
        echo -e "  • ${GREEN}ICMP Ping${PLAIN}: http://[你的公网IP]:${PORT}/ping?address=1.1.1.1&token=${TOKEN}"
        echo -e "  • ${GREEN}TCPing   ${PLAIN}: http://[你的公网IP]:${PORT}/tcping?address=1.1.1.1&port=80&token=${TOKEN}"
        echo ""
        echo -e "${YELLOW}👉 请复制此模板填入 AegisProbe 管理后台【系统设置】->【外部测活 API URL 模板】：${PLAIN}"
        echo -e "   ${CYAN}http://[你的公网IP或DDNS]:${PORT}/ping?address={ip}&token=${TOKEN}${PLAIN}"
    else
        echo -e "  • ${GREEN}健康检查${PLAIN} : http://[你的公网IP]:${PORT}/health"
        echo -e "  • ${GREEN}ICMP Ping${PLAIN}: http://[你的公网IP]:${PORT}/ping?address=1.1.1.1"
        echo -e "  • ${GREEN}TCPing   ${PLAIN}: http://[你的公网IP]:${PORT}/tcping?address=1.1.1.1&port=80"
        echo ""
        echo -e "${YELLOW}👉 请复制此模板填入 AegisProbe 管理后台【系统设置】->【外部测活 API URL 模板】：${PLAIN}"
        echo -e "   ${CYAN}http://[你的公网IP或DDNS]:${PORT}/ping?address={ip}${PLAIN}"
    fi
    echo -e "${GREEN}=================================================================${PLAIN}"
    echo -e "${PLAIN}提示: 该接口为纯 ICMP Ping，不需要服务器开放任何 80/22 端口，只要小鸡在线即可精准测活！${PLAIN}"
}

uninstall_aegis_ping() {
    check_root
    detect_init

    if [ "$INIT" = "systemd" ]; then
        systemctl stop aegis-ping 2>/dev/null || true
        systemctl disable aegis-ping 2>/dev/null || true
        rm -f /etc/systemd/system/aegis-ping.service
        systemctl daemon-reload 2>/dev/null || true
    elif [ "$INIT" = "openrc" ]; then
        rc-service aegis-ping stop 2>/dev/null || true
        rc-update del aegis-ping 2>/dev/null || true
        rm -f /etc/init.d/aegis-ping
    elif [ "$INIT" = "openwrt" ]; then
        /etc/init.d/aegis-ping stop 2>/dev/null || true
        /etc/init.d/aegis-ping disable 2>/dev/null || true
        rm -f /etc/init.d/aegis-ping
    else
        killall aegis_ping 2>/dev/null || true
    fi

    rm -rf "$INSTALL_DIR"
    echo -e "${GREEN}✅ Aegis-Ping 已彻底卸载！${PLAIN}"
}

show_status() {
    detect_init
    echo -e "${CYAN}============ Aegis-Ping 服务状态 ============${PLAIN}"
    if [ "$INIT" = "systemd" ]; then
        systemctl status aegis-ping --no-pager 2>/dev/null || true
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

# ── 参数解析 ──
ACTION="install"
PORT=""
TOKEN=""

while [ $# -gt 0 ]; do
    case "$1" in
        --install)
            ACTION="install"
            shift
            ;;
        --uninstall)
            ACTION="uninstall"
            shift
            ;;
        --status)
            ACTION="status"
            shift
            ;;
        --restart)
            ACTION="restart"
            shift
            ;;
        --port=*)
            PORT="${1#*=}"
            shift
            ;;
        --token=*)
            TOKEN="${1#*=}"
            shift
            ;;
        *)
            shift
            ;;
    esac
done

case "$ACTION" in
    install)
        install_aegis_ping
        ;;
    uninstall)
        uninstall_aegis_ping
        ;;
    status)
        show_status
        ;;
    restart)
        check_root
        detect_init
        if [ "$INIT" = "systemd" ]; then systemctl restart aegis-ping
        elif [ "$INIT" = "openrc" ]; then rc-service aegis-ping restart
        elif [ "$INIT" = "openwrt" ]; then /etc/init.d/aegis-ping restart
        fi
        echo -e "${GREEN}✅ Aegis-Ping 已重启！${PLAIN}"
        ;;
esac
