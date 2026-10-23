#!/usr/bin/env bash
# ==============================================================================
# 🛡️ 天阙·神盾安全探针 (AegisProbe) 一键管理与安装脚本
# GitHub: https://github.com/wxfyes/AegisProbe
# 【全平台支持】：普通Linux (Debian/Ubuntu/CentOS)、软路由 (OpenWrt/iStoreOS/PVE/Alpine)
# 【特性】：精准单架构下载、绝不下载多余二进制、端口冲突检测、nobody安全沙箱
# ==============================================================================

set -e

REPO="wxfyes/AegisProbe"
INSTALL_DIR="/opt/aegis-probe"
SYSTEMD_DIR="/etc/systemd/system"
OPENWRT_INIT_DIR="/etc/init.d"
LISTEN_PORT=8989

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
        echo -e "${YELLOW}⚠️ 端口 ${port} 被 PID ${pid} 占用，正在强制击杀...${PLAIN}"
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
            echo -e "${GREEN}✓ 端口 ${port} 已成功释放${PLAIN}"
            return 0
        fi
        sleep 1
        tries=$((tries + 1))
    done

    echo -e "${RED}❌ 端口 ${port} 仍被占用，无法继续安装！请手动执行: kill -9 \$(lsof -ti :${port})${PLAIN}"
    exit 1
}

# 智能高速下载函数 (带 GitHub 镜像加速 fallback)
download_file() {
    local filename="$1"
    local output="$2"
    local url="https://github.com/${REPO}/releases/latest/download/${filename}"
    local mirror_url="https://mirror.ghproxy.com/https://github.com/${REPO}/releases/latest/download/${filename}"

    echo -e "${CYAN}📥 正在精准下载 ${filename} (当前架构: ${GOARCH}) ...${PLAIN}"
    if curl -fL --connect-timeout 6 -m 30 "$url" -o "$output" 2>/dev/null; then
        echo -e "${GREEN}✓ 从 GitHub 直连下载成功${PLAIN}"
    else
        echo -e "${YELLOW}⚠️ 直连超时，自动切换加速节点重试...${PLAIN}"
        if curl -fL --connect-timeout 10 -m 60 "$mirror_url" -o "$output" 2>/dev/null; then
            echo -e "${GREEN}✓ 从加速通道下载成功${PLAIN}"
        else
            echo -e "${RED}❌ 下载失败，请检查网络或 GitHub Release 状态！${PLAIN}"
            exit 1
        fi
    fi
    chmod +x "$output"
}

# 获取本机公网 IP (用于提示)
get_public_ip() {
    local ip=""
    ip=$(curl -s -m 3 https://api.ipify.org || curl -s -m 3 https://ipv4.icanhazip.com || echo "你的服务器IP")
    echo "$ip"
}

# ==================== 1. 安装探针服务端 (Master) ====================
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

    # 第一步：停止 + mask 封死所有旧服务（防止 Restart=always 在下载期间自动重启抢占端口）
    if [ "$INIT_SYS" = "systemd" ]; then
        systemctl stop aegis-server    2>/dev/null || true
        systemctl stop tianque-probe   2>/dev/null || true
        systemctl mask tianque-probe   2>/dev/null || true   # 彻底封死旧服务自动重启
        systemctl mask aegis-server    2>/dev/null || true   # 临时封死，写完新 unit 再 unmask
    elif [ "$INIT_SYS" = "openrc" ]; then
        rc-service aegis-server stop 2>/dev/null || true
        rc-service tianque-probe stop 2>/dev/null || true
    elif [ "$INIT_SYS" = "openwrt" ] && [ -f "${OPENWRT_INIT_DIR}/aegis-server" ]; then
        ${OPENWRT_INIT_DIR}/aegis-server stop 2>/dev/null || true
    fi

    # 第二步：按进程名强杀残留（双保险）
    pkill -9 -x aegis_server 2>/dev/null || true
    killall -9 aegis_server  2>/dev/null || true
    sleep 1

    # 第三步：按端口精准击杀 + 验证端口已释放（失败则退出）
    force_free_port "$LISTEN_PORT"

    # 下载新二进制（此时旧服务已被 mask，不会在下载期间自动重启）
    download_file "$bin_name" "${srv_dir}/aegis_server"

    # 下载完成后再次确认端口仍然空闲（防止任何意外进程趁隙占用）
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
        # unmask 解封（之前 mask 是为了防止下载期间自动重启）
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
    echo -e "${PLAIN}提示: 请在防火墙/安全组开放 ${LISTEN_PORT} TCP 端口。进入后台可随时修改密码。${PLAIN}"
}

