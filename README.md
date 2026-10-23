# 🛡️ 天阙·神盾安全探针 (AegisProbe / TianQue-Probe)

<div align="center">

![License](https://img.shields.io/badge/license-MIT-blue.svg)
![Go Version](https://img.shields.io/badge/Go-1.21%2B-00ADD8?logo=go)
![Architecture](https://img.shields.io/badge/Arch-AMD64%20%7C%20ARM64%20%7C%20386-orange)
![Security](https://img.shields.io/badge/Security-Pure%20Push--Only-brightgreen)
![GPU Load](https://img.shields.io/badge/GPU%20Load-0%25%20(No%20WebGL)-success)

**专为高敏感节点集群打造的下一代轻量化、零 GPU 负担、零下行渗透通道、物理级 IP 脱敏的自研服务器探针系统**

[白皮书文档](WHITEPAPER.md) • [快速安装](#-快速上手与部署) • [后台配置](#-管理后台与权限体系) • [Telegram 告警](#-telegram-智能告警中枢) • [安全白盒](#-四大安全铁律)

</div>

---

## 📖 立项背景与第一性原理

在当前的服务器探针生态中（如哪吒探针、ServerStatus 等），集群运维普遍面临致命安全威胁：

```
【传统探针高危架构】
+-----------------------+                    +------------------------+
|   受控 VPS 节点       | <================> |   探针服务端 (Master)   | <--- 黑客攻击
| (以 Root 最高特权运行) |    双向RPC / WebSSH| (暴露后台管理与插件系统)|
+-----------------------+    远程命令下发通道 +------------------------+
                                                         |
                                 💥 一旦控制端失陷，黑客通过下行通道一键全灭，
                                    全网母鸡、中转机、落地机全部被获取 Root 权限！
```

### 传统探针的致命痛点
1. **双向下行通道（RCE 远程执行温床）**：主控端与 Agent 之间保留反向控制、WebSSH、脚本下发通道。主控一旦被渗透，全网节点集体沦陷；
2. **公网真实 IP 裸奔**：API 接口明文返回每台节点的公网 IP，直接向攻击者提供 DDoS 打击靶子与封锁线索；
3. **过度滥用 GPU 渲染**：滥用 3D WebGL、Canvas 连续重绘、多层毛玻璃高斯模糊（`backdrop-filter: blur(40px)`），导致低配设备与安卓模拟器显存飙升、掉帧卡死；
4. **臃肿依赖**：引入动态脚本沙箱、数据库中间件，带来未知的内存开销与攻击面。

---

## 🛡️ 天阙·神盾的四大安全铁律

```
【天阙·神盾 (AegisProbe) 零渗透单向架构】
+-----------------------+                    +------------------------+
|   被控 VPS 节点       |                    |   探针服务端 (Master)   |
| (以 nobody 普通用户运行)|                    | (纯静态嵌入 + 内存时序) |
|                       |                    +------------------------+
| 仅读取系统 /proc 状态 |                                ▲
|                       |                                |
| 物理单向 POST 推送数据 --------------------------------+
| (无入站端口/无监听/无RPC)        [纯单向汇报，代码层面 100% 无任何命令执行模块]
+-----------------------+                                ▲
                                                         |
                                 🛡️ 即使主控遭遇极端入侵，黑客拿到的也仅是一堆
                                    脱敏的负载数字，在网络与协议上绝对无法逆向触碰节点！
```

* **铁律 1：绝对物理级单向推送（Pure Push-Only）**
  - 受控端（Agent）**零端口监听**，不开放任何公网/内网入站端口，彻底隐匿于网络扫描之外；
  - 服务端与协议中，**彻底不定义、不实现、不预留任何反向控制、WebSSH、脚本下发代码**。
* **铁律 2：真实 IP 物理级脱敏（盲盒式防护）**
  - 前端大屏及公开 API 仅输出节点别名、国旗图标、系统图标、硬件规格与实时负载；
  - 服务端向外返回的接口中**物理级剔除 IP、MAC 地址、私网拓扑等敏感元数据**，爬虫与黑客无法测绘节点。
* **铁律 3：Agent 最小特权原则（强制非 Root 运行）**
  - 读取 Linux 系统负载（`/proc/stat`、`/proc/meminfo`、`/proc/net/dev`、`/proc/loadavg`）完全无需 Root 特权；
  - 安装脚本默认创建受限普通用户 `nobody:nogroup`，并配置 systemd 沙箱（`NoNewPrivileges=true`, `ProtectSystem=strict`）。
* **铁律 4：纯 Go 单一静态二进制构建（零外部动态依赖）**
  - 采用 `CGO_ENABLED=0` 纯静态交叉编译，零外部动态链接库依赖；
  - 前端大屏与后台页面通过 Go 1.16+ `go:embed` 全部内嵌至单个二进制中，开箱即用。

---

## 🎨 视觉美学与“零 GPU 负担”前端工程

为了在**“界面极其美观（Komari 极简精致卡片）”**与**“绝不过度消耗 GPU”**之间取得黄金平衡：

* ❌ **坚决不用 WebGL / 3D 渲染**：彻底杜绝 Three.js / Canvas 连续重绘，显卡占用归零；
* ❌ **坚决不用高半径高斯模糊滥用**：采用**预烘焙半透明色彩叠加（RGBA）**与硬件合成加速层，消除全屏离屏重绘发热；
* ⚡ **动画合成加速**：所有进度条与网速刷新统一使用 CSS `transform: scaleX(...)`，杜绝触发布局回流（Layout Reflow）；
* 🌓 **双主题平滑切换**：默认支持暗黑模式（Dark Mode）与明亮模式（Light Mode）无感切换；
* 📱 **低配秒开 60FPS**：在手机端、安卓模拟器（雷电/夜神/MuMu）及低配工控机上丝滑满帧运行。

---

## 🚀 快速上手与部署

### 方式一：使用预编译二进制运行（推荐）

#### 1. 启动服务端 (Master)
下载对应系统架构的服务端二进制（位于 `release_bins/` 目录），直接运行：

```bash
# Linux AMD64 运行 (指定端口为 8989)
chmod +x aegis_server_linux_amd64
./aegis_server_linux_amd64 -port=8989
```

- **访问监控大屏**：`http://你的服务器IP:8989`
- **访问管理控制台**：`http://你的服务器IP:8989/admin.html`
- *首次启动时服务端会在终端随机生成初始管理员密码，数据自动原子保存于 `data/config.json`。*

#### 2. 受控端 (Agent) 一键安全安装
登录管理控制台（`/admin.html`），在节点列表中点击 **【一键命令】** 复制生成的专用安装指令：

```bash
curl -fsSL http://你的主控IP:8989/install.sh | bash -s -- --server=http://你的主控IP:8989 --id=hk-01 --token=YOUR_TOKEN
```

> **系统服务保护说明**：一键脚本会自动检测系统架构（x86_64 / aarch64 / i386），下载对应二进制，并配置 systemd 受限服务 `tianque-probe.service`（以 `nobody` 用户运行）。

---

### 方式二：本地 10 秒极速全架构编译

本项目采用纯 Go 静态编译，零 CGO，零第三方外部库，本地全架构交叉编译仅需 10 秒：

```powershell
# Windows PowerShell 下执行
powershell -ExecutionPolicy Bypass -File scripts\build_all.ps1
```

输出目录 `release_bins/` 包含以下产物：
- `aegis_server_linux_amd64` (Linux 服务端)
- `aegis_server_windows_amd64.exe` (Windows 服务端)
- `downloads/aegis_agent_linux_amd64` (Linux 64位受控端)
- `downloads/aegis_agent_linux_arm64` (Linux ARM64/aarch64 受控端)
- `downloads/aegis_agent_linux_386` (Linux 32位受控端)
- `downloads/aegis_agent_windows_amd64.exe` (Windows 受控端)

---

## 🔐 管理后台与双模权限体系

### 1. 双模访问控制（KISS 极简设计）
探针大屏支持“公开展示”与“密码私密”两种模式，在后台一键切换：

| 模式 | 表现行为 | 适用场景 |
| :--- | :--- | :--- |
| **公开模式 (Public)** | 任何人无需输入密码，直接打开网址浏览脱敏后的状态大屏。 | 商业化公开展示、客户服务测速、节点可用性透明化展板。 |
| **私密模式 (Private)** | 访客打开网址时，呈现毛玻璃密码解锁框。**密码直接通用管理员密码**，输入后无感解锁。 | 个人私密监控、中转/机房内部运维不公开。 |

* **安全防护**：私密模式下，数据接口（`/api/nodes/summary`）强制进行 Session 校验，杜绝未经授权直接抓取接口；内置防暴力破解（单个 IP 连续输错 5 次密码封禁 15 分钟）。

### 2. 节点与分组管理
- 添加/编辑节点：名称、国旗 Emoji（🇭🇰/🇹🇼/🇯🇵/🇺🇸 等）、操作系统图标、所属分组（中转/亚洲/直连/欧洲/DNS 等）；
- 计费与到期：设置月付价格（如 `$29.99/月`）与续费到期时间（卡片智能显示剩余天数：绿 >15天、黄 3~15天、红 <3天）；
- 自动生成唯一对接 Token 与一键复制部署命令。

---

## 🤖 Telegram 智能告警中枢

服务端内置轻量级纯 Go 实现的 Telegram 消息中枢，无需 Python 或任何第三方中继：

```
+-----------------------------------------------------------------------------+
| 🤖 Telegram 智能告警消息示例                                                 |
+-----------------------------------------------------------------------------+
| 🔴【节点掉线报警】                                                          |
| 节点名称：香港转发-225                                                      |
| 节点分组：中转                                                              |
| 离线时间：2026-10-23 19:30:15                                               |
| 告警说明：已超过 60 秒未收到上报，请尽快排查！                              |
|                                                                             |
| 🟢【节点上线恢复】                                                          |
| 节点名称：香港转发-225                                                      |
| 节点分组：中转                                                              |
| 恢复时间：2026-10-23 19:35:22                                               |
| 系统状态：负载 0.05 | 内存 21.0%                                            |
|                                                                             |
| ⚠️【节点到期提醒】                                                          |
| 节点名称：香港-30M-122                                                      |
| 当前计费：$29.99/月                                                         |
| 到期时间：2026-11-04 (仅剩 12 天)                                           |
| 友情提示：请注意及时续费，防止机器停机造成业务中断！                        |
+-----------------------------------------------------------------------------+
```

### 稳健性与防抖特性
1. **防抖告警（Anti-Flapping）**：掉线仅告警 1 次；恢复时需连续稳定上报 2 次以上才推送“恢复通知”，避免网络抖动刷屏；
2. **阶梯式账单预警**：每日自动比对到期时间，在到期前 **7 天、3 天、1 天** 及 **当天** 阶梯预警；
3. **全异步无阻塞**：所有消息推入 Go 内存缓冲队列，由独立协程异步发送，Telegram 波动绝不拖慢主控；
4. **原生网络代理支持**：支持配置 `Socks5` 或 `HTTP` 代理（如 `socks5://127.0.0.1:10808`），确保国内无外网 VPS 稳定推送。

---

## 📡 接口与脱敏数据结构

### 1. Agent 纯单向上报接口 (`POST /api/report`)
Agent 每 2 秒单向 POST 一次紧凑 JSON（传输体量 < 1KB，网络带宽消耗几乎为零），服务端响应不包含任何远程控制指令：

```json
{
  "node_id": "node_01",
  "auth_token": "aegis_token_sha256",
  "timestamp": 1729681646,
  "metrics": {
    "cpu_percent": 1.2,
    "memory_used": 843284480,
    "memory_total": 4123168768,
    "swap_used": 0,
    "swap_total": 0,
    "disk_used": 4423680000,
    "disk_total": 31665487872,
    "net_in_speed": 14448,
    "net_out_speed": 12779,
    "net_in_total": 106161100000,
    "net_out_total": 102341000000,
    "load_1": 0.00,
    "load_5": 0.01,
    "load_15": 0.00,
    "uptime_seconds": 568800,
    "cpu_cores": 4,
    "os": "debian"
  }
}
```

### 2. 大屏脱敏展示接口 (`GET /api/nodes/summary`)
返回的数据结构中**物理级剔除 IP、MAC 地址、私有网段**，完全脱敏：

```json
{
  "site_title": "天阙·神盾安全探针",
  "summary": {
    "current_time": "06:07:26",
    "online_count": 31,
    "total_count": 33,
    "region_count": 18,
    "total_net_out_bytes": 38328481234944,
    "total_net_in_bytes": 36240294871040,
    "instant_speed_out": 3103784,
    "instant_speed_in": 3051290
  },
  "groups": ["所有", "中转", "亚洲", "直连"],
  "nodes": [
    {
      "id": "node_01",
      "name": "香港-30M-122",
      "region": "HK",
      "flag": "🇭🇰",
      "os": "debian",
      "group": "中转",
      "billing": { "price": "$29.99/月", "days_left": 13, "expire_date": "2026/11/05" },
      "hardware": { "cpu_cores": 4, "ram_str": "3.84 GB", "disk_str": "29.49 GB" },
      "status": {
        "is_online": true,
        "cpu_pct": 1.2,
        "ram_pct": 20.4,
        "swap_pct": 0,
        "swap_off": true,
        "disk_pct": 14.0,
        "speed_up_str": "12.48 KB/s",
        "speed_down_str": "14.11 KB/s",
        "traffic_up_str": "95.31 GB",
        "traffic_down_str": "98.87 GB",
        "load_str": "0.00 | 0.01 | 0.00",
        "uptime_str": "在线: 6天13小时",
        "last_seen_sec": 0
      }
    }
  ]
}
```

---

## ❓ 常见问题 (FAQ)

<details>
<summary><b>Q1: 为什么被控端 Agent 不需要 Root 特权？</b></summary>
Linux 核心设计中，读取 <code>/proc/stat</code>、<code>/proc/meminfo</code>、<code>/proc/net/dev</code>、<code>/proc/loadavg</code> 等系统状态信息是全系统通用只读权限，任何受限普通用户（如 <code>nobody</code>）均可直接读取。坚决使用非 Root 用户运行能杜绝程序潜在漏洞导致的提权风险。
</details>

<details>
<summary><b>Q2: 控制端如果被黑客攻破，受控节点是否安全？</b></summary>
绝对安全。在网络层面上，Agent 零端口监听，防火墙仅允许外出 HTTP POST；在代码层面上，服务端和 Agent 100% 没有任何远程执行、脚本下发或 WebSSH 代码。攻击者即使拿到服务端的完整权限，也无法向任何受控机器逆向发送任何命令。
</details>

<details>
<summary><b>Q3: 如何重置管理员密码？</b></summary>
直接打开服务端目录下的 <code>data/config.json</code>，找到 <code>admin_password</code> 字段修改为你所需的密码并保存，重启服务或在后台更新即可生效。
</details>

---

## 📄 开源许可证

本项目基于 [MIT 许可证](LICENSE) 开源。
