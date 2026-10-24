#!/usr/bin/env bash
# ==============================================================================
# 🛡️ 天阙·神盾安全探针 (AegisProbe) 一键管理与安装脚本
# GitHub: https://github.com/wxfyes/AegisProbe
# 【全平台支持】：普通Linux (Debian/Ubuntu/CentOS)、软路由 (OpenWrt/iStoreOS/PVE/Alpine)
# 【特性】：主控/被控独立管理、无损平滑热更新、干净卸载、nobody安全沙箱、极速双镜像加速
# ==============================================================================

REPO="wxfyes/AegisProbe"
INSTALL_DIR="/opt/aegis-probe"
SYSTEMD_DIR="/etc/systemd/system"
OPENWRT_INIT_DIR="/etc/init.d"
LISTEN_PORT=8989
SCRIPT_VERSION="v1.2.0"

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
PLAIN='\033[0m'

# 检查 root 权限
check_root() {
    if [ "$(id -u)" != "0" ]; then
        echo -e "${RED}❌ 错误: 此脚本必须以 root 用户权限运行！请使用 sudo 或切换 root。${PLAIN}"
        exit 1
    fi
}

# 架构精准检测 (只匹配并下载当前机器所需架构)
detect_arch() {
    ARCH=$(uname -m)
    case "$ARCH" in
        x86_64|amd64)
            GOARCH="amd64"
            ;;
        aarch64|arm64)
            GOARCH="arm64"
            ;;
        i386|i686)
            GOARCH="386"
            ;;
        *)
            echo -e "${RED}❌ 暂不支持的硬件架构: $ARCH${PLAIN}"
            exit 1
            ;;
    esac
}

# 服务管理器检测 (Systemd vs OpenRC vs OpenWrt procd vs 通用后台)
detect_init_sys() {
    if command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; then
        INIT_SYS="systemd"
    elif [ -f /etc/alpine-release ] || (command -v rc-service >/dev/null 2>&1 && [ -d /etc/init.d ]); then
        INIT_SYS="openrc"
    elif [ -f /etc/openwrt_release ] || [ -f /etc/init.d/procd ] || command -v procd >/dev/null 2>&1; then
        INIT_SYS="openwrt"
    else
        INIT_SYS="generic"
    fi
}

# 强制释放端口 —— 获取精准 PID 后直接 kill -9，并验证端口已释放
force_free_port() {
    local port="$1"
    local pid=""

    # 三种工具轮询获取占用端口的 PID
    if command -v lsof >/dev/null 2>&1; then
        pid=$(lsof -ti :"$port" 2>/dev/null || true)
    elif command -v ss >/dev/null 2>&1; then
        pid=$(ss -tlnp "sport = :$port" 2>/dev/null | grep -oP 'pid=\K[0-9]+' | head -1 || true)
    elif command -v netstat >/dev/null 2>&1; then
        pid=$(netstat -tlnp 2>/dev/null | grep ":$port " | awk '{print $7}' | cut -d/ -f1 | head -1 || true)
    fi

    if [ -n "$pid" ]; then
        echo -e "${YELLOW}⚠️ 端口 ${port} 被 PID ${pid} 占用，正在强制释放...${PLAIN}"
        kill -9 $pid 2>/dev/null || true
        sleep 1
    fi

    # 兜底：fuser 强制释放
    if command -v fuser >/dev/null 2>&1; then
        fuser -k "${port}/tcp" 2>/dev/null || true
    fi

    # 验证端口是否已释放（最多等 3 秒）
    local tries=0
    while [ $tries -lt 3 ]; do
        local still_pid=""
        if command -v lsof >/dev/null 2>&1; then
            still_pid=$(lsof -ti :"$port" 2>/dev/null || true)
        elif command -v ss >/dev/null 2>&1; then
            still_pid=$(ss -tlnp "sport = :$port" 2>/dev/null | grep -oP 'pid=\K[0-9]+' | head -1 || true)
        fi
        if [ -z "$still_pid" ]; then
            echo -e "${GREEN}✓ 端口 ${port} 已就绪${PLAIN}"
            return 0
        fi
        sleep 1
        tries=$((tries + 1))
    done

    echo -e "${RED}❌ 端口 ${port} 仍被占用，无法继续安装！请手动执行: kill -9 \$(lsof -ti :${port})${PLAIN}"
    exit 1
}

