## 🛡️ 天阙·神盾安全探针 (AegisProbe) v1.2.5

### 📈 v1.2.5 更新特性
- **三网质量 24 小时趋势图**：卡片「国内线路」弹窗展示电信、移动、联通的延迟曲线与丢包采样；历史数据滚动保留最近 24 小时，并写入 `data/route_quality_history.json`，主控重启后继续保留。
- **按需读取历史**：大屏仅在打开三网详情时请求对应节点的历史，避免增加全量卡片摘要的负载。

---

## 🛡️ 天阙·神盾安全探针 (AegisProbe) v1.2.4

### 🐛 v1.2.4 修复
- **修复三网 ICMP 请求被 Globalping HTTP 400 拒绝**：目标为 Agent 公网 IP 时不再发送仅适用于域名目标的 `ipVersion` 参数。

---

## 🛡️ 天阙·神盾安全探针 (AegisProbe) v1.2.3

### ✨ v1.2.3 更新特性
- **国内三网 ICMP 质量监测**：通过 Globalping 中国电信、联通、移动公开探针，周期测量每台在线节点的 Ping 延迟与丢包率。
- **紧凑卡片入口**：大屏增加「国内线路」入口，支持鼠标悬停预览和点击查看电信、移动、联通详情。
- **丰富的管理员调节项**：可选 Globalping 或自定义 JSON API，配置 API Token、各运营商筛选条件、检测间隔、探针数、超时和 ICMP 包数；实时估算每小时用量并提示匿名额度。
- **主控集中调度与缓存**：Agent 继续保持单向上报，不需要开放新端口或升级 Agent。

专为高敏感节点集群打造的下一代**轻量化、零 GPU 负担、零下行渗透通道、物理级 IP 脱敏**的自研服务器探针系统。

---

### ✨ v1.2.2 更新特性
- **动态公网 IP 自动刷新**：服务端根据每次 Agent 心跳的实际 IPv4/IPv6 来源地址更新对应地址，修复 Agent 缓存旧地址导致后台 IP 不变的问题。

### ✨ v1.2.1 更新特性
- **节点连接数监控**：卡片网络行下新增 TCP 已建立连接数与 UDP 套接字数；Linux 从 `/proc` 采样，Windows 每 30 秒采样并缓存。
- **连接数采集需升级 Agent**：已安装节点不会随 Release 自动升级，执行 `curl -fsSL https://github.com/wxfyes/AegisProbe/releases/latest/download/install.sh | bash -s -- --update-agent` 后开始上报。

### ✨ v1.2.0 重大更新特性
1. **主控与被控独立运维与更新闭环**：
   - 拆分独立的主控（Master）与受控端（Agent）的安装、更新与卸载能力；
   - 支持主控无损平滑热更新（自动备份保留 `data/config.json` 节点与密码）；
   - 支持受控端一键热升级与独立卸载清理，杜绝残留进程与服务冲突；
   - 命令行静默参数支持：`--update-master`, `--uninstall-master`, `--update-agent`, `--uninstall-agent`。
2. **版本号全链路透传与展示**：
   - 管理后台【受控服务器列表】在“IP 地址”右侧增加「探针版本」列，清晰展示各小鸡 Agent 版本；
   - 页面顶部与侧边栏高质感展示主控服务端版本号（`Master v1.2.0`），运维排查一目了然。
3. **全面升级国内极速双镜像下载通道**：
   - 彻底剔除失效镜像，换用实测极速的 `ghfast.top` 与 `ghproxy.net` 智能轮询 fallback，国内小鸡 100% 秒级下载。
4. **集成 Aegis-Ping 家宽测活微探针 (< 2MB)**：
   - 专为软路由/家宽设计，纯 ICMP Ping 毫秒测活，深度适配 OpenWrt / iStoreOS 防火墙放行与 procd 守护。

---

### 🚀 极速一键安装指令

#### 1. 交互式多功能管理控制台 (推荐)
```bash
bash <(curl -fsSL https://github.com/wxfyes/AegisProbe/releases/latest/download/install.sh)
```
*(国内极速镜像: `bash <(curl -fsSL https://ghfast.top/https://github.com/wxfyes/AegisProbe/releases/latest/download/install.sh)`)*

#### 2. 一键安装主控服务端 (Master)
```bash
curl -fsSL https://github.com/wxfyes/AegisProbe/releases/latest/download/install.sh | bash -s -- --master
```

#### 3. 一键安装受控端 (Agent)
```bash
curl -fsSL https://github.com/wxfyes/AegisProbe/releases/latest/download/install.sh | bash -s -- --agent --server=http://主控IP:8989 --id=node_01 --token=YOUR_TOKEN
```

#### 4. 一键热升级已安装的组件
- **升级主控服务端**：`curl -fsSL https://github.com/wxfyes/AegisProbe/releases/latest/download/install.sh | bash -s -- --update-master`
- **升级受控端探针**：`curl -fsSL https://github.com/wxfyes/AegisProbe/releases/latest/download/install.sh | bash -s -- --update-agent`
