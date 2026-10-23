## 🛡️ 天阙·神盾安全探针 (AegisProbe) 正式发布

专为高敏感节点集群打造的下一代**轻量化、零 GPU 负担、零下行渗透通道、物理级 IP 脱敏**的自研服务器探针系统。

---

### 🚀 极速一键安装指令 (Linux VPS 推荐)

#### 1. 一键安装主控服务端 (Master)
```bash
curl -fsSL https://github.com/wxfyes/AegisProbe/releases/latest/download/install.sh | bash -s -- --master
```
*(国内 VPS 极速镜像: `curl -fsSL https://mirror.ghproxy.com/https://github.com/wxfyes/AegisProbe/releases/latest/download/install.sh | bash -s -- --master`)*

安装完成后即可访问：
- 监控大屏：`http://你的服务器IP:8989`
- 管理后台：`http://你的服务器IP:8989/admin.html`

#### 2. 一键安装受控端 (Agent)
在管理后台点击【一键命令】复制生成的专属指令，或执行：
```bash
curl -fsSL https://github.com/wxfyes/AegisProbe/releases/latest/download/install.sh | bash -s -- --agent --server=http://主控IP:8989 --id=node_01 --token=YOUR_TOKEN
```

#### 3. 交互式多功能管理菜单
```bash
bash <(curl -fsSL https://github.com/wxfyes/AegisProbe/releases/latest/download/install.sh)
```

---

### 📦 资产二进制清单
- `aegis_server_linux_amd64`：Linux 主控服务端（纯静态二进制，内嵌大屏与后台）
- `aegis_server_windows_amd64.exe`：Windows 主控服务端
- `aegis_agent_linux_amd64`：Linux 64位受控端
- `aegis_agent_linux_arm64`：Linux ARM64/aarch64 受控端
- `aegis_agent_linux_386`：Linux 32位受控端
- `aegis_agent_windows_amd64.exe`：Windows 64位受控端
- `install.sh`：服务端与受控端通用一键管理脚本