# 智能高速下载函数 (支持 GitHub 直连 + 国内双极速镜像源轮询 fallback)
download_file() {
    local filename="$1"
    local output="$2"
    local url="https://github.com/${REPO}/releases/latest/download/${filename}"
    local mirror1="https://ghfast.top/${url}"
    local mirror2="https://ghproxy.net/${url}"

    echo -e "${CYAN}📥 正在下载 ${filename} (架构: ${GOARCH}) ...${PLAIN}"
    
    # 1. 尝试直连 (限时 8s)
    if curl -fL --connect-timeout 6 -m 30 "$url" -o "$output" 2>/dev/null; then
        echo -e "${GREEN}✓ 从 GitHub 直连下载成功${PLAIN}"
        chmod +x "$output"
        return 0
    fi

    # 2. 尝试极速镜像 1 (ghfast.top)
    echo -e "${YELLOW}⚠️ 直连较慢，切换国内极速镜像源 1 (ghfast.top)...${PLAIN}"
    if curl -fL --connect-timeout 8 -m 60 "$mirror1" -o "$output" 2>/dev/null; then
        echo -e "${GREEN}✓ 从极速镜像 1 下载成功${PLAIN}"
        chmod +x "$output"
        return 0
    fi

    # 3. 尝试极速镜像 2 (ghproxy.net)
    echo -e "${YELLOW}⚠️ 切换国内备用镜像源 2 (ghproxy.net)...${PLAIN}"
    if curl -fL --connect-timeout 10 -m 60 "$mirror2" -o "$output" 2>/dev/null; then
        echo -e "${GREEN}✓ 从极速镜像 2 下载成功${PLAIN}"
        chmod +x "$output"
        return 0
    fi

    echo -e "${RED}❌ 下载失败，请检查网络或 GitHub Release 状态！${PLAIN}"
    exit 1
}

