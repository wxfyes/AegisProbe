#!/usr/bin/env bash
# ==============================================================================
# 🛡️ 天阙·神盾安全探针 (AegisProbe) 一键管理与安装脚本
# GitHub: https://github.com/wxfyes/AegisProbe
# 【全平台支持】：普通Linux (Debian/Ubuntu/CentOS)、软路由 (OpenWrt/iStoreOS/PVE/Alpine)
# 【特性】：零依赖、Musl/Glibc全自洽、自动识别systemd/procd、nobody安全沙箱
# ==============================================================================

set -e

REPO="wxfyes/AegisProbe"
INSTALL_DIR="/opt/aegis-probe"
SYSTEMD_DIR="/etc/systemd/system"
OPENWRT_INIT_DIR="/etc/init.d"

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

# 架构检测
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

# 服务管理器检测 (Systemd vs OpenWrt procd vs 通用后台)
detect_init_sys() {
    if command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; then
        INIT_SYS="systemd"
    elif [ -f /etc/openwrt_release ] || [ -f /etc/init.d/procd ] || command -v procd >/dev/null 2>&1; then
        INIT_SYS="openwrt"
    else
        INIT_SYS="generic"
    fi
}

# 智能高速下载函数 (带 GitHub 镜像加速 fallback)
download_file() {
    local filename="$1"
    local output="$2"
    local url="https://github.com/${REPO}/releases/latest/download/${filename}"
    local mirror_url="https://mirror.ghproxy.com/https://github.com/${REPO}/releases/latest/download/${filename}"

    echo -e "${CYAN}📥 正在下载 ${filename} ...${PLAIN}"
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
    ip=$(curl -s -m 3 https://api.ipify.org || curl -s -m 3 https://ipv4.icanhazip.com || echo "你的路由器/服务器IP")
    echo "$ip"
}

# ==================== 1. 安装探针服务端 (Master) ====================
install_master() {
    check_root
    detect_arch
    detect_init_sys

    local bin_name="aegis_server_linux_amd64"
    local srv_dir="${INSTALL_DIR}/server"
    mkdir -p "${srv_dir}/data"
    mkdir -p "${srv_dir}/downloads"

    echo -e "${BLUE}=================================================================${PLAIN}"
    echo -e "${BLUE}🛡️ 开始安装 天阙·神盾 探针服务端 (Master)...${PLAIN}"
    echo -e "${BLUE}检测到运行环境: ${INIT_SYS} (架构: ${GOARCH})${PLAIN}"
    echo -e "${BLUE}=================================================================${PLAIN}"

    # 停止旧服务
    if [ "$INIT_SYS" = "systemd" ]; then
        systemctl stop aegis-server 2>/dev/null || true
    elif [ "$INIT_SYS" = "openwrt" ] && [ -f "${OPENWRT_INIT_DIR}/aegis-server" ]; then
        ${OPENWRT_INIT_DIR}/aegis-server stop 2>/dev/null || true
    fi

    download_file "$bin_name" "${srv_dir}/aegis_server"

    # 同时下载受控端到 downloads 供客户端一键拉取
    echo -e "${CYAN}📦 正在预载受控端安装包至本地服务目录...${PLAIN}"
    download_file "aegis_agent_linux_amd64" "${srv_dir}/downloads/aegis_agent_linux_amd64" || true
    download_file "aegis_agent_linux_arm64" "${srv_dir}/downloads/aegis_agent_linux_arm64" || true

    # 根据系统类型配置自启服务
    if [ "$INIT_SYS" = "systemd" ]; then
        cat <<EOF > "${SYSTEMD_DIR}/aegis-server.service"
[Unit]
Description=TianQue Aegis Probe Master Server
After=network.target

[Service]
Type=simple
WorkingDirectory=${srv_dir}
ExecStart=${srv_dir}/aegis_server -port=8989 -bin-dir=${srv_dir}/downloads
Restart=always
RestartSec=3
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
EOF
        systemctl daemon-reload
        systemctl enable aegis-server
        systemctl restart aegis-server

    elif [ "$INIT_SYS" = "openwrt" ]; then
        cat <<EOF > "${OPENWRT_INIT_DIR}/aegis-server"
#!/bin/sh /etc/rc.common
START=99
STOP=10
USE_PROCD=1

PROG=${srv_dir}/aegis_server

start_service() {
    procd_open_instance
    procd_set_param command "\$PROG" -port=8989 -bin-dir=${srv_dir}/downloads
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
        nohup ${srv_dir}/aegis_server -port=8989 -bin-dir=${srv_dir}/downloads >/dev/null 2>&1 &
    fi

    # 等待生成初始密码
    sleep 2

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
    echo -e "${CYAN}👉 监控大屏地址  : ${PLAIN}http://${server_ip}:8989"
    echo -e "${CYAN}👉 管理控制台地址: ${PLAIN}http://${server_ip}:8989/admin.html"
    echo -e "${YELLOW}👉 初始管理员密码: ${PLAIN}${admin_pwd}"
    echo -e "${PLAIN}=================================================================${PLAIN}"
    echo -e "${PLAIN}提示: 请在软路由或防火墙中开放 8989 TCP 端口。可在后台随时修改密码与访问模式。${PLAIN}"
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
        read -rp "节点唯一标识 (例如 router-01): " node_id
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
    echo -e "${BLUE}检测到运行环境: ${INIT_SYS} (硬件架构: ${GOARCH})${PLAIN}"
    echo -e "${BLUE}【安全铁律生效】：纯单向推送、零入站端口、不影响软路由 NAT 转发${PLAIN}"
    echo -e "${BLUE}=================================================================${PLAIN}"

    if [ "$INIT_SYS" = "systemd" ]; then
        systemctl stop aegis-agent 2>/dev/null || true
    elif [ "$INIT_SYS" = "openwrt" ] && [ -f "${OPENWRT_INIT_DIR}/aegis-agent" ]; then
        ${OPENWRT_INIT_DIR}/aegis-agent stop 2>/dev/null || true
    fi

    # 优先从主控下载，若超时则从 GitHub Release 下载
    local local_download_url="${server_url%/}/download/${bin_name}"
    echo -e "${CYAN}📥 正在下载探针程序...${PLAIN}"
    if ! curl -fsSL --connect-timeout 5 -m 30 "$local_download_url" -o "${agent_dir}/aegis_agent" 2>/dev/null; then
        echo -e "${YELLOW}⚠️ 从主控直连下载失败，自动切换至 GitHub Release 镜像通道...${PLAIN}"
        download_file "$bin_name" "${agent_dir}/aegis_agent"
    fi
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
WorkingDirectory=${agent_dir}
ExecStart=${agent_dir}/aegis_agent -server=${server_url} -id=${node_id} -token=${node_token}
Restart=always
RestartSec=3
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true

[Install]
WantedBy=multi-user.target
EOF
        systemctl daemon-reload
        systemctl enable aegis-agent
        systemctl restart aegis-agent

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
    echo -e "${YELLOW}确定要卸载天阙·神盾探针吗？(包含服务端和受控端) [y/N]: ${PLAIN}"
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
if [ "$1" = "--master" ]; then
    install_master
    exit 0
elif [ "$1" = "--agent" ]; then
    shift
    server=""
    id=""
    token=""
    while [[ $# -gt 0 ]]; do
        case $1 in
            --server=*) server="${1#*=}"; shift ;;
            --id=*) id="${1#*=}"; shift ;;
            --token=*) token="${1#*=}"; shift ;;
            *) shift ;;
        esac
    done
    install_agent "$server" "$id" "$token"
    exit 0
fi

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
