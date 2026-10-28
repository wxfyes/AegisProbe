## v1.3.6 累计修复与设置保存
- 保留离线节点最近一次上报的网速与时间，并修复离线超时设置无法持久化的问题。
- 修复大屏标题与系统设置联动；三网统计弹窗首次打开时正确显示历史统计。
- TCP 端口测活现在根据 TCPing 响应判断端口状态，不再把普通 HTTP 成功误判为端口畅通；未识别的响应按未确认处理。
- 端口测活配置可独立保存，校验 `{ip}`、`{port}` 并提示自建探针使用 `/tcping`。
- 管理后台异步加载配置时保留正在编辑的输入，避免旧配置覆盖新地址；保存成功后再刷新。
- 离线节点保留并显示最近一次上报的上下行网速，悬停提示标明这是最后上报值，不是实时速度。
- 在超时阈值输入框旁增加可见的全局保存按钮和说明；保存后仍通过配置接口持久化并重新加载确认。

---

## v1.3.5 离线节点保留最后状态
- 主控每 10 秒将最新 Agent 状态快照原子保存到配置目录的 `node_state.json`，文件权限为 `0600`；重启后恢复快照供离线卡片查看。
- 离线卡片保留最后一次上报的 CPU、内存、磁盘、负载、连接数和累计流量；实时网速显示 `--`，并以“最后在线”显示最近上报时间。
- 在线节点仍显示实时指标和在线时长；离线节点不计入全局在线流量与实时网速汇总。

---

## v1.3.4 三网弹窗首次加载修复与离线卡片强调
- 修复三网弹窗首次打开时历史曲线加载后统计数据仍显示为 0 的问题；历史数据返回后同步刷新统计和曲线。
- 离线节点卡片增加红色斜纹遮罩并强化离线标记，让离线状态在节点列表中更醒目。

---

## v1.3.3 修复 Agent 更新签名包格式
- 修复更新器无法解析旧版 Cosign JSON bundle（`base64Signature`）的问题，改由 GitHub Actions 生成 Sigstore 标准 protobuf bundle。
- 发布流程升级到修复版 Cosign，并在上传前检查 bundle 结构；Agent 仍会执行严格的签名身份与 SHA-256 校验。

---

## v1.3.2 三网质量颜色与零丢包显示
- 当前检测结果的丢包率为 0 时明确显示 `0.0%`；该字段在接口中省略时，正常状态按 0% 展示。
- 当前延迟和历史曲线按延迟分级着色：≤100 ms 绿色、100–200 ms 黄色、>200 ms 红色；图例说明阈值，采样数值和丢包率曲线保持可查看。

---

## v1.3.1 管理后台 Agent 手动升级入口修复
- 修复主控嵌入版管理页面缺少 Agent 更新按钮的问题；节点在线且支持签名更新时，可在操作列逐台发起更新请求。
- 显示等待领取、下载、验签、安装和失败状态；处理中的请求自动刷新。
- 更新仍由节点本地助手轮询 GitHub、验证 Sigstore 签名和 SHA-256 后执行，不增加远程命令通道。

---

## v1.3.0 三网质量趋势图增强
- 三网弹窗改为更宽的监测面板，延迟与丢包率分别绘图，并显示清晰的时间刻度和数值轴。
- 悬停、点击或键盘聚焦采样点可查看时间、延迟和丢包率精确值。
- 增加近 24 小时平均延迟、延迟范围、平均丢包率、最高丢包率和有效检测次数；平均丢包率按有效检测轮次计算。
- 仅更新主控前端和静态资源，不改变 Agent 探测逻辑，也不增加 GPU 动画。

---

## v1.2.9 Agent 签名更新
- GitHub Actions 在发布时构建各架构 Agent 与本地更新助手，并使用 OIDC/Sigstore 无长期私钥签名更新清单。
- 管理后台可逐节点登记更新请求；Agent 更新助手只安装通过签名身份与 SHA-256 校验的官方 Agent，并重启固定服务。
- 首次升级仍需逐节点运行一次后台一键安装命令，以安装本地更新助手；后续可在后台逐台触发。
- 更新助手当前支持 Linux systemd 与 OpenRC；Windows 与 OpenWrt/procd 暂不支持自动更新。

---

## 🛡️ 天阙·神盾安全探针 (AegisProbe) v1.2.7

### 🐛 v1.2.7 三网详情交互修复
- **改为点击打开完整弹窗**：移除鼠标悬浮展开和悬浮预加载；点击卡片「国内线路」后，在固定居中窗口查看电信、移动、联通的实时数据和 24 小时曲线。
- **避免探针上报时跳动**：详情浮层脱离卡片布局，不再被卡片边界、视口上下位置或轮询更新推着移动；支持点遮罩、关闭按钮或 Esc 关闭。

---

## 🛡️ 天阙·神盾安全探针 (AegisProbe) v1.2.6

### ✨ v1.2.6 更新特性与修复
- **动态公网 IP 多源刷新**：Agent 每 5 分钟异步查询 IPv4/IPv6 公网出口并覆盖缓存；主控仍优先以 Agent 心跳连接来源更新对应地址栈。
- **Globalping 额度保护**：每轮检测前读取剩余额度，不足时跳过整轮并显示原因；ICMP 包数限制为 1–16，HTTP 400 显示接口参数详情。
- **线路质量告警**：管理员可配置 Telegram 延迟/丢包阈值；连续两轮异常后告警，恢复后通知。
- **三网弹窗与首页细节**：修正弹窗字体变量，提升字号和对比度，标记过期结果；维持纯 CSS/SVG 渲染，不增加 GPU 动画。

---

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