# 获取本机公网 IP (用于提示)
get_public_ip() {
    local ip=""
    ip=$(curl -s -m 3 https://api.ipify.org || curl -s -m 3 https://ipv4.icanhazip.com || echo "你的服务器IP")
    echo "$ip"
}

# ==================== 1. 主控服务端管理 (Master) ====================

# 1.1 安装服务端
install_master() {
    check_root
    detect_arch
    detect_init_sys

    if [ "$GOARCH" != "amd64" ]; then
        echo -e "${YELLOW}⚠️ 当前机器架构为 ${GOARCH}，服务端官方二进制建议在 Linux amd64 运行。${PLAIN}"
    fi

    local bin_name="aegis_server_linux_amd64"
    local srv_dir="${INSTALL_DIR}/server"
    mkdir -p "${srv_dir}/data"

    echo -e "${BLUE}=================================================================${PLAIN}"
    echo -e "${BLUE}🛡️ 开始安装 天阙·神盾 探针服务端 (Master)...${PLAIN}"
    echo -e "${BLUE}运行环境: ${INIT_SYS} | 机器架构: ${GOARCH} | 监听端口: ${LISTEN_PORT}${PLAIN}"
    echo -e "${BLUE}=================================================================${PLAIN}"

    # 停止旧服务
    if [ "$INIT_SYS" = "systemd" ]; then
        systemctl stop aegis-server    2>/dev/null || true
        systemctl stop tianque-probe   2>/dev/null || true
        systemctl mask tianque-probe   2>/dev/null || true
        systemctl mask aegis-server    2>/dev/null || true
    elif [ "$INIT_SYS" = "openrc" ]; then
        rc-service aegis-server stop 2>/dev/null || true
        rc-service tianque-probe stop 2>/dev/null || true
    elif [ "$INIT_SYS" = "openwrt" ] && [ -f "${OPENWRT_INIT_DIR}/aegis-server" ]; then
        ${OPENWRT_INIT_DIR}/aegis-server stop 2>/dev/null || true
    fi

    pkill -9 -x aegis_server 2>/dev/null || true
    killall -9 aegis_server  2>/dev/null || true
    sleep 1

    force_free_port "$LISTEN_PORT"

    # 下载新二进制
    download_file "$bin_name" "${srv_dir}/aegis_server"

    force_free_port "$LISTEN_PORT"

    # 根据系统类型配置自启服务
    if [ "$INIT_SYS" = "systemd" ]; then
        cat <<EOF > "${SYSTEMD_DIR}/aegis-server.service"
[Unit]
Description=TianQue Aegis Probe Master Server
After=network.target

[Service]
Type=simple
WorkingDirectory=${srv_dir}
ExecStart=${srv_dir}/aegis_server -port=${LISTEN_PORT}
Restart=always
RestartSec=3
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
EOF
        systemctl unmask aegis-server 2>/dev/null || true
        systemctl daemon-reload
        systemctl enable aegis-server
        systemctl restart aegis-server

        sleep 2
        if ! systemctl is-active --quiet aegis-server; then
            echo -e "${RED}❌ 服务端启动失败！正在输出最近日志排查原因：${PLAIN}"
            journalctl -u aegis-server -n 15 --no-pager
            exit 1
        fi

    elif [ "$INIT_SYS" = "openrc" ]; then
        cat <<EOF > /etc/init.d/aegis-server
#!/sbin/openrc-run
description="TianQue Aegis Probe Master Server"
command="${srv_dir}/aegis_server"
command_args="-port=${LISTEN_PORT}"
command_background="yes"
pidfile="/run/\${RC_SVCNAME}.pid"

depend() {
    need net
    after firewall
}
EOF
        chmod +x /etc/init.d/aegis-server
        rc-update add aegis-server default 2>/dev/null || true
        rc-service aegis-server restart || /etc/init.d/aegis-server restart

    elif [ "$INIT_SYS" = "openwrt" ]; then
        cat <<EOF > "${OPENWRT_INIT_DIR}/aegis-server"
#!/bin/sh /etc/rc.common
START=99
STOP=10
USE_PROCD=1

PROG=${srv_dir}/aegis_server

start_service() {
    procd_open_instance
    procd_set_param command "\$PROG" -port=${LISTEN_PORT}
    procd_set_param respawn 3600 5 0
    procd_set_param stdout 1
    procd_set_param stderr 1
    procd_close_instance
}
EOF
        chmod +x "${OPENWRT_INIT_DIR}/aegis-server"
        ${OPENWRT_INIT_DIR}/aegis-server enable
        ${OPENWRT_INIT_DIR}/aegis-server restart
    else
        nohup ${srv_dir}/aegis_server -port=${LISTEN_PORT} >/dev/null 2>&1 &
    fi

    local server_ip
    server_ip=$(get_public_ip)
    local admin_pwd="请查看后台或 data/config.json"
    if [ -f "${srv_dir}/data/config.json" ]; then
        local found_pwd
        found_pwd=$(grep -o '"admin_password": "[^"]*"' "${srv_dir}/data/config.json" | cut -d'"' -f4 || echo "")
        if [ -n "$found_pwd" ]; then
            admin_pwd="$found_pwd"
        fi
    fi

    echo -e "${GREEN}=================================================================${PLAIN}"
    echo -e "${GREEN}✅ 天阙·神盾安全探针服务端 已成功安装并运行！${PLAIN}"
    echo -e "${CYAN}👉 监控大屏地址  : ${PLAIN}http://${server_ip}:${LISTEN_PORT}"
    echo -e "${CYAN}👉 管理控制台地址: ${PLAIN}http://${server_ip}:${LISTEN_PORT}/admin.html"
    echo -e "${YELLOW}👉 初始管理员密码: ${PLAIN}${admin_pwd}"
    echo -e "${PLAIN}=================================================================${PLAIN}"
}

# 1.2 一键热更新服务端 (保留所有配置)
update_master() {
    check_root
    detect_arch
    detect_init_sys

    local srv_dir="${INSTALL_DIR}/server"
    local srv_bin="${srv_dir}/aegis_server"

    if [ ! -f "$srv_bin" ]; then
        echo -e "${YELLOW}⚠️ 未检测到已安装的服务端，自动转入全新安装流程...${PLAIN}"
        install_master
        return 0
    fi

    echo -e "${BLUE}=================================================================${PLAIN}"
    echo -e "${BLUE}🔄 正在热更新 天阙·神盾 探针服务端 (保留 data/config.json 配置)...${PLAIN}"
    echo -e "${BLUE}=================================================================${PLAIN}"

    # 临时下载新二进制
    local tmp_bin="/tmp/aegis_server_update"
    download_file "aegis_server_linux_amd64" "$tmp_bin"
    chmod +x "$tmp_bin"

    # 停止旧服务
    if [ "$INIT_SYS" = "systemd" ]; then
        systemctl stop aegis-server 2>/dev/null || true
    elif [ "$INIT_SYS" = "openrc" ]; then
        rc-service aegis-server stop 2>/dev/null || true
    elif [ "$INIT_SYS" = "openwrt" ]; then
        ${OPENWRT_INIT_DIR}/aegis-server stop 2>/dev/null || true
    fi
    pkill -9 -x aegis_server 2>/dev/null || true
    sleep 1

    # 替换主控二进制
    mv -f "$tmp_bin" "$srv_bin"
    chmod 755 "$srv_bin"

    # 重启服务
    if [ "$INIT_SYS" = "systemd" ]; then
        systemctl restart aegis-server
        sleep 2
        if systemctl is-active --quiet aegis-server; then
            echo -e "${GREEN}✅ 主控服务端已平滑热更新至最新版本！所有节点配置与密码完整保留。${PLAIN}"
        else
            echo -e "${RED}❌ 重启失败，请检查 journalctl -u aegis-server -n 20${PLAIN}"
        fi
    elif [ "$INIT_SYS" = "openrc" ]; then
        rc-service aegis-server restart
        echo -e "${GREEN}✅ 主控服务端已热更新并重启！${PLAIN}"
    elif [ "$INIT_SYS" = "openwrt" ]; then
        ${OPENWRT_INIT_DIR}/aegis-server restart
        echo -e "${GREEN}✅ 主控服务端已热更新并重启！${PLAIN}"
    fi
}

# 1.3 独立卸载服务端
uninstall_master() {
    check_root
    detect_init_sys

    local srv_dir="${INSTALL_DIR}/server"
    if [ ! -d "$srv_dir" ] && [ ! -f "${SYSTEMD_DIR}/aegis-server.service" ]; then
        echo -e "${YELLOW}⚠️ 未检测到已安装的主控服务端。${PLAIN}"
        return 0
    fi

    echo -e "${YELLOW}=================================================================${PLAIN}"
    echo -e "${YELLOW}⚠️ 准备卸载 天阙·神盾 探针服务端 (Master)${PLAIN}"
    echo -e "${YELLOW}=================================================================${PLAIN}"
    read -rp "确认卸载主控服务端吗？[y/N]: " confirm
    if [[ ! "$confirm" =~ ^[yY]$ ]]; then
        echo -e "${CYAN}已取消卸载。${PLAIN}"
        return 0
    fi

    read -rp "是否同时删除节点数据与配置文件 (data/config.json)？[y/N] (默认保留配置): " purge_data

    # 停止与清理自启服务
    if [ "$INIT_SYS" = "systemd" ]; then
        systemctl stop aegis-server 2>/dev/null || true
        systemctl disable aegis-server 2>/dev/null || true
        rm -f "${SYSTEMD_DIR}/aegis-server.service"
        systemctl daemon-reload
    elif [ "$INIT_SYS" = "openrc" ]; then
        rc-service aegis-server stop 2>/dev/null || true
        rc-update del aegis-server default 2>/dev/null || true
        rm -f /etc/init.d/aegis-server
    elif [ "$INIT_SYS" = "openwrt" ]; then
        ${OPENWRT_INIT_DIR}/aegis-server stop 2>/dev/null || true
        ${OPENWRT_INIT_DIR}/aegis-server disable 2>/dev/null || true
        rm -f "${OPENWRT_INIT_DIR}/aegis-server"
    fi

    pkill -9 -x aegis_server 2>/dev/null || true
    killall -9 aegis_server 2>/dev/null || true

    if [[ "$purge_data" =~ ^[yY]$ ]]; then
        rm -rf "$srv_dir"
        echo -e "${GREEN}✓ 主控服务端二进制与数据配置已彻底删除。${PLAIN}"
    else
        rm -f "${srv_dir}/aegis_server"
        echo -e "${GREEN}✓ 主控服务已卸载，配置数据已完整保留在: ${srv_dir}/data/${PLAIN}"
    fi

    # 若 Agent 也不存在，清理母目录
    if [ ! -d "${INSTALL_DIR}/agent" ] && [ ! -d "${INSTALL_DIR}/server" ]; then
        rm -rf "$INSTALL_DIR" 2>/dev/null || true
    fi

    echo -e "${GREEN}✅ 主控服务端已成功卸载！${PLAIN}"
}

# ==================== 2. 受控端管理 (Agent) ====================

# 2.1 安装受控端
install_agent() {
    check_root
    detect_arch
    detect_init_sys

    local server_url="$1"
    local node_id="$2"
    local node_token="$3"

    if [ -z "$server_url" ] || [ -z "$node_id" ] || [ -z "$node_token" ]; then
        echo -e "${BLUE}请输入受控端连接参数 (在主控后台 [一键命令] 中可直接复制完整指令):${PLAIN}"
        read -rp "主控端地址 (例如 http://1.2.3.4:8989): " server_url
        read -rp "节点唯一标识 (例如 hk-01): " node_id
        read -rp "节点认证 Token (例如 secret_token): " node_token
    fi

    if [ -z "$server_url" ] || [ -z "$node_id" ] || [ -z "$node_token" ]; then
        echo -e "${RED}❌ 参数不可为空！${PLAIN}"
        exit 1
    fi

    local bin_name="aegis_agent_linux_${GOARCH}"
    local agent_dir="${INSTALL_DIR}/agent"
    mkdir -p "$agent_dir"

    echo -e "${BLUE}=================================================================${PLAIN}"
    echo -e "${BLUE}🛡️ 开始部署 天阙·神盾 受控端 (Agent)...${PLAIN}"
    echo -e "${BLUE}运行环境: ${INIT_SYS} | 机器架构: ${GOARCH} (精准下载 ${bin_name})${PLAIN}"
    echo -e "${BLUE}【安全铁律生效】：纯单向推送、零入站端口、nobody 安全沙箱${PLAIN}"
    echo -e "${BLUE}=================================================================${PLAIN}"

    if [ "$INIT_SYS" = "systemd" ]; then
        systemctl stop aegis-agent 2>/dev/null || true
    elif [ "$INIT_SYS" = "openrc" ]; then
        rc-service aegis-agent stop 2>/dev/null || true
    elif [ "$INIT_SYS" = "openwrt" ] && [ -f "${OPENWRT_INIT_DIR}/aegis-agent" ]; then
        ${OPENWRT_INIT_DIR}/aegis-agent stop 2>/dev/null || true
    fi

    # 下载对应架构的 agent
    download_file "$bin_name" "${agent_dir}/aegis_agent"
    chmod +x "${agent_dir}/aegis_agent"

    # 配置开机自启
    if [ "$INIT_SYS" = "systemd" ]; then
        cat <<EOF > "${SYSTEMD_DIR}/aegis-agent.service"
[Unit]
Description=TianQue Aegis Probe Agent
After=network.target

[Service]
Type=simple
User=nobody
Group=nogroup
ExecStart=${agent_dir}/aegis_agent -server=${server_url} -id=${node_id} -token=${node_token}
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
        if ! systemctl is-active --quiet aegis-agent; then
            echo -e "${RED}❌ 受控端启动失败！排查日志：${PLAIN}"
            journalctl -u aegis-agent -n 15 --no-pager
            exit 1
        fi

    elif [ "$INIT_SYS" = "openrc" ]; then
        cat <<EOF > /etc/init.d/aegis-agent
#!/sbin/openrc-run
description="TianQue Aegis Probe Agent"
command="${agent_dir}/aegis_agent"
command_args="-server=${server_url} -id=${node_id} -token=${node_token}"
command_background="yes"
pidfile="/run/\${RC_SVCNAME}.pid"

depend() {
    need net
    after firewall
}
EOF
        chmod +x /etc/init.d/aegis-agent
        rc-update add aegis-agent default 2>/dev/null || true
        rc-service aegis-agent restart || /etc/init.d/aegis-agent restart

    elif [ "$INIT_SYS" = "openwrt" ]; then
        cat <<EOF > "${OPENWRT_INIT_DIR}/aegis-agent"
#!/bin/sh /etc/rc.common
START=99
STOP=10
USE_PROCD=1

PROG=${agent_dir}/aegis_agent

start_service() {
    procd_open_instance
    procd_set_param command "\$PROG" -server="${server_url}" -id="${node_id}" -token="${node_token}"
    procd_set_param respawn 3600 5 0
    procd_set_param stdout 1
    procd_set_param stderr 1
    procd_close_instance
}
EOF
        chmod +x "${OPENWRT_INIT_DIR}/aegis-agent"
        ${OPENWRT_INIT_DIR}/aegis-agent enable
        ${OPENWRT_INIT_DIR}/aegis-agent restart
    else
        nohup ${agent_dir}/aegis_agent -server="${server_url}" -id="${node_id}" -token="${node_token}" >/dev/null 2>&1 &
    fi

    echo -e "${GREEN}=================================================================${PLAIN}"
    echo -e "${GREEN}✅ 天阙·神盾受控探针已启动！内存占用 < 5MB，CPU < 0.1%${PLAIN}"
    echo -e "${GREEN}🛡️ 数据正在单向汇报至: ${server_url}${PLAIN}"
    echo -e "${GREEN}=================================================================${PLAIN}"
}

# 2.2 一键热更新受控端 (保留启动参数)
update_agent_binary() {
    check_root
    detect_arch
    detect_init_sys

    local agent_dir="${INSTALL_DIR}/agent"
    local agent_bin="${agent_dir}/aegis_agent"

    if [ ! -f "$agent_bin" ]; then
        echo -e "${RED}❌ 未检测到已安装的 AegisProbe Agent！请直接使用后台一键命令安装。${PLAIN}"
        exit 1
    fi

    echo -e "${BLUE}=================================================================${PLAIN}"
    echo -e "${BLUE}🔄 正在热更新 天阙·神盾 受控端 (Agent)...${PLAIN}"
    echo -e "${BLUE}=================================================================${PLAIN}"

    local bin_name="aegis_agent_linux_${GOARCH}"
    local tmp_bin="/tmp/aegis_agent_update_${GOARCH}"

    download_file "$bin_name" "$tmp_bin"
    chmod +x "$tmp_bin"
    mv -f "$tmp_bin" "$agent_bin"
    chmod 755 "$agent_bin"

    echo -e "${GREEN}🔄 正在热重启探针服务...${PLAIN}"
    if [ "$INIT_SYS" = "systemd" ]; then
        systemctl restart aegis-agent
        sleep 2
        if systemctl is-active --quiet aegis-agent; then
            echo -e "${GREEN}✅ 受控端探针已成功热更新至最新版本！版本号与双栈数据已自动上报。${PLAIN}"
        fi
    elif [ "$INIT_SYS" = "openrc" ]; then
        rc-service aegis-agent restart
        echo -e "${GREEN}✅ 受控端探针已热更新并重启！${PLAIN}"
    elif [ "$INIT_SYS" = "openwrt" ]; then
        ${OPENWRT_INIT_DIR}/aegis-agent restart
        echo -e "${GREEN}✅ 受控端探针已热更新并重启！${PLAIN}"
    fi
}

# 2.3 独立卸载受控端
uninstall_agent() {
    check_root
    detect_init_sys

    local agent_dir="${INSTALL_DIR}/agent"
    if [ ! -d "$agent_dir" ] && [ ! -f "${SYSTEMD_DIR}/aegis-agent.service" ]; then
        echo -e "${YELLOW}⚠️ 未检测到已安装的受控端探针 (Agent)。${PLAIN}"
        return 0
    fi

    echo -e "${YELLOW}=================================================================${PLAIN}"
    echo -e "${YELLOW}⚠️ 准备卸载 天阙·神盾 受控端探针 (Agent)${PLAIN}"
    echo -e "${YELLOW}=================================================================${PLAIN}"
    read -rp "确认卸载受控端探针吗？[y/N]: " confirm
    if [[ ! "$confirm" =~ ^[yY]$ ]]; then
        echo -e "${CYAN}已取消卸载。${PLAIN}"
        return 0
    fi

    # 停止服务并清理自启
    if [ "$INIT_SYS" = "systemd" ]; then
        systemctl stop aegis-agent 2>/dev/null || true
        systemctl disable aegis-agent 2>/dev/null || true
        rm -f "${SYSTEMD_DIR}/aegis-agent.service"
        systemctl daemon-reload
    elif [ "$INIT_SYS" = "openrc" ]; then
        rc-service aegis-agent stop 2>/dev/null || true
        rc-update del aegis-agent default 2>/dev/null || true
        rm -f /etc/init.d/aegis-agent
    elif [ "$INIT_SYS" = "openwrt" ]; then
        ${OPENWRT_INIT_DIR}/aegis-agent stop 2>/dev/null || true
        ${OPENWRT_INIT_DIR}/aegis-agent disable 2>/dev/null || true
        rm -f "${OPENWRT_INIT_DIR}/aegis-agent"
    fi

    pkill -9 -x aegis_agent 2>/dev/null || true
    killall -9 aegis_agent 2>/dev/null || true
    rm -rf "$agent_dir"

    # 若 Server 也不存在，清理母目录
    if [ ! -d "${INSTALL_DIR}/server" ]; then
        rm -rf "$INSTALL_DIR" 2>/dev/null || true
    fi

    echo -e "${GREEN}✅ 受控端探针已成功卸载并彻底清理！${PLAIN}"
}

# ==================== 3. 运维辅助与配置备份 ====================

# 查看运行状态与密码
view_server_status() {
    detect_init_sys
    local srv_dir="${INSTALL_DIR}/server"
    echo -e "${BLUE}==================== 服务运行状态 ====================${PLAIN}"
    
    # 服务端状态
    if [ -f "${srv_dir}/aegis_server" ]; then
        echo -e "${CYAN}【服务端 Master】:${PLAIN}"
        if [ "$INIT_SYS" = "systemd" ]; then
            systemctl status aegis-server --no-pager -l || true
        else
            ps | grep aegis_server | grep -v grep || echo "未在运行"
        fi
        if [ -f "${srv_dir}/data/config.json" ]; then
            local pwd
            pwd=$(grep -o '"admin_password": "[^"]*"' "${srv_dir}/data/config.json" | cut -d'"' -f4 || echo "")
            echo -e "${YELLOW}🔑 当前管理员密码: ${pwd}${PLAIN}"
        fi
    else
        echo -e "${PLAIN}服务端 Master: 未安装${PLAIN}"
    fi

    echo -e "------------------------------------------------------"

    # 受控端状态
    if [ -f "${INSTALL_DIR}/agent/aegis_agent" ]; then
        echo -e "${CYAN}【受控端 Agent】:${PLAIN}"
        if [ "$INIT_SYS" = "systemd" ]; then
            systemctl status aegis-agent --no-pager -l || true
        else
            ps | grep aegis_agent | grep -v grep || echo "未在运行"
        fi
    else
        echo -e "${PLAIN}受控端 Agent: 未安装${PLAIN}"
    fi

    echo -e "${BLUE}======================================================${PLAIN}"
}

# 服务重启管理子菜单
restart_services_menu() {
    detect_init_sys
    echo -e "${CYAN}请选择要重启的服务:${PLAIN}"
    echo -e " 1. 重启 主控服务端 (aegis-server)"
    echo -e " 2. 重启 受控端 (aegis-agent)"
    echo -e " 3. 重启 全部服务"
    echo -e " 0. 返回上一级"
    read -rp "请输入选项编号 [0-3]: " opt
    case "$opt" in
        1)
            if [ "$INIT_SYS" = "systemd" ]; then
                systemctl restart aegis-server
            elif [ "$INIT_SYS" = "openrc" ]; then
                rc-service aegis-server restart
            elif [ "$INIT_SYS" = "openwrt" ]; then
                ${OPENWRT_INIT_DIR}/aegis-server restart
            fi
            echo -e "${GREEN}✓ 主控服务端已重启${PLAIN}"
            ;;
        2)
            if [ "$INIT_SYS" = "systemd" ]; then
                systemctl restart aegis-agent
            elif [ "$INIT_SYS" = "openrc" ]; then
                rc-service aegis-agent restart
            elif [ "$INIT_SYS" = "openwrt" ]; then
                ${OPENWRT_INIT_DIR}/aegis-agent restart
            fi
            echo -e "${GREEN}✓ 受控端已重启${PLAIN}"
            ;;
        3)
            if [ "$INIT_SYS" = "systemd" ]; then
                systemctl restart aegis-server 2>/dev/null || true
                systemctl restart aegis-agent 2>/dev/null || true
            elif [ "$INIT_SYS" = "openrc" ]; then
                rc-service aegis-server restart 2>/dev/null || true
                rc-service aegis-agent restart 2>/dev/null || true
            elif [ "$INIT_SYS" = "openwrt" ]; then
                ${OPENWRT_INIT_DIR}/aegis-server restart 2>/dev/null || true
                ${OPENWRT_INIT_DIR}/aegis-agent restart 2>/dev/null || true
            fi
            echo -e "${GREEN}✓ 全部探针服务已重启${PLAIN}"
            ;;
        *)
            return 0
            ;;
    esac
}