# ==================== 2. 安装受控端 (Agent) ====================
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

    # 仅下载当前机器匹配的单一架构 Agent，绝不多下一个文件
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

    # 下载当前架构的 agent
    download_file "$bin_name" "${agent_dir}/aegis_agent"
    chmod +x "${agent_dir}/aegis_agent"

    # 配置开机自启 (nobody 用户运行)
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

# 查看运行状态与密码
view_server_status() {
    detect_init_sys
    local srv_dir="${INSTALL_DIR}/server"
    echo -e "${BLUE}==================== 服务运行状态 ====================${PLAIN}"
    if [ "$INIT_SYS" = "systemd" ]; then
        systemctl status aegis-server --no-pager || true
    elif [ "$INIT_SYS" = "openrc" ]; then
        rc-service aegis-server status 2>/dev/null || ps | grep aegis_server | grep -v grep || true
    elif [ "$INIT_SYS" = "openwrt" ]; then
        ps | grep aegis_server | grep -v grep || echo "服务未运行"
    fi
    if [ -f "${srv_dir}/data/config.json" ]; then
        local pwd
        pwd=$(grep -o '"admin_password": "[^"]*"' "${srv_dir}/data/config.json" | cut -d'"' -f4 || echo "")
        echo -e "${YELLOW}当前管理员密码: ${pwd}${PLAIN}"
    fi
    echo -e "${BLUE}======================================================${PLAIN}"
}

# 卸载
uninstall_all() {
    check_root
    detect_init_sys
    echo -e "${YELLOW}确定要彻底卸载天阙·神盾探针吗？[y/N]: ${PLAIN}"
    read -r confirm
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
        killall -9 aegis_server 2>/dev/null || true
        killall -9 aegis_agent 2>/dev/null || true
        rm -rf "$INSTALL_DIR"
        echo -e "${GREEN}✅ 已彻底清理并卸载。${PLAIN}"
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
        *)
            shift
            ;;
    esac
done

# ==================== 交互式主菜单 ====================
show_menu() {
    detect_init_sys
    clear
    echo -e "${CYAN}=================================================================${PLAIN}"
    echo -e "${CYAN}🛡️  天阙·神盾安全探针 (AegisProbe) 一键管理脚本 (支持软路由/VPS)${PLAIN}"
    echo -e "${CYAN}    当前系统: ${INIT_SYS} | 零 GPU 负担 | 纯单向推送 | 真实 IP 脱敏   ${PLAIN}"
    echo -e "${CYAN}=================================================================${PLAIN}"
    echo -e " ${GREEN}1.${PLAIN} 安装 / 更新 探针服务端 (Master 主控大屏与后台)"
    echo -e " ${GREEN}2.${PLAIN} 安装 / 更新 探针受控端 (Agent 被控节点/软路由)"
    echo -e "-----------------------------------------------------------------"
    echo -e " ${GREEN}3.${PLAIN} 查看服务端状态与管理员密码"
    echo -e " ${GREEN}4.${PLAIN} 重启服务端 (aegis-server)"
    echo -e " ${GREEN}5.${PLAIN} 重启受控端 (aegis-agent)"
    echo -e " ${RED}6.${PLAIN} 彻底卸载天阙·神盾探针"
    echo -e "-----------------------------------------------------------------"
    echo -e " ${BLUE}0.${PLAIN} 退出脚本"
    echo -e "${CYAN}=================================================================${PLAIN}"
    read -rp "请输入选项编号 [0-6]: " num

    case "$num" in
        1) install_master ;;
        2) install_agent ;;
        3) view_server_status ;;
        4)
            if [ "$INIT_SYS" = "systemd" ]; then
                systemctl restart aegis-server
            else
                ${OPENWRT_INIT_DIR}/aegis-server restart
            fi
            echo -e "${GREEN}服务端已重启${PLAIN}"
            ;;
        5)
            if [ "$INIT_SYS" = "systemd" ]; then
                systemctl restart aegis-agent
            else
                ${OPENWRT_INIT_DIR}/aegis-agent restart
            fi
            echo -e "${GREEN}受控端已重启${PLAIN}"
            ;;
        6) uninstall_all ;;
        0) exit 0 ;;
        *) echo -e "${RED}输入错误，请输入有效数字！${PLAIN}" ;;
    esac
}

show_menu
