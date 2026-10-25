# 🛡️ 天阙·神盾安全探针 (AegisProbe / TianQue-Probe)

<div align="center">

![License](https://img.shields.io/badge/license-MIT-blue.svg)
![Go Version](https://img.shields.io/badge/Go-1.21%2B-00ADD8?logo=go)
![Architecture](https://img.shields.io/badge/Arch-AMD64%20%7C%20ARM64%20%7C%20386-orange)
![Security](https://img.shields.io/badge/Security-Pure%20Push--Only-brightgreen)
![GPU Load](https://img.shields.io/badge/GPU%20Load-0%25%20(No%20WebGL)-success)

**轻量化服务器探针系统，支持 GitHub Actions 签名发布和逐节点 Agent 更新。**

## Agent 签名更新

- 首次接入需在管理员后台逐台运行「一键命令」，安装本地更新助手；此后可在后台逐台登记更新请求。
- 更新助手只轮询固定更新状态，从 GitHub Release 下载 Agent；验证 GitHub Actions 的 Sigstore 身份、签名清单和 SHA-256 后，才替换 Agent 并重启固定服务。
- GitHub Actions 使用 OIDC 临时身份签名，无需在本地保存发布私钥。
- 主控不能下发 Shell、脚本或任意命令。若 GitHub 仓库、发布工作流或 Sigstore 信任链被攻破，仍可能产生恶意更新风险。

---

**v1.3.3 修复**：Agent 更新签名清单改用 Sigstore 标准 protobuf bundle，解决更新器无法识别旧格式的问题。**v1.3.2 优化**：0 丢包明确显示为 0.0%，延迟曲线按 ≤100/100–200/>200 ms 用绿/黄/红区分。**v1.3.1 修复**：管理后台可逐台点击触发在线 Agent 签名更新，并查看任务状态。**v1.3.0 新增**：三网弹窗提供延迟/丢包双曲线、可交互采样点数值、24 小时平均丢包率与延迟统计。**v1.2.9** 增加 Agent 签名更新；**v1.2.7** 三网详情改为点击打开的居中弹窗，避免卡片刷新时跳动。

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
| 单向 POST 推送指标 ------------------------------------+
| 无入站监听；root 更新助手只轮询固定状态并验签安装 Agent |
+-----------------------+                                ▲
                                                         |
                         仅接受本仓库 GitHub Actions 签名的 Agent Release
```

* **安全边界 1：Agent 无入站监听，指标仍为单向上报**
  - 受控端（Agent）**零端口监听**，不开放任何公网/内网入站端口，彻底隐匿于网络扫描之外；
  - 独立更新助手只向主控查询固定更新状态；协议没有命令、脚本或通用任务字段。
  - 更新助手仅安装通过 GitHub Actions Sigstore 身份及签名清单哈希验证的 Agent，并且只重启固定 Agent 服务。
* **铁律 2：真实 IP 物理级脱敏（盲盒式防护）**
  - 前端大屏及公开 API 仅输出节点别名、国旗图标、系统图标、硬件规格与实时负载；
  - 服务端向外返回的接口中**物理级剔除 IP、MAC 地址、私网拓扑等敏感元数据**，爬虫与黑客无法测绘节点。
* **安全边界 3：指标 Agent 使用最小权限**
  - 读取 Linux 系统负载（`/proc/stat`、`/proc/meminfo`、`/proc/net/dev`、`/proc/loadavg`）完全无需 Root 特权；
  - 指标 Agent 以受限用户 `nobody:nogroup` 运行。独立 root 更新助手只写 Agent 安装目录，并重启固定的 `aegis-agent` 服务。
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

## 🚀 极速一键部署 (支持 Master 服务端 & Agent 受控端)

无论是探针主控服务端，还是被控节点，均可通过官方最新 Release 脚本实现**秒级自动化无感安装**：

### 1. 一键安装主控服务端 (Master)
在您的主控服务器终端直接执行：
```bash
curl -fsSL https://github.com/wxfyes/AegisProbe/releases/latest/download/install.sh | bash -s -- --master
```
*(国内 VPS 极速镜像: `curl -fsSL https://mirror.ghproxy.com/https://github.com/wxfyes/AegisProbe/releases/latest/download/install.sh | bash -s -- --master`)*

- 脚本会自动检测架构、下载最新静态服务端二进制、配置开机自启 systemd 服务并启动；
- **监控大屏**：`http://你的服务器IP:8989`
- **管理控制台**：`http://你的服务器IP:8989/admin.html`（终端会自动显示随机初始管理员密码）。

---

### 2. 一键安装受控端 (Agent)
登录管理后台（`/admin.html`），在节点列表中点击 **【一键命令】** 复制生成的专用指令，或直接在被控机器执行：
```bash
curl -fsSL https://github.com/wxfyes/AegisProbe/releases/latest/download/install.sh | bash -s -- --agent --server=http://你的主控IP:8989 --id=node_01 --token=YOUR_TOKEN
```
> **安全沙箱保证**：安装脚本自动创建受限用户 `nobody:nogroup`，配置 systemd 严格限制特权（`NoNewPrivileges=true`, `ProtectSystem=strict`），无任何入站端口，零下行渗透通道。

---

### 3. 多功能交互式运维管理菜单
如需查看状态、获取密码、重启服务或卸载，可直接运行交互式菜单：
```bash
bash <(curl -fsSL https://github.com/wxfyes/AegisProbe/releases/latest/download/install.sh)
```

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
<summary><b>Q2: 主控被攻破后，攻击者能否控制 Agent？</b></summary>
主控没有远程 Shell、脚本下发或任意命令接口。主控最多为指定节点登记一个更新请求；节点上的 root 更新助手只接受固定官方 GitHub Release，并验证 Sigstore 签名身份、清单和 Agent 哈希后再安装。仓库、GitHub Actions 发布工作流或 Sigstore 信任链遭到攻破，仍可能带来恶意更新风险。
</details>

<details>
<summary><b>Q3: 如何重置管理员密码？</b></summary>
直接打开服务端目录下的 <code>data/config.json</code>，找到 <code>admin_password</code> 字段修改为你所需的密码并保存，重启服务或在后台更新即可生效。
</details>

---

## 📄 开源许可证

本项目基于 [MIT 许可证](LICENSE) 开源。