# 备份服务端配置数据 (导出当前目录)
backup_config() {
    local srv_dir="${INSTALL_DIR}/server"
    local cfg_file="${srv_dir}/data/config.json"
    if [ ! -f "$cfg_file" ]; then
        echo -e "${RED}❌ 未检测到服务端配置文件: ${cfg_file}${PLAIN}"
        return 1
    fi
    local backup_name="aegis_backup_$(date +%Y%m%d_%H%M%S).json"
    cp -f "$cfg_file" "./${backup_name}"
    echo -e "${GREEN}=================================================================${PLAIN}"
    echo -e "${GREEN}✅ 主控配置与全部节点数据已成功备份！${PLAIN}"
    echo -e "${CYAN}备份文件存放路径: $(pwd)/${backup_name}${PLAIN}"
    echo -e "${PLAIN}请妥善保存此文件，换机时直接导入即可瞬间复活所有探针连接！${PLAIN}"
    echo -e "${GREEN}=================================================================${PLAIN}"
}

# 恢复服务端配置数据 (从指定 json 文件恢复)
restore_config() {
    local target_file="$1"
    if [ -z "$target_file" ]; then
        read -rp "请输入备份文件完整路径 (例如 /root/aegis_backup.json): " target_file
    fi
    if [ ! -f "$target_file" ]; then
        echo -e "${RED}❌ 备份文件不存在: ${target_file}${PLAIN}"
        return 1
    fi

    local srv_dir="${INSTALL_DIR}/server"
    mkdir -p "${srv_dir}/data"
    cp -f "$target_file" "${srv_dir}/data/config.json"
    echo -e "${GREEN}✓ 备份文件已写入 ${srv_dir}/data/config.json${PLAIN}"

    detect_init_sys
    if [ "$INIT_SYS" = "systemd" ]; then
        systemctl restart aegis-server 2>/dev/null || true
    elif [ "$INIT_SYS" = "openrc" ]; then
        rc-service aegis-server restart 2>/dev/null || true
    elif [ "$INIT_SYS" = "openwrt" ]; then
        ${OPENWRT_INIT_DIR}/aegis-server restart 2>/dev/null || true
    fi
    echo -e "${GREEN}✅ 配置与全部节点已满血恢复并重启生效！${PLAIN}"
}

