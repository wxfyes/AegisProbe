## 🛡️ 天阙·神盾安全探针 (AegisProbe) v1.2.0

专为高敏感节点集群打造的下一代**轻量化、零 GPU 负担、零下行渗透通道、物理级 IP 脱敏**的自研服务器探针系统。

---

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
