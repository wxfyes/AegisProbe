// ==============================================================================
// 天阙·神盾安全探针 (TianQue-Probe) 控制台逻辑 (Komari 风格双页面架构)
// ==============================================================================

(function () {
  'use strict';

  let currentConfig = null;
  let allNodes = [];

  const dom = {
    loginModal: document.getElementById('loginModal'),
    adminLoginForm: document.getElementById('adminLoginForm'),
    adminPassword: document.getElementById('adminPassword'),
    adminLoginError: document.getElementById('adminLoginError'),
    btnLogout: document.getElementById('btnLogout'),
    nodeTableBody: document.getElementById('nodeTableBody'),
    nodeSearchInput: document.getElementById('nodeSearchInput'),
    btnAddNode: document.getElementById('btnAddNode'),
    nodeModal: document.getElementById('nodeModal'),
    nodeForm: document.getElementById('nodeForm'),
    nodeModalTitle: document.getElementById('nodeModalTitle'),
    btnCancelNode: document.getElementById('btnCancelNode'),
    cmdModal: document.getElementById('cmdModal'),
    cmdText: document.getElementById('cmdText'),
    btnCloseCmd: document.getElementById('btnCloseCmd'),
    btnCopyCmd: document.getElementById('btnCopyCmd'),
    settingsForm: document.getElementById('settingsForm'),
    btnTestTG: document.getElementById('btnTestTG'),
    btnExportBackup: document.getElementById('btnExportBackup'),
    btnImportBackup: document.getElementById('btnImportBackup'),
    backupFileInput: document.getElementById('backupFileInput'),

    // 表单元素
    cfgSiteTitle: document.getElementById('cfgSiteTitle'),
    cfgAccessMode: document.getElementById('cfgAccessMode'),
    cfgAdminPassword: document.getElementById('cfgAdminPassword'),
    cfgOfflineSec: document.getElementById('cfgOfflineSec'),
    cfgTGEnabled: document.getElementById('cfgTGEnabled'),
    cfgTGBotToken: document.getElementById('cfgTGBotToken'),
    cfgTGChatID: document.getElementById('cfgTGChatID'),
    cfgTGProxy: document.getElementById('cfgTGProxy'),
    cfgGFWCheckURL: document.getElementById('cfgGFWCheckURL'),
    gfwStatusBadge: document.getElementById('gfwStatusBadge'),
    cfgGFWCheckIntervalSelect: document.getElementById('cfgGFWCheckIntervalSelect'),
    cfgGFWCheckInterval: document.getElementById('cfgGFWCheckInterval'),
    cfgGFWCustomIntervalGroup: document.getElementById('cfgGFWCustomIntervalGroup'),
    btnTriggerGFW: document.getElementById('btnTriggerGFW'),
    btnFillDefaultGFW: document.getElementById('btnFillDefaultGFW'),
    cfgRouteEnabled: document.getElementById('cfgRouteEnabled'),
    cfgRouteProvider: document.getElementById('cfgRouteProvider'),
    cfgRouteAPIBaseURL: document.getElementById('cfgRouteAPIBaseURL'),
    cfgRouteAPIToken: document.getElementById('cfgRouteAPIToken'),
    cfgRouteClearToken: document.getElementById('cfgRouteClearToken'),
    cfgRouteTelecom: document.getElementById('cfgRouteTelecom'),
    cfgRouteUnicom: document.getElementById('cfgRouteUnicom'),
    cfgRouteMobile: document.getElementById('cfgRouteMobile'),
    cfgRouteCustomURL: document.getElementById('cfgRouteCustomURL'),
    cfgRouteIntervalSelect: document.getElementById('cfgRouteIntervalSelect'),
    cfgRouteInterval: document.getElementById('cfgRouteInterval'),
    cfgRouteProbeCount: document.getElementById('cfgRouteProbeCount'),
    cfgRouteTimeout: document.getElementById('cfgRouteTimeout'),
    cfgRoutePackets: document.getElementById('cfgRoutePackets'),
    cfgRouteAlertEnabled: document.getElementById('cfgRouteAlertEnabled'),
    cfgRouteAlertLatency: document.getElementById('cfgRouteAlertLatency'),
    cfgRouteAlertLoss: document.getElementById('cfgRouteAlertLoss'),
    routeQualityUsage: document.getElementById('routeUsageEstimate'),
    routeQualityRunStatus: document.getElementById('routeQualityRunStatus'),
    btnTriggerRouteQuality: document.getElementById('btnTriggerRouteQuality'),

    nodeId: document.getElementById('nodeId'),
    nodeName: document.getElementById('nodeName'),
    nodeRegion: document.getElementById('nodeRegion'),
    nodeGroup: document.getElementById('nodeGroup'),
    nodePrice: document.getElementById('nodePrice'),
    nodeExpireDate: document.getElementById('nodeExpireDate'),
    nodeAutoRenew: document.getElementById('nodeAutoRenew'),
    nodeRenewalPeriod: document.getElementById('nodeRenewalPeriod'),
    nodeToken: document.getElementById('nodeToken'),
    nodeProbePort: document.getElementById('nodeProbePort'),
  };

  // 1. 同步主题
  const savedTheme = localStorage.getItem('aegis_theme') || 'dark';
  document.documentElement.setAttribute('data-theme', savedTheme);

  // 2. 侧边栏 Tab 切换 (服务器列表 vs 系统设置，秒切无卡顿)
  document.querySelectorAll('.sidebar-nav .nav-item').forEach(btn => {
    btn.addEventListener('click', () => {
      document.querySelectorAll('.sidebar-nav .nav-item').forEach(b => b.classList.remove('active'));
      document.querySelectorAll('.tab-pane').forEach(p => p.classList.remove('active'));

      btn.classList.add('active');
      const tabId = btn.dataset.tab;
      const targetPane = document.getElementById(tabId);
      if (targetPane) {
        targetPane.classList.add('active');
      }
    });
  });

  // 3. 登录流程
  dom.adminLoginForm.addEventListener('submit', async (e) => {
    e.preventDefault();
    const pwd = dom.adminPassword.value.trim();
    if (!pwd) return;

    dom.adminLoginError.textContent = '';
    try {
      const res = await fetch('/api/auth/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ password: pwd })
      });
      const data = await res.json();
      if (res.ok && data.ok) {
        dom.loginModal.style.display = 'none';
        loadAdminConfig();
      } else {
        dom.adminLoginError.textContent = data.error || '密码错误';
      }
    } catch (err) {
      dom.adminLoginError.textContent = '网络请求失败';
    }
  });

  // 4. 注销
  dom.btnLogout.addEventListener('click', async () => {
    await fetch('/api/auth/logout', { method: 'POST' });
    window.location.reload();
  });

  // 5. 使用与大屏一致的完整离线国家/地区旗帜库
  function getFlagSVG(code, name, region) {
    return window.TianQueFlags.getFlagHTML(code, name, region);
  }

  function inferRenewalPeriod(price) {
    const value = String(price || '').toLowerCase();
    if (value.includes('半年') || value.includes('6个月') || value.includes('semiannual') || value.includes('semi-annual')) return 'half_year';
    if (value.includes('季度') || value.includes('季') || value.includes('quarter') || value.includes('qtr')) return 'quarter';
    if (value.includes('年') || value.includes('year') || value.includes('annual')) return 'year';
    return 'month';
  }

  function renewalPeriodLabel(period) {
    return ({ month: '每月', quarter: '每季度', half_year: '每半年', year: '每年' })[period] || '每月';
  }

  function compareSemver(left, right) {
    const parse = value => String(value || '').replace(/^v/, '').split('.').map(n => Number.parseInt(n, 10) || 0);
    const a = parse(left), b = parse(right);
    for (let i = 0; i < 3; i++) {
      if ((a[i] || 0) !== (b[i] || 0)) return (a[i] || 0) - (b[i] || 0);
    }
    return 0;
  }
  // 6. 加载配置与节点列表
  async function loadAdminConfig() {
    try {
      const res = await fetch('/api/admin/config');
      if (res.status === 401) {
        dom.loginModal.style.display = 'flex';
        dom.adminPassword.focus();
        return;
      }
      if (!res.ok) return;

      const data = await res.json();
      currentConfig = data;
      allNodes = data.nodes || [];
      renderSettings(data);
      renderNodesTable(allNodes);
    } catch (err) {
      console.error('加载后台配置失败:', err);
    }
  }

  function updateGFWBadge(url, intervalSec) {
    if (!dom.gfwStatusBadge) return;
    if (!url) {
      dom.gfwStatusBadge.innerHTML = '⚪ 当前未启用 (留空关闭)';
      dom.gfwStatusBadge.style.background = 'rgba(148, 163, 184, 0.15)';
      dom.gfwStatusBadge.style.color = 'var(--text-sub)';
      dom.gfwStatusBadge.style.borderColor = 'var(--panel-border)';
    } else {
      const hours = Math.round(((intervalSec || 14400) / 3600) * 10) / 10;
      dom.gfwStatusBadge.innerHTML = `🟢 测活巡检运行中 (每 ${hours} 小时)`;
      dom.gfwStatusBadge.style.background = 'rgba(16, 185, 129, 0.15)';
      dom.gfwStatusBadge.style.color = '#10b981';
      dom.gfwStatusBadge.style.borderColor = 'rgba(16, 185, 129, 0.35)';
    }
  }

  function renderSettings(cfg) {
    dom.cfgSiteTitle.value = cfg.site_title || '';
    dom.cfgAccessMode.value = cfg.access_mode || 'public';
    dom.cfgAdminPassword.value = '';
    dom.cfgOfflineSec.value = cfg.offline_sec || 60;
    dom.cfgTGEnabled.checked = !!cfg.tg_enabled;
    dom.cfgTGBotToken.value = cfg.tg_bot_token || '';
    dom.cfgTGChatID.value = cfg.tg_chat_id || '';
    dom.cfgTGProxy.value = cfg.tg_proxy || '';
    
    const gfwURL = cfg.gfw_check_url || '';
    if (dom.cfgGFWCheckURL) {
      dom.cfgGFWCheckURL.value = gfwURL;
    }
    const intervalSec = cfg.gfw_check_interval || 14400;
    if (dom.cfgGFWCheckInterval) {
      dom.cfgGFWCheckInterval.value = intervalSec;
    }
    if (dom.cfgGFWCheckIntervalSelect) {
      const presetValues = ['3600', '7200', '14400', '21600', '28800', '43200', '86400'];
      if (presetValues.includes(String(intervalSec))) {
        dom.cfgGFWCheckIntervalSelect.value = String(intervalSec);
        if (dom.cfgGFWCustomIntervalGroup) dom.cfgGFWCustomIntervalGroup.style.display = 'none';
      } else {
        dom.cfgGFWCheckIntervalSelect.value = 'custom';
        if (dom.cfgGFWCustomIntervalGroup) dom.cfgGFWCustomIntervalGroup.style.display = 'block';
      }
    }
    updateGFWBadge(gfwURL, intervalSec);

    const rq = cfg.route_quality || {};
    if (dom.cfgRouteEnabled) dom.cfgRouteEnabled.checked = !!rq.enabled;
    if (dom.cfgRouteProvider) dom.cfgRouteProvider.value = rq.provider || 'globalping';
    if (dom.cfgRouteAPIBaseURL) dom.cfgRouteAPIBaseURL.value = rq.api_base_url || 'https://api.globalping.io';
    if (dom.cfgRouteAPIToken) dom.cfgRouteAPIToken.value = '';
    const tokenHint = document.getElementById('routeTokenHint');
    if (tokenHint) tokenHint.textContent = rq.api_token ? '已保存 Token（出于安全不会回显；留空保持不变）' : '当前未配置 Token，将使用接口匿名额度。';
    if (dom.cfgRouteClearToken) dom.cfgRouteClearToken.checked = false;
    if (dom.cfgRouteTelecom) dom.cfgRouteTelecom.value = rq.telecom_magic || 'AS4134+China';
    if (dom.cfgRouteUnicom) dom.cfgRouteUnicom.value = rq.unicom_magic || 'AS4837+China';
    if (dom.cfgRouteMobile) dom.cfgRouteMobile.value = rq.mobile_magic || 'AS9808+China';
    if (dom.cfgRouteCustomURL) dom.cfgRouteCustomURL.value = rq.custom_url || '';
    const routeInterval = rq.interval_sec || 1800;
    const routePresets = ['900', '1800', '3600', '21600', '86400'];
    if (dom.cfgRouteIntervalSelect) {
      dom.cfgRouteIntervalSelect.value = routePresets.includes(String(routeInterval)) ? String(routeInterval) : 'custom';
      if (dom.cfgRouteInterval) {
        dom.cfgRouteInterval.value = routeInterval;
        dom.cfgRouteInterval.style.display = dom.cfgRouteIntervalSelect.value === 'custom' ? 'block' : 'none';
      }
    }
    if (dom.cfgRouteProbeCount) dom.cfgRouteProbeCount.value = rq.probe_count || 1;
    if (dom.cfgRouteTimeout) dom.cfgRouteTimeout.value = rq.timeout_sec || 10;
    if (dom.cfgRoutePackets) dom.cfgRoutePackets.value = rq.packets || 5;
    if (dom.cfgRouteAlertEnabled) dom.cfgRouteAlertEnabled.checked = !!rq.alert_enabled;
    if (dom.cfgRouteAlertLatency) dom.cfgRouteAlertLatency.value = rq.alert_latency_ms || 200;
    if (dom.cfgRouteAlertLoss) dom.cfgRouteAlertLoss.value = rq.alert_loss_pct || 5;
    toggleRouteProvider();
    updateRouteUsageEstimate();

    if (cfg.master_version) {
      const vText = cfg.master_version;
      const sbEl = document.getElementById('sidebarMasterVersion');
      if (sbEl) sbEl.textContent = vText;
      const hdEl = document.getElementById('headerMasterVersion');
      if (hdEl) hdEl.textContent = `Master ${vText}`;
    }
  }

  let draggedRow = null;

  function renderNodesTable(nodes) {
    const keyword = (dom.nodeSearchInput.value || '').trim().toLowerCase();
    const filtered = nodes.filter(n => {
      if (!keyword) return true;
      return (n.name || '').toLowerCase().includes(keyword) ||
             (n.id || '').toLowerCase().includes(keyword) ||
             (n.group || '').toLowerCase().includes(keyword) ||
             (n.region || '').toLowerCase().includes(keyword);
    });

    if (filtered.length === 0) {
      dom.nodeTableBody.innerHTML = `<tr><td colspan="10" style="text-align: center; color: var(--text-muted); padding: 30px;">暂无匹配的服务器节点</td></tr>`;
      return;
    }

    dom.nodeTableBody.innerHTML = filtered.map(n => {
      const flagHtml = getFlagSVG(n.flag, n.name, n.region);

      // 双栈 IPv4 / IPv6 渲染引擎 (Komari 同款双行展示，各自独立复制)
      let ipHtml = '';
      const items = [];
      if (n.ipv4) {
        items.push(`
          <div style="display: flex; align-items: center; gap: 5px;">
            <code style="font-family: var(--font-mono); font-weight: 700; color: #0284c7; font-size: 12.5px;">${escapeHtml(n.ipv4)}</code>
            <button class="btn-sm" style="padding: 1px 5px; font-size: 10px;" onclick="window.copyText('${escapeHtml(n.ipv4)}')" title="复制IPv4">📋</button>
          </div>
        `);
      }
      if (n.ipv6) {
        items.push(`
          <div style="display: flex; align-items: center; gap: 5px;">
            <code style="font-family: var(--font-mono); font-weight: 700; color: #38bdf8; font-size: 11.5px;" title="${escapeHtml(n.ipv6)}">${escapeHtml(n.ipv6)}</code>
            <button class="btn-sm" style="padding: 1px 5px; font-size: 10px;" onclick="window.copyText('${escapeHtml(n.ipv6)}')" title="复制IPv6">📋</button>
          </div>
        `);
      }
      if (items.length > 0) {
        ipHtml = `<div style="display: flex; flex-direction: column; gap: 3px;">${items.join('')}</div>`;
      } else if (n.real_ip) {
        ipHtml = `
          <div style="display: flex; align-items: center; gap: 5px;">
            <code style="font-family: var(--font-mono); font-weight: 700; color: #0284c7; font-size: 12.5px;">${escapeHtml(n.real_ip)}</code>
            <button class="btn-sm" style="padding: 1px 5px; font-size: 10px;" onclick="window.copyText('${escapeHtml(n.real_ip)}')" title="点击复制IP">📋</button>
          </div>
        `;
      } else {
        ipHtml = `<span style="color: var(--text-muted); font-size: 12px;">${n.is_online ? '检测中...' : '--'}</span>`;
      }

      // 探针版本渲染引擎 (动态感知主控最新版本，若小鸡落后则醒目提示待更新)
      let verHtml = '';
      const masterVer = (currentConfig && currentConfig.master_version) ? currentConfig.master_version : 'v1.2.0';
      if (!n.is_online) {
        verHtml = `<span style="color: var(--text-muted); font-size: 12px; font-family: var(--font-mono);">--</span>`;
      } else {
        const agentVer = n.version || 'v1.0.0';
        if (agentVer === masterVer) {
          verHtml = `<span class="capsule" style="font-family: var(--font-mono); font-size: 11px; font-weight: 700; background: rgba(16, 185, 129, 0.12); color: #10b981; border: 1px solid rgba(16, 185, 129, 0.3);" title="当前 Agent 为最新版本">${escapeHtml(agentVer)} ✓</span>`;
        } else {
          verHtml = `<span class="capsule" style="font-family: var(--font-mono); font-size: 11px; font-weight: 700; background: rgba(245, 158, 11, 0.12); color: #f59e0b; border: 1px solid rgba(245, 158, 11, 0.3);" title="当前 Agent 版本落后于主控 (${masterVer})，请在小鸡执行一键更新">${escapeHtml(agentVer)} 待更新</span>`;
        }
      }

      const updateState = n.update || {};
      const updateLabels = {
        pending: '等待节点领取', downloading: '正在下载', verifying: '正在验签',
        installing: '正在安装', success: `已更新 ${updateState.requested_version || ''}`,
        error: `更新失败：${updateState.message || '未知错误'}`
      };
      const updateStatusHtml = updateState.status
        ? `<div title="${escapeHtml(updateState.updated_at || '')}" style="margin-top:4px;font-size:11px;color:${updateState.status === 'error' ? '#dc2626' : (updateState.status === 'success' ? '#059669' : '#d97706')};">${escapeHtml(updateLabels[updateState.status] || updateState.status)}</div>`
        : '';
      let updateButton;
      if (!n.is_online) {
        updateButton = `<button class="btn-sm" disabled title="节点离线时不能领取更新">升级 Agent</button>`;
      } else if (/openwrt/i.test(n.real_os || n.os || '')) {
        updateButton = `<button class="btn-sm" disabled title="当前更新助手暂不支持 OpenWrt/procd">手动更新</button>`;
      } else if (compareSemver(n.version, 'v1.2.9') < 0) {
        updateButton = `<button class="btn-sm" onclick="window.showInstallCmd('${escapeHtml(n.id)}')" title="首次升级需运行一次安装命令，以部署本地签名更新助手">更新并安装助手</button>`;
      } else if (compareSemver(n.version, masterVer) < 0) {
        updateButton = `<button class="btn-sm" onclick="window.requestAgentUpdate('${escapeHtml(n.id)}')" title="只请求此节点从 GitHub 获取并校验正式 Agent 更新">升级 Agent</button>`;
      } else {
        updateButton = `<button class="btn-sm" disabled title="Agent 已是最新版本">已是最新</button>`;
      }

      return `
        <tr draggable="true" data-id="${escapeHtml(n.id)}">
          <td style="text-align: center; width: 44px;">
            <span class="drag-handle" title="按住拖拽排序">≡</span>
          </td>
          <td><code style="color: var(--accent-blue); font-weight: 700;">${escapeHtml(n.id)}</code></td>
          <td>
            <div style="display: flex; align-items: center; gap: 8px;">
              ${flagHtml}
              <b style="font-size: 14px; color: var(--text-main);">${escapeHtml(n.name)}</b>
            </div>
          </td>
          <td>${ipHtml}</td>
          <td>${verHtml}${updateStatusHtml}</td>
          <td>
            <span class="node-os-badge">${escapeHtml(n.real_os || n.os || 'Linux')}</span>
          </td>
          <td><span class="capsule capsule-days green">${escapeHtml(n.group || '默认')}</span></td>
          <td><span class="capsule capsule-price">${escapeHtml(n.price || '--')}</span></td>
          <td>
            <div class="node-expiry-cell">
              <span style="font-family: var(--font-mono); font-size: 12px; color: var(--text-sub);">${escapeHtml(n.expire_date || '--')}</span>
              ${n.auto_renew ? `<span class="auto-renew-badge" title="到期且在线时按${renewalPeriodLabel(n.renewal_period)}顺延">自动续费 · ${renewalPeriodLabel(n.renewal_period)}</span>` : ''}
            </div>
          </td>
          <td style="text-align: right;">
            <div style="display: inline-flex; gap: 6px;">
              ${updateButton}
              <button class="btn-sm" onclick="window.showInstallCmd('${escapeHtml(n.id)}')">一键命令</button>
              <button class="btn-sm" onclick="window.editNode('${escapeHtml(n.id)}')">编辑</button>
              <button class="btn-sm danger" onclick="window.deleteNode('${escapeHtml(n.id)}')">删除</button>
            </div>
          </td>
        </tr>
      `;
    }).join('');
  }

  // 绑定原生拖拽排序事件 (零依赖，丝滑流畅，支持即拖即存)
  function initDragAndDrop() {
    const tbody = dom.nodeTableBody;

    tbody.addEventListener('dragstart', (e) => {
      const tr = e.target.closest('tr');
      if (!tr) return;
      draggedRow = tr;
      tr.classList.add('dragging');
      if (e.dataTransfer) {
        e.dataTransfer.effectAllowed = 'move';
        e.dataTransfer.setData('text/plain', tr.dataset.id || '');
      }
    });

    tbody.addEventListener('dragover', (e) => {
      e.preventDefault();
      if (e.dataTransfer) {
        e.dataTransfer.dropEffect = 'move';
      }
      if (!draggedRow) return;

      const targetTr = e.target.closest('tr');
      if (!targetTr || targetTr === draggedRow) return;

      const rect = targetTr.getBoundingClientRect();
      const isAfter = (e.clientY - rect.top) > (rect.height / 2);
      if (isAfter) {
        tbody.insertBefore(draggedRow, targetTr.nextSibling);
      } else {
        tbody.insertBefore(draggedRow, targetTr);
      }
    });

    tbody.addEventListener('dragend', async () => {
      if (!draggedRow) return;
      draggedRow.classList.remove('dragging');
      draggedRow = null;

      // 提取排序后的 ID 序列
      const rows = Array.from(tbody.querySelectorAll('tr[data-id]'));
      const newOrder = rows.map(r => r.dataset.id).filter(Boolean);
      if (newOrder.length === 0) return;

      // 同步本地 allNodes 顺序
      const nodeMap = new Map(allNodes.map(n => [n.id, n]));
      allNodes = newOrder.map(id => nodeMap.get(id)).filter(Boolean);

      try {
        const res = await fetch('/api/admin/node/reorder', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ node_ids: newOrder })
        });
        const data = await res.json();
        if (res.ok && data.ok) {
          showToast('✅ 节点排列顺序已实时保存！', 'success');
        } else {
          showToast('❌ 保存排序失败: ' + (data.error || '未知错误'), 'error');
        }
      } catch (err) {
        showToast('❌ 保存排序异常，请检查网络', 'error');
      }
    });
  }

  // 搜索框输入过滤
  dom.nodeSearchInput.addEventListener('input', () => {
    renderNodesTable(allNodes);
  });

  // 7. 保存全局系统配置
  dom.settingsForm.addEventListener('submit', async (e) => {
    e.preventDefault();
    let gfwInterval = 14400;
    if (dom.cfgGFWCheckIntervalSelect) {
      if (dom.cfgGFWCheckIntervalSelect.value === 'custom') {
        gfwInterval = parseInt(dom.cfgGFWCheckInterval.value, 10) || 14400;
      } else {
        gfwInterval = parseInt(dom.cfgGFWCheckIntervalSelect.value, 10) || 14400;
      }
    }

    const routeInterval = dom.cfgRouteIntervalSelect && dom.cfgRouteIntervalSelect.value !== 'custom'
      ? parseInt(dom.cfgRouteIntervalSelect.value, 10)
      : parseInt(dom.cfgRouteInterval.value, 10);
    const routeQuality = {
      enabled: !!dom.cfgRouteEnabled.checked,
      provider: dom.cfgRouteProvider.value,
      api_base_url: dom.cfgRouteAPIBaseURL.value.trim(),
      api_token: dom.cfgRouteAPIToken.value.trim(),
      interval_sec: routeInterval,
      probe_count: parseInt(dom.cfgRouteProbeCount.value, 10),
      timeout_sec: parseInt(dom.cfgRouteTimeout.value, 10),
      packets: parseInt(dom.cfgRoutePackets.value, 10),
      alert_enabled: !!dom.cfgRouteAlertEnabled.checked,
      alert_latency_ms: parseFloat(dom.cfgRouteAlertLatency.value),
      alert_loss_pct: parseFloat(dom.cfgRouteAlertLoss.value),
      telecom_magic: dom.cfgRouteTelecom.value.trim(),
      unicom_magic: dom.cfgRouteUnicom.value.trim(),
      mobile_magic: dom.cfgRouteMobile.value.trim(),
      custom_url: dom.cfgRouteCustomURL.value.trim()
    };
    const payload = {
      site_title: dom.cfgSiteTitle.value.trim(),
      access_mode: dom.cfgAccessMode.value,
      admin_password: dom.cfgAdminPassword.value.trim(),
      offline_sec: parseInt(dom.cfgOfflineSec.value, 10) || 60,
      tg_enabled: dom.cfgTGEnabled.checked,
      tg_bot_token: dom.cfgTGBotToken.value.trim(),
      tg_chat_id: dom.cfgTGChatID.value.trim(),
      tg_proxy: dom.cfgTGProxy.value.trim(),
      gfw_check_url: dom.cfgGFWCheckURL ? dom.cfgGFWCheckURL.value.trim() : '',
      gfw_check_interval: gfwInterval,
      route_quality: routeQuality,
      route_quality_clear_token: dom.cfgRouteClearToken.checked
    };

    try {
      const res = await fetch('/api/admin/config', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      });
      const data = await res.json();
      if (res.ok && data.ok) {
        showToast('✅ 系统配置已成功保存！', 'success');
        loadAdminConfig();
      } else {
        alert('❌ 保存失败: ' + (data.error || '未知错误'));
      }
    } catch (err) {
      alert('❌ 网络通信异常');
    }
  });

  function toggleRouteProvider() {
    const isCustom = dom.cfgRouteProvider && dom.cfgRouteProvider.value === 'custom';
    const custom = document.getElementById('routeCustomSettings');
    const global = document.getElementById('routeGlobalSettings');
    const base = document.getElementById('routeApiBaseGroup');
    const token = document.getElementById('routeTokenGroup');
    if (custom) custom.style.display = isCustom ? 'block' : 'none';
    if (global) global.style.display = isCustom ? 'none' : 'block';
    if (base) base.style.display = isCustom ? 'none' : 'block';
    if (token) token.style.display = 'block';
    const tokenLabel = token && token.querySelector('.form-label');
    if (tokenLabel) tokenLabel.textContent = isCustom ? '自定义接口 Bearer Token（选填）' : 'API Token（选填，Globalping 登录令牌可提高额度）';
  }

  function updateRouteUsageEstimate() {
    if (!dom.routeQualityUsage) return;
    const count = Math.max(0, (allNodes || []).length);
    const probes = Math.max(1, parseInt(dom.cfgRouteProbeCount.value, 10) || 1);
    const seconds = dom.cfgRouteIntervalSelect.value === 'custom'
      ? (parseInt(dom.cfgRouteInterval.value, 10) || 1800)
      : (parseInt(dom.cfgRouteIntervalSelect.value, 10) || 1800);
    const hourly = Math.ceil(count * 3 * probes * 3600 / Math.max(seconds, 1));
    const authenticated = !!(currentConfig && currentConfig.route_quality && currentConfig.route_quality.api_token);
    const tokenNew = (dom.cfgRouteAPIToken.value || '').trim().length > 0;
    const cap = (authenticated || tokenNew) ? 500 : 250;
    const color = hourly > cap ? '#dc2626' : hourly >= cap * 0.8 ? '#d97706' : '#047857';
    const rounded = seconds % 60 === 0 ? `${seconds / 60} 分钟` : `${seconds} 秒`;
    dom.routeQualityUsage.innerHTML = `📊 当前 ${count} 台节点 × 3 家运营商 × ${probes} 个探针 ÷ 每 ${rounded} 一轮 ≈ <b style="color:${color}">${hourly} 次/小时</b>。额度参考：${authenticated || tokenNew ? '已认证 500' : '匿名 250'} 次/小时。${hourly > cap ? '<br><b style="color:#dc2626">超过参考额度，保存时会被拒绝；请减少探针数或延长间隔。</b>' : hourly >= cap * .8 ? '<br><b style="color:#b45309">已接近参考额度；匿名额度还会与同一出口 IP 的其他用户共享。</b>' : '<br>常规用量在参考额度内；主控每轮会预查剩余额度，不足则跳过整轮。修改检测参数保存或手动检测会额外触发一轮。'}`;
  }

  [dom.cfgRouteProbeCount, dom.cfgRouteInterval, dom.cfgRouteAPIToken].forEach(el => el && el.addEventListener('input', updateRouteUsageEstimate));
  if (dom.cfgRouteIntervalSelect) dom.cfgRouteIntervalSelect.addEventListener('change', () => {
    const custom = dom.cfgRouteIntervalSelect.value === 'custom';
    dom.cfgRouteInterval.style.display = custom ? 'block' : 'none';
    if (!custom) dom.cfgRouteInterval.value = dom.cfgRouteIntervalSelect.value;
    updateRouteUsageEstimate();
  });
  if (dom.cfgRouteProvider) dom.cfgRouteProvider.addEventListener('change', toggleRouteProvider);
  if (dom.cfgRouteClearToken) dom.cfgRouteClearToken.addEventListener('change', () => { if (dom.cfgRouteClearToken.checked) dom.cfgRouteAPIToken.value = ''; updateRouteUsageEstimate(); });

  let routeQualityStatusTimer = null;
  if (dom.btnTriggerRouteQuality) dom.btnTriggerRouteQuality.addEventListener('click', async () => {
    const old = dom.btnTriggerRouteQuality.textContent;
    dom.btnTriggerRouteQuality.disabled = true;
    dom.btnTriggerRouteQuality.textContent = '⏳ 正在排队检测…';
    try {
      const res = await fetch('/api/admin/route-quality/trigger', { method: 'POST' });
      const data = await res.json();
      if (!res.ok) throw new Error(data.error || '检测请求失败');
      dom.routeQualityRunStatus.textContent = '已开始检测；结果会在本轮完成后显示，检测期间仍展示上一轮缓存。';
      if (routeQualityStatusTimer) clearInterval(routeQualityStatusTimer);
      routeQualityStatusTimer = setInterval(async () => {
        try {
          const statusRes = await fetch('/api/admin/route-quality/status');
          const status = await statusRes.json();
          if (statusRes.ok && !status.running) {
            clearInterval(routeQualityStatusTimer);
            routeQualityStatusTimer = null;
            dom.routeQualityRunStatus.textContent = '本轮检测已完成；大屏卡片将在下一次刷新时显示最新结果。';
          }
        } catch (_) { /* keep polling while the run status is temporarily unavailable */ }
      }, 2500);
      showToast('三网 ICMP 检测已开始', 'success');
    } catch (err) {
      dom.routeQualityRunStatus.textContent = err.message;
      showToast(err.message, 'error');
    } finally {
      dom.btnTriggerRouteQuality.disabled = false;
      dom.btnTriggerRouteQuality.textContent = old;
    }
  });

  // 测活周期选择器与实时徽章联动
  if (dom.cfgGFWCheckIntervalSelect) {
    dom.cfgGFWCheckIntervalSelect.addEventListener('change', () => {
      if (dom.cfgGFWCheckIntervalSelect.value === 'custom') {
        if (dom.cfgGFWCustomIntervalGroup) dom.cfgGFWCustomIntervalGroup.style.display = 'block';
      } else {
        if (dom.cfgGFWCustomIntervalGroup) dom.cfgGFWCustomIntervalGroup.style.display = 'none';
        if (dom.cfgGFWCheckInterval) dom.cfgGFWCheckInterval.value = dom.cfgGFWCheckIntervalSelect.value;
      }
      updateGFWBadge(dom.cfgGFWCheckURL ? dom.cfgGFWCheckURL.value.trim() : '', parseInt(dom.cfgGFWCheckInterval.value, 10));
    });
  }

  if (dom.cfgGFWCheckURL) {
    dom.cfgGFWCheckURL.addEventListener('input', () => {
      const interval = (dom.cfgGFWCheckIntervalSelect && dom.cfgGFWCheckIntervalSelect.value !== 'custom')
        ? parseInt(dom.cfgGFWCheckIntervalSelect.value, 10)
        : (parseInt(dom.cfgGFWCheckInterval.value, 10) || 14400);
      updateGFWBadge(dom.cfgGFWCheckURL.value.trim(), interval);
    });
  }

  // 填入推荐测活模板
  if (dom.btnFillDefaultGFW && dom.cfgGFWCheckURL) {
    dom.btnFillDefaultGFW.addEventListener('click', () => {
      dom.cfgGFWCheckURL.value = 'https://v2.xxapi.cn/api/tcping?address={ip}&port={port}';
      const interval = (dom.cfgGFWCheckIntervalSelect && dom.cfgGFWCheckIntervalSelect.value !== 'custom')
        ? parseInt(dom.cfgGFWCheckIntervalSelect.value, 10)
        : (parseInt(dom.cfgGFWCheckInterval.value, 10) || 14400);
      updateGFWBadge(dom.cfgGFWCheckURL.value.trim(), interval);
      showToast('✨ 已填入官方标准测活模板，请点击【保存系统配置】！', 'success');
    });
  }

  // 立即触发一次全网测活
  if (dom.btnTriggerGFW) {
    dom.btnTriggerGFW.addEventListener('click', async () => {
      const gfwURL = dom.cfgGFWCheckURL ? dom.cfgGFWCheckURL.value.trim() : '';
      if (!gfwURL) {
        alert('⚠️ 请先在上方填入外部测活 API URL 模板并保存系统配置，然后再执行测活！');
        return;
      }

      const origText = dom.btnTriggerGFW.textContent;
      dom.btnTriggerGFW.textContent = '⏳ 正在测活巡检...';
      dom.btnTriggerGFW.disabled = true;

      try {
        const res = await fetch('/api/admin/gfw/trigger', { method: 'POST' });
        const data = await res.json();
        if (res.ok && data.ok) {
          showToast('⚡ ' + (data.msg || '测活指令已触发，结果正秒级同步至大屏！'), 'success');
        } else {
          alert('❌ 触发测活失败: ' + (data.error || '未知错误'));
        }
      } catch (err) {
        alert('❌ 请求测活失败，请检查网络');
      } finally {
        dom.btnTriggerGFW.textContent = origText;
        dom.btnTriggerGFW.disabled = false;
      }
    });
  }

  // 8. 测试 Telegram 发送
  dom.btnTestTG.addEventListener('click', async () => {
    const originalText = dom.btnTestTG.textContent;
    dom.btnTestTG.textContent = '⏳ 发送中...';
    try {
      const res = await fetch('/api/admin/test_tg', { method: 'POST' });
      const data = await res.json();
      if (res.ok && data.ok) {
        alert('✅ Telegram 测试消息已成功发出，请检查您的群组或频道！');
      } else {
        alert('❌ TG 测试发送失败: ' + (data.error || ''));
      }
    } catch (err) {
      alert('❌ 网络请求失败');
    } finally {
      dom.btnTestTG.textContent = originalText;
    }
  });

  // 8.1 导出完整配置备份 (.json)
  if (dom.btnExportBackup) {
    dom.btnExportBackup.addEventListener('click', () => {
      window.location.href = '/api/admin/backup/export';
    });
  }

  // 8.2 导入备份一键灾备恢复
  if (dom.btnImportBackup && dom.backupFileInput) {
    dom.btnImportBackup.addEventListener('click', () => {
      dom.backupFileInput.value = '';
      dom.backupFileInput.click();
    });

    dom.backupFileInput.addEventListener('change', async (e) => {
      const file = e.target.files && e.target.files[0];
      if (!file) return;

      try {
        const text = await file.text();
        let parsed;
        try {
          parsed = JSON.parse(text);
        } catch (err) {
          alert('❌ 备份文件解析失败，不是合法的 JSON 格式！');
          return;
        }

        const nodeCount = (parsed.nodes && Array.isArray(parsed.nodes)) ? parsed.nodes.length : 0;
        const title = parsed.site_title || '未命名配置';
        const ok = confirm(`⚠️ 灾难恢复确认：\n\n该备份包含「${nodeCount}」台受控服务器节点。\n站点标题: 「${title}」\n\n导入后将直接替换并恢复当前主控的配置和全部机器！\n受控节点凭证一致，无需重新配置即可自动上线。\n\n确定立即导入恢复吗？`);
        if (!ok) return;

        const origText = dom.btnImportBackup.textContent;
        dom.btnImportBackup.textContent = '⏳ 正在极速恢复中...';
        dom.btnImportBackup.disabled = true;

        const res = await fetch('/api/admin/backup/import', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: text
        });
        const data = await res.json();
        if (res.ok && data.ok) {
          showToast(`✅ 满血恢复成功！已导入 ${data.node_count} 台服务器`, 'success');
          loadAdminConfig();
        } else {
          alert('❌ 恢复失败: ' + (data.error || '未知错误'));
        }
        dom.btnImportBackup.textContent = origText;
        dom.btnImportBackup.disabled = false;
      } catch (err) {
        alert('❌ 读取或导入备份失败: ' + (err.message || err));
      }
    });
  }

  // 9. 添加/编辑节点
  dom.btnAddNode.addEventListener('click', () => {
    dom.nodeModalTitle.textContent = '添加新受控服务器';
    dom.nodeId.value = 'node_' + Math.floor(Math.random() * 899 + 100);
    dom.nodeId.readOnly = false;
    dom.nodeName.value = '';
    dom.nodeRegion.value = '';
    dom.nodeGroup.value = '中转';
    dom.nodePrice.value = '$29.99/月';
    dom.nodeExpireDate.value = '';
    dom.nodeAutoRenew.checked = false;
    dom.nodeRenewalPeriod.value = inferRenewalPeriod(dom.nodePrice.value);
    dom.nodeToken.value = '';
    if (dom.nodeProbePort) dom.nodeProbePort.value = '22';

    dom.nodeModal.style.display = 'flex';
  });

  dom.btnCancelNode.addEventListener('click', () => {
    dom.nodeModal.style.display = 'none';
  });

  window.editNode = function (id) {
    if (!currentConfig || !currentConfig.nodes) return;
    const n = currentConfig.nodes.find(item => item.id === id);
    if (!n) return;

    dom.nodeModalTitle.textContent = '编辑服务器: ' + n.name;
    dom.nodeId.value = n.id;
    dom.nodeId.readOnly = true;
    dom.nodeName.value = n.name || '';
    dom.nodeRegion.value = n.region || '';
    dom.nodeGroup.value = n.group || '中转';
    dom.nodePrice.value = n.price || '$29.99/月';
    dom.nodeExpireDate.value = (n.expire_date || '').replace(/\//g, '-');
    dom.nodeAutoRenew.checked = Boolean(n.auto_renew);
    dom.nodeRenewalPeriod.value = n.renewal_period || inferRenewalPeriod(n.price);
    dom.nodeToken.value = n.token || '';
    if (dom.nodeProbePort) dom.nodeProbePort.value = n.probe_port || 22;

    dom.nodeModal.style.display = 'flex';
  };

  dom.nodeForm.addEventListener('submit', async (e) => {
    e.preventDefault();
    const regionCode = dom.nodeRegion.value.trim().toUpperCase();

    const payload = {
      id: dom.nodeId.value.trim(),
      name: dom.nodeName.value.trim(),
      flag: regionCode,
      region: regionCode,
      os: 'linux', // 优先由 Agent 真实上报采集，无需手动写死
      group: dom.nodeGroup.value.trim(),
      price: dom.nodePrice.value.trim(),
      expire_date: dom.nodeExpireDate.value.replace(/-/g, '/'),
      auto_renew: dom.nodeAutoRenew.checked,
      renewal_period: dom.nodeRenewalPeriod.value,
      token: dom.nodeToken.value.trim(),
      probe_port: parseInt(dom.nodeProbePort ? dom.nodeProbePort.value : '22', 10) || 22
    };

    const submitBtn = dom.nodeForm.querySelector('button[type="submit"]');
    const origText = submitBtn ? submitBtn.textContent : '确定保存';
    if (submitBtn) {
      submitBtn.textContent = '保存中...';
      submitBtn.disabled = true;
    }

    try {
      const res = await fetch('/api/admin/node/save', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      });
      const data = await res.json();
      if (res.ok && data.ok) {
        dom.nodeModal.style.display = 'none';
        showToast('✅ 节点已成功保存！', 'success');
        loadAdminConfig();
      } else {
        alert('❌ 保存节点失败: ' + (data.error || ''));
      }
    } catch (err) {
      alert('❌ 网络通信异常: ' + (err.message || err));
    } finally {
      if (submitBtn) {
        submitBtn.textContent = origText;
        submitBtn.disabled = false;
      }
    }
  });

  window.deleteNode = async function (id) {
    if (!confirm(`确定要彻底删除服务器节点 [${id}] 吗？`)) return;

    try {
      const res = await fetch('/api/admin/node/delete', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ id: id })
      });
      const data = await res.json();
      if (res.ok && data.ok) {
        loadAdminConfig();
      } else {
        alert('❌ 删除失败: ' + (data.error || ''));
      }
    } catch (err) {
      alert('❌ 网络通信异常');
    }
  };

  // 10. 一键安装命令生成与复制
  window.requestAgentUpdate = async function (id) {
    const node = allNodes.find(item => item.id === id);
    if (!node || !node.is_online) {
      showToast('节点离线，无法领取更新', 'error');
      return;
    }
    if (!confirm(`仅向节点 ${id} 登记 Agent ${currentConfig.master_version} 更新请求。节点本地更新助手会从 GitHub 校验签名后安装。继续？`)) return;
    try {
      const res = await fetch('/api/admin/agent/update', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ node_id: id })
      });
      const data = await res.json();
      if (!res.ok) throw new Error(data.error || '更新请求失败');
      showToast(`已向 ${id} 登记更新请求；等待该节点助手轮询`, 'success');
      await loadAdminConfig();
    } catch (err) {
      showToast(`更新请求失败：${err.message || err}`, 'error');
    }
  };

  window.showInstallCmd = async function (id) {
    try {
      const res = await fetch(`/api/admin/agent_install_cmd?id=${encodeURIComponent(id)}`);
      if (!res.ok) {
        alert('获取安装命令失败');
        return;
      }
      const data = await res.json();
      dom.cmdText.textContent = data.cmd;
      dom.cmdModal.style.display = 'flex';
    } catch (e) {
      alert('网络通信异常');
    }
  };

  dom.btnCloseCmd.addEventListener('click', () => {
    dom.cmdModal.style.display = 'none';
  });

  dom.btnCopyCmd.addEventListener('click', () => {
    window.copyText(dom.cmdText.textContent, dom.btnCopyCmd);
  });

  window.copyText = function (text, btnElem) {
    if (!text) return;
    if (navigator.clipboard && navigator.clipboard.writeText) {
      navigator.clipboard.writeText(text).then(() => {
        if (btnElem) {
          const old = btnElem.textContent;
          btnElem.textContent = '✅ 已复制！';
          setTimeout(() => { btnElem.textContent = old; }, 2000);
        } else {
          alert('✅ 已成功复制到剪贴板: ' + text);
        }
      }).catch(() => fallbackCopy(text, btnElem));
    } else {
      fallbackCopy(text, btnElem);
    }
  };

  function fallbackCopy(text, btnElem) {
    const ta = document.createElement('textarea');
    ta.value = text;
    ta.style.position = 'fixed';
    ta.style.top = '-9999px';
    ta.style.left = '-9999px';
    document.body.appendChild(ta);
    ta.focus();
    ta.select();
    try {
      document.execCommand('copy');
      if (btnElem) {
        const old = btnElem.textContent;
        btnElem.textContent = '✅ 已复制！';
        setTimeout(() => { btnElem.textContent = old; }, 2000);
      } else {
        alert('✅ 已成功复制到剪贴板: ' + text);
      }
    } catch (err) {
      alert('复制失败，请手动选中文本复制');
    }
    document.body.removeChild(ta);
  }

  function escapeHtml(str) {
    if (!str) return '';
    return str.replace(/[&<>"']/g, m => ({
      '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
    })[m]);
  }

  function showToast(msg, type = 'info') {
    let toast = document.getElementById('adminToast');
    if (!toast) {
      toast = document.createElement('div');
      toast.id = 'adminToast';
      toast.style.position = 'fixed';
      toast.style.bottom = '24px';
      toast.style.right = '24px';
      toast.style.padding = '10px 18px';
      toast.style.borderRadius = '8px';
      toast.style.color = '#fff';
      toast.style.fontSize = '13px';
      toast.style.fontWeight = '700';
      toast.style.boxShadow = '0 4px 12px rgba(0,0,0,0.3)';
      toast.style.zIndex = '99999';
      toast.style.transition = 'opacity 0.25s ease';
      document.body.appendChild(toast);
    }
    toast.style.background = type === 'error' ? '#dc2626' : (type === 'success' ? '#16a34a' : '#2563eb');
    toast.textContent = msg;
    toast.style.opacity = '1';
    toast.style.display = 'block';
    clearTimeout(toast._timer);
    toast._timer = setTimeout(() => {
      toast.style.opacity = '0';
      setTimeout(() => { toast.style.display = 'none'; }, 250);
    }, 2000);
  }

  // 初始化加载与事件绑定
  initDragAndDrop();
  loadAdminConfig();
  setInterval(() => {
    if (allNodes.some(n => ['pending', 'downloading', 'verifying', 'installing'].includes((n.update || {}).status))) {
      loadAdminConfig();
    }
  }, 10000);
})();