# 彻底卸载全部 (Master + Agent)
uninstall_all() {
    check_root
    detect_init_sys
    echo -e "${RED}=================================================================${PLAIN}"
    echo -e "${RED}⚠️  警告: 此操作将彻底卸载并清理 主控端 + 受控端 全部文件与服务！${PLAIN}"
    echo -e "${RED}=================================================================${PLAIN}"
    read -rp "确定要彻底卸载天阙·神盾全部探针吗？[y/N]: " confirm
    if [[ "$confirm" =~ ^[yY]$ ]]; then
        if [ "$INIT_SYS" = "systemd" ]; then
            systemctl stop aegis-server 2>/dev/null || true
            systemctl disable aegis-server 2>/dev/null || true
            rm -f "${SYSTEMD_DIR}/aegis-server.service"

            systemctl stop aegis-agent 2>/dev/null || true
            systemctl disable aegis-agent 2>/dev/null || true
            rm -f "${SYSTEMD_DIR}/aegis-agent.service"
            systemctl daemon-reload
        elif [ "$INIT_SYS" = "openrc" ]; then
            rc-service aegis-server stop 2>/dev/null || true
            rc-update del aegis-server default 2>/dev/null || true
            rm -f /etc/init.d/aegis-server
            rc-service aegis-agent stop 2>/dev/null || true
            rc-update del aegis-agent default 2>/dev/null || true
            rm -f /etc/init.d/aegis-agent
        elif [ "$INIT_SYS" = "openwrt" ]; then
            ${OPENWRT_INIT_DIR}/aegis-server stop 2>/dev/null || true
            ${OPENWRT_INIT_DIR}/aegis-server disable 2>/dev/null || true
            rm -f "${OPENWRT_INIT_DIR}/aegis-server"

            ${OPENWRT_INIT_DIR}/aegis-agent stop 2>/dev/null || true
            ${OPENWRT_INIT_DIR}/aegis-agent disable 2>/dev/null || true
            rm -f "${OPENWRT_INIT_DIR}/aegis-agent"
        fi
        pkill -9 -x aegis_server 2>/dev/null || true
        pkill -9 -x aegis_agent 2>/dev/null || true
        killall -9 aegis_server 2>/dev/null || true
        killall -9 aegis_agent 2>/dev/null || true
        rm -rf "$INSTALL_DIR"
        echo -e "${GREEN}✅ 已彻底清理并卸载全部探针组件。${PLAIN}"
    else
        echo -e "${CYAN}已取消。${PLAIN}"
    fi
}

