# 🛡️ 天阙·神盾安全探针 (TianQue-Probe / AegisProbe)

> **专为高敏感节点集群打造的下一代轻量化、零 GPU 负担、零渗透通道、物理级 IP 脱敏的自研服务器探针监控系统。**

---

## 🌟 核心第一性原理与安全铁律

传统探针（如哪吒探针等）为了追求 Web 终端和远程脚本下发，普遍建立了 Agent 与 Master 之间的**双向反向控制通道**。一旦控制端遭受入侵，攻击者可向全网受控节点一键下发恶意后门，造成全网母鸡与落地机集体失陷。

天阙·神盾安全探针通过物理级架构创新彻底粉碎这一安全隐患：

1. 🛡️ **绝对物理级单向通信（Pure Push-Only）**：
   - 受控端（Agent）**零端口监听**，不暴露任何公网/内网入站端口，彻底隐匿于网络扫描之外；
   - 服务端代码与通信协议中，**100% 不定义、不实现、不预留任何远程命令执行接口**，即便服务端遭遇极端渗透，黑客也无法触碰任何节点。
2. 🔒 **全网真实 IP 物理级脱敏（盲盒式防护）**：
   - 大屏前端与对外 API 彻底剔除公网真实 IP、MAC 地址与内网拓扑，仅输出节点别名、国旗、系统图标与硬件负载，防止同行打击与定点阻断。
3. ⚡ **零 GPU 负担与低配丝滑 60FPS**：
   - 坚决杜绝 Three.js / WebGL / Canvas 连续重绘，坚决不滥用全屏高斯模糊；
   - 采用预烘焙半透明叠加（RGBA）与 CSS `transform: scaleX(...)` 硬件合成加速层，在手机、低配工控机和各类安卓模拟器中秒开流畅。
4. 🤖 **Telegram 智能告警中枢**：
   - 全异步缓冲队列调度，支持 Socks5/HTTP 网络代理；
   - 掉线防抖告警、上线稳定恢复通知、节点账单到期阶梯式预警（提前 7天、3天、1天及当天）。
5. 🔑 **双模访问控制与极简运维（KISS 原则）**：
   - 公开模式（免密展示）与私密模式（毛玻璃密码锁屏）一键切换；
   - 密码直接通用管理员密码，内置防暴力破解（连续输错 5 次拉黑 IP 15 分钟）。

---

## 📁 目录工程架构

```text
TianQue-Probe/
├── cmd/
│   ├── server/                   # 服务端主入口 (main.go)
│   └── agent/                    # 受控端 Agent 主入口 (main.go)
├── internal/
│   ├── common/                   # 数据通信协议与脱敏模型
│   ├── server/                   # 服务端核心组件 (配置持久化、内存状态机、网关、TG引擎)
│   │   └── web/dist/             # go:embed 纯静态内嵌大屏与控制台页面
│   └── agent/                    # 受控端核心采集 (无特权 /proc 与跨平台适配)
├── scripts/
│   ├── install_agent.sh          # Linux 一键安全部署脚本 (配置 nobody 沙箱服务)
│   └── build_all.ps1             # 全架构 10 秒纯静态交叉编译脚本
├── release_bins/                 # 预编译静态二进制包发布目录
├── WHITEPAPER.md                 # 架构白皮书
└── README.md
```

---

## 🚀 快速上手与运行

### 1. 本地极速编译全架构二进制

项目采用纯 Go 静态无 CGO 编译，零外部动态依赖，执行脚本 10 秒内即可输出全平台文件：

```powershell
powershell -ExecutionPolicy Bypass -File scripts\build_all.ps1
```

编译产物将存放在 `release_bins/` 目录：
- `aegis_server_linux_amd64` / `aegis_server_windows_amd64.exe` (服务端)
- `downloads/aegis_agent_linux_amd64` / `aegis_agent_linux_arm64` / `aegis_agent_linux_386` (受控端)

---

### 2. 启动服务端 (Master)

```bash
# 启动服务端 (默认监听 8989 端口，数据自动持久化到 data/config.json)
./aegis_server_linux_amd64 -port=8989
```

- **监控大屏**：`http://你的服务器IP:8989`
- **管理控制台**：`http://你的服务器IP:8989/admin.html`
- *首次启动时终端会随机生成初始高强度管理员密码，也可在 `data/config.json` 或后台中直接修改。*

---

### 3. 受控端 (Agent) 一键安全部署

进入管理控制台（`/admin.html`），在节点列表中点击 **【一键命令】** 即可复制专属安装命令，登录目标 VPS 执行：

```bash
curl -fsSL http://你的主控IP:8989/install.sh | bash -s -- --server=http://你的主控IP:8989 --id=node_01 --token=YOUR_TOKEN
```

> **安全沙箱保证**：安装脚本自动创建 `nobody:nogroup` 系统受限用户并配置 systemd 沙箱（`NoNewPrivileges=true`, `ProtectSystem=strict`），以最低权限读取系统状态，无入站端口。

---

## 🔒 渗透攻防验证

天阙·神盾探针在通信链路与数据协议层面进行了物理隔离：
- 服务端 `/api/report` 仅接收数据，响应中不包含任何控制指令；
- Agent 纯单向 HTTP POST，请求发送完成后直接 `io.Copy(io.Discard, resp.Body)` 丢弃，代码层面无任何 `os/exec`、反向 Shell 或 RPC 模块；
- 彻底斩断传统探针“主控被攻破导致集群全灭”的致命威胁。
