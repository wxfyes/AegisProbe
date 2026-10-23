#!/usr/bin/env bash
# ==============================================================================
# 🛡️ 天阙·神盾安全探针 (TianQue-Probe) 一键部署脚本
# 【安全铁律】：纯单向推送、强制配置 nobody 非特权沙箱运行、零入站监听
# ==============================================================================
set -e

SERVER_URL=""
NODE_ID=""
TOKEN=""

while [[ $# -gt 0 ]]; do
  case $1 in
    --server=*)
      SERVER_URL="${1#*=}"
      shift
      ;;
    --id=*)
      NODE_ID="${1#*=}"
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

if [ -z "$SERVER_URL" ] || [ -z "$NODE_ID" ] || [ -z "$TOKEN" ]; then
  echo "❌ 参数缺失！使用规范: bash install_agent.sh --server=http://主控IP:8989 --id=hk-01 --token=YOUR_TOKEN"
  exit 1
fi

echo "================================================================="
echo "🛡️ 开始部署天阙·神盾 (TianQue Aegis Probe) 受控端..."
echo "👉 主控端地址: $SERVER_URL"
echo "👉 节点标识符: $NODE_ID"
echo "================================================================="

ARCH=$(uname -m)
case $ARCH in
  x86_64)
    BIN_NAME="aegis_agent_linux_amd64"
    ;;
  aarch64|arm64)
    BIN_NAME="aegis_agent_linux_arm64"
    ;;
  i386|i686)
    BIN_NAME="aegis_agent_linux_386"
    ;;
  *)
    echo "❌ 暂不支持的架构: $ARCH"
    exit 1
    ;;
esac

INSTALL_DIR="/opt/tianque-probe"
mkdir -p "$INSTALL_DIR"
cd "$INSTALL_DIR"

echo "📥 正在从主控下载二进制探针文件 (${BIN_NAME})..."
curl -fsSL "${SERVER_URL}/download/${BIN_NAME}" -o aegis_agent || {
  echo "❌ 下载失败！请确保服务端 ${SERVER_URL} 正常运行且网络通常。"
  exit 1
}
chmod +x aegis_agent

echo "⚙️ 配置 systemd 安全沙箱服务 (最小特权 nobody 运行)..."
cat <<EOF > /etc/systemd/system/tianque-probe.service
[Unit]
Description=TianQue Aegis Probe Agent
After=network.target

[Service]
Type=simple
User=nobody
Group=nogroup
WorkingDirectory=${INSTALL_DIR}
ExecStart=${INSTALL_DIR}/aegis_agent -server=${SERVER_URL} -id=${NODE_ID} -token=${TOKEN}
Restart=always
RestartSec=5
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable tianque-probe
systemctl restart tianque-probe

echo "================================================================="
echo "✅ 天阙·神盾探针已成功安装并以 nobody 用户安全启动！"
echo "🛡️ 【安全铁律已生效】：未开启任何监听端口，无远程下行执行通道！"
echo "================================================================="
systemctl status tianque-probe --no-pager