# ==================== 命令行非交互式入口 ====================
while [[ $# -gt 0 ]]; do
    case "$1" in
        --master)
            shift
            while [[ $# -gt 0 ]]; do
                case "$1" in
                    --port=*) LISTEN_PORT="${1#*=}"; shift ;;
                    *) shift ;;
                esac
            done
            install_master
            exit 0
            ;;
        --update-master)
            update_master
            exit 0
            ;;
        --uninstall-master)
            uninstall_master
            exit 0
            ;;
        --agent)
            shift
            server=""
            id=""
            token=""
            while [[ $# -gt 0 ]]; do
                case "$1" in
                    --server=*) server="${1#*=}"; shift ;;
                    --id=*) id="${1#*=}"; shift ;;
                    --token=*) token="${1#*=}"; shift ;;
                    *) shift ;;
                esac
            done
            install_agent "$server" "$id" "$token"
            exit 0
            ;;
        --update-agent)
            update_agent_binary
            exit 0
            ;;
        --uninstall-agent)
            uninstall_agent
            exit 0
            ;;
        --uninstall-all)
            uninstall_all
            exit 0
            ;;
        --backup)
            backup_config
            exit 0
            ;;
        --restore=*)
            restore_config "${1#*=}"
            exit 0
            ;;
        *)
            shift
            ;;
    esac
