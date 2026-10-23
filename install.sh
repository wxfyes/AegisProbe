#!/usr/bin/env bash
# ==============================================================================
# 🛡️ 天阙·神盾安全探针 (AegisProbe) 一键管理与安装脚本
# GitHub: https://github.com/wxfyes/AegisProbe
# 【特性】：零依赖、支持服务端与受控端、自动架构识别、国内镜像加速、nobody安全沙箱
# ==============================================================================

set -e

REPO="wxfyes/AegisProbe"
INSTALL_DIR="/opt/aegis-probe"
SYSTEMD_DIR="/etc/systemd/system"

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
            echo -e "${RED}❌ 下载失败，请检查网络或 GitHub 仓库 Release 发布状态！${PLAIN}"
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

    if [ "$GOARCH" != "amd64" ]; then
        echo -e "${YELLOW}⚠️ 当前架构为 ${GOARCH}，服务端官方发布包目前推荐 Linux amd64。若需其他架构请本地交叉编译。${PLAIN}"
    fi

    local bin_name="aegis_server_linux_amd64"
    local srv_dir="${INSTALL_DIR}/server"
    mkdir -p "${srv_dir}/data"
    mkdir -p "${srv_dir}/downloads"

    echo -e "${BLUE}=================================================================${PLAIN}"
    echo -e "${BLUE}🛡️ 开始安装 天阙·神盾 探针服务端 (Master)...${PLAIN}"
    echo -e "${BLUE}=================================================================${PLAIN}"

    # 停止旧服务
    if systemctl is-active --quiet aegis-server; then
        systemctl stop aegis-server
    fi

    download_file "$bin_name" "${srv_dir}/aegis_server"

    # 同时下载受控端到 downloads 供客户端一键拉取
    echo -e "${CYAN}📦 正在预载受控端安装包至本地服务目录...${PLAIN}"
    download_file "aegis_agent_linux_amd64" "${srv_dir}/downloads/aegis_agent_linux_amd64" || true
    download_file "aegis_agent_linux_arm64" "${srv_dir}/downloads/aegis_agent_linux_arm64" || true

    # 配置 systemd 服务
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
    echo -e "${PLAIN}提示: 请在防火墙/安全组开放 8989 TCP 端口。可在后台随时修改密码与访问模式。${PLAIN}"
}

# ==================== 2. 安装受控端 (Agent) ====================
install_agent() {
    check_root
    detect_arch

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
    echo -e "${BLUE}【安全铁律】：纯单向推送、强制 nobody 最小特权沙箱、零入站监听${PLAIN}"
    echo -e "${BLUE}=================================================================${PLAIN}"

    if systemctl is-active --quiet aegis-agent; then
        systemctl stop aegis-agent
    fi

    # 优先从本地服务端下载，失败再从 GitHub Release 备用下载
    local local_download_url="${server_url%/}/download/${bin_name}"
    echo -e "${CYAN}📥 正在从主控下载探针程序...${PLAIN}"
    if ! curl -fsSL --connect-timeout 5 -m 30 "$local_download_url" -o "${agent_dir}/aegis_agent" 2>/dev/null; then
        echo -e "${YELLOW}⚠️ 从主控直连下载失败，尝试从 GitHub Release 下载...${PLAIN}"
        download_file "$bin_name" "${agent_dir}/aegis_agent"
    fi
    chmod +x "${agent_dir}/aegis_agent"

    # 配置 systemd 安全沙箱服务 (nobody 运行)
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

    echo -e "${GREEN}=================================================================${PLAIN}"
    echo -e "${GREEN}✅ 天阙·神盾受控探针已启动！当前运行用户: nobody${PLAIN}"
    echo -e "${GREEN}🛡️ 数据正在单向汇报至: ${server_url}${PLAIN}"
    echo -e "${GREEN}=================================================================${PLAIN}"
}

# 查看服务端状态与密码
view_server_status() {
    local srv_dir="${INSTALL_DIR}/server"
    echo -e "${BLUE}==================== 服务端运行状态 ====================${PLAIN}"
    systemctl status aegis-server --no-pager || true
    if [ -f "${srv_dir}/data/config.json" ]; then
        local pwd
        pwd=$(grep -o '"admin_password": "[^"]*"' "${srv_dir}/data/config.json" | cut -d'"' -f4 || echo "")
        echo -e "${YELLOW}当前管理员密码: ${pwd}${PLAIN}"
    fi
    echo -e "${BLUE}========================================================${PLAIN}"
}

# 卸载
uninstall_all() {
    check_root
    echo -e "${YELLOW}确定要卸载天阙·神盾探针吗？(包含服务端和受控端) [y/N]: ${PLAIN}"
    read -r confirm
    if [[ "$confirm" =~ ^[yY]$ ]]; then
        systemctl stop aegis-server 2>/dev/null || true
        systemctl disable aegis-server 2>/dev/null || true
        rm -f "${SYSTEMD_DIR}/aegis-server.service"

        systemctl stop aegis-agent 2>/dev/null || true
        systemctl disable aegis-agent 2>/dev/null || true
        rm -f "${SYSTEMD_DIR}/aegis-agent.service"

        systemctl daemon-reload
        rm -rf "$INSTALL_DIR"
        echo -e "${GREEN}✅ 已彻底清理并卸载。${PLAIN}"
    fi
}

# ==================== 命令行非交互式直接传参入口 ====================
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
    clear
    echo -e "${CYAN}=================================================================${PLAIN}"
    echo -e "${CYAN}🛡️  天阙·神盾安全探针 (TianQue AegisProbe) 一键管理脚本          ${PLAIN}"
    echo -e "${CYAN}    零 GPU 负担 | 纯单向推送 | 真实 IP 脱敏 | 零渗透通道         ${PLAIN}"
    echo -e "${CYAN}=================================================================${PLAIN}"
    echo -e " ${GREEN}1.${PLAIN} 安装 / 更新 探针服务端 (Master 主控大屏与后台)"
    echo -e " ${GREEN}2.${PLAIN} 安装 / 更新 探针受控端 (Agent 被控节点)"
    echo -e "-----------------------------------------------------------------"
    echo -e " ${GREEN}3.${PLAIN} 查看服务端状态与管理员密码"
    echo -e " ${GREEN}4.${PLAIN} 重启服务端 (aegis-server)"
    echo -e " ${GREEN}5.${PLAIN} 重启受控端 (aegis-agent)"
    echo -e " ${GREEN}6.${PLAIN} 查看受控端实时日志"
    echo -e " ${RED}7.${PLAIN} 彻底卸载天阙·神盾探针"
    echo -e "-----------------------------------------------------------------"
    echo -e " ${BLUE}0.${PLAIN} 退出脚本"
    echo -e "${CYAN}=================================================================${PLAIN}"
    read -rp "请输入选项编号 [0-7]: " num

    case "$num" in
        1) install_master ;;
        2) install_agent ;;
        3) view_server_status ;;
        4) systemctl restart aegis-server && echo -e "${GREEN}服务端已重启${PLAIN}" ;;
        5) systemctl restart aegis-agent && echo -e "${GREEN}受控端已重启${PLAIN}" ;;
        6) journalctl -u aegis-agent -f ;;
        7) uninstall_all ;;
        0) exit 0 ;;
        *) echo -e "${RED}输入错误，请输入有效数字！${PLAIN}" ;;
    esac
}

show_menu