done

# ==================== 交互式主菜单 ====================
show_menu() {
    detect_init_sys
    detect_arch
    clear
    echo -e "${CYAN}=================================================================${PLAIN}"
    echo -e "${CYAN}🛡️  天阙·神盾安全探针 (AegisProbe) 一键管理控制台 ${PLAIN}"
    echo -e "${CYAN}    脚本版本: ${SCRIPT_VERSION} | 机器架构: ${GOARCH} | 运行环境: ${INIT_SYS}${PLAIN}"
    echo -e "${CYAN}    安全准则: 纯单向推送 | 零入站端口 | 真实 IP 脱敏 | 支持软路由/VPS${PLAIN}"
    echo -e "${CYAN}=================================================================${PLAIN}"
    echo -e "${BLUE}【服务端管理 (Master 主控大屏与后台)】:${PLAIN}"
    echo -e " ${GREEN} 1.${PLAIN} 安装 探针服务端 (Master)"
    echo -e " ${GREEN} 2.${PLAIN} 更新 探针服务端 (Master 二进制热升级，保留配置)"
    echo -e " ${RED} 3.${PLAIN} 卸载 探针服务端 (Master 停止服务并清理)"
    echo -e " ${GREEN} 4.${PLAIN} 查看 服务端运行状态与管理员密码"
    echo -e "-----------------------------------------------------------------"
    echo -e "${BLUE}【受控端管理 (Agent 节点采集探针)】:${PLAIN}"
    echo -e " ${GREEN} 5.${PLAIN} 安装 探针受控端 (Agent 支持VPS/软路由)"
    echo -e " ${GREEN} 6.${PLAIN} 更新 探针受控端 (Agent 二进制热升级)"
    echo -e " ${RED} 7.${PLAIN} 卸载 探针受控端 (Agent 停止并卸载)"
    echo -e "-----------------------------------------------------------------"
    echo -e "${BLUE}【运维与数据维护】:${PLAIN}"
    echo -e " ${GREEN} 8.${PLAIN} 重启服务 (可选服务端/受控端)"
    echo -e " ${GREEN} 9.${PLAIN} 备份主控配置数据 (导出全部节点与设置到当前目录)"
    echo -e " ${GREEN}10.${PLAIN} 恢复主控配置数据 (从指定 json 备份文件一键恢复)"
    echo -e " ${RED}11.${PLAIN} 彻底清理卸载全部 (Master + Agent)"
    echo -e "-----------------------------------------------------------------"
    echo -e " ${BLUE} 0.${PLAIN} 退出脚本"
    echo -e "${CYAN}=================================================================${PLAIN}"
    read -rp "请输入选项编号 [0-11]: " num

    case "$num" in
        1) install_master ;;
        2) update_master ;;
        3) uninstall_master ;;
        4) view_server_status ;;
        5) install_agent ;;
        6) update_agent_binary ;;
        7) uninstall_agent ;;
        8) restart_services_menu ;;
        9) backup_config ;;
        10) restore_config ;;
        11) uninstall_all ;;
        0) exit 0 ;;
        *) echo -e "${RED}输入错误，请输入有效数字！${PLAIN}" ;;
    esac
}

show_menu
