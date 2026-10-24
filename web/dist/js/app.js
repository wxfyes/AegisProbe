// ==============================================================================
// 天阙·神盾安全探针 (TianQue-Probe) 前端大屏引擎
// 【原则】：零 GPU 负担、原生轻量、硬件加速合成、瞬时响应
// ==============================================================================

(function () {
  'use strict';

  let currentGroup = '所有';
  let cachedData = null;
  let isLocked = false;
  let pollTimer = null;

  const dom = {
    html: document.documentElement,
    siteTitle: document.getElementById('siteTitle'),
    btnThemeToggle: document.getElementById('btnThemeToggle'),
    btnAdmin: document.getElementById('btnAdmin'),
    valCurrentTime: document.getElementById('valCurrentTime'),
    valOnlineRatio: document.getElementById('valOnlineRatio'),
    valRegionCount: document.getElementById('valRegionCount'),
    valTrafficUp: document.getElementById('valTrafficUp'),
    valTrafficDown: document.getElementById('valTrafficDown'),
    valSpeedUp: document.getElementById('valSpeedUp'),
    valSpeedDown: document.getElementById('valSpeedDown'),
    groupFilterBar: document.getElementById('groupFilterBar'),
    nodeGrid: document.getElementById('nodeGrid'),
    lockModal: document.getElementById('lockModal'),
    unlockForm: document.getElementById('unlockForm'),
    unlockPassword: document.getElementById('unlockPassword'),
    unlockError: document.getElementById('unlockError'),
  };

  // 1. 主题初始化与切换
  function initTheme() {
    const saved = localStorage.getItem('aegis_theme') || 'dark';
    setTheme(saved);
    dom.btnThemeToggle.addEventListener('click', () => {
      const current = dom.html.getAttribute('data-theme');
      const target = current === 'dark' ? 'light' : 'dark';
      setTheme(target);
    });
  }

  function setTheme(theme) {
    dom.html.setAttribute('data-theme', theme);
    localStorage.setItem('aegis_theme', theme);
    dom.btnThemeToggle.textContent = theme === 'dark' ? '🌙' : '☀️';
  }

  // 2. 跳转管理后台 (新标签页打开，绝不覆盖大屏)
  dom.btnAdmin.addEventListener('click', () => {
    window.open('/admin.html', '_blank');
  });

  // 3. 密码解锁处理
  dom.unlockForm.addEventListener('submit', async (e) => {
    e.preventDefault();
    const pwd = dom.unlockPassword.value.trim();
    if (!pwd) return;

    dom.unlockError.textContent = '';
    try {
      const res = await fetch('/api/auth/unlock', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ password: pwd })
      });
      const data = await res.json();
      if (res.ok && data.ok) {
        dom.lockModal.style.display = 'none';
        isLocked = false;
        fetchSummary();
      } else {
        dom.unlockError.textContent = data.error || '密码错误';
      }
    } catch (err) {
      dom.unlockError.textContent = '网络连接异常，请重试';
    }
  });

  // 4. 获取服务端脱敏聚合数据
  async function fetchSummary() {
    if (isLocked) return;

    try {
      const res = await fetch('/api/nodes/summary');
      if (res.status === 401) {
        const errData = await res.json();
        if (errData.locked) {
          isLocked = true;
          dom.lockModal.style.display = 'flex';
          dom.unlockPassword.focus();
          return;
        }
      }

      if (!res.ok) return;

      const data = await res.json();
      cachedData = data;
      renderAll(data);
    } catch (e) {
      console.warn('轮询数据失败:', e);
    }
  }

  // 5. 渲染顶部与多维分组
  function renderAll(data) {
    if (data.siteTitle) {
      dom.siteTitle.textContent = data.siteTitle;
      document.title = data.siteTitle;
    }

    // 顶部看板
    const s = data.summary;
    if (s) {
      dom.valCurrentTime.textContent = s.current_time;
      dom.valOnlineRatio.textContent = `${s.online_count} / ${s.total_count}`;

      // 智能去重准确统计地区数量（优先根据节点真实地区/名称智能提取）
      const regionSet = new Set();
      (data.nodes || []).forEach(n => {
        const c = detectCountry(n.flag, n.name, n.region);
        if (c && c !== 'global') {
          regionSet.add(c);
        }
      });
      dom.valRegionCount.textContent = regionSet.size > 0 ? regionSet.size : (s.region_count || 0);

      dom.valTrafficUp.textContent = formatBytes(s.total_net_out_bytes);
      dom.valTrafficDown.textContent = formatBytes(s.total_net_in_bytes);
      dom.valSpeedUp.textContent = formatSpeed(s.instant_speed_out);
      dom.valSpeedDown.textContent = formatSpeed(s.instant_speed_in);
    }

    // 分组按钮
    renderGroups(data.groups || ['所有']);

    // 节点卡片列表
    renderNodes(data.nodes || []);
  }

  function renderGroups(groups) {
    // 检查是否有变动
    const existingButtons = Array.from(dom.groupFilterBar.querySelectorAll('.filter-btn')).map(b => b.dataset.group);
    if (JSON.stringify(existingButtons) === JSON.stringify(groups)) {
      return;
    }

    dom.groupFilterBar.innerHTML = '';
    groups.forEach(g => {
      const btn = document.createElement('button');
      btn.className = `filter-btn ${g === currentGroup ? 'active' : ''}`;
      btn.dataset.group = g;
      btn.textContent = g;
      btn.addEventListener('click', () => {
        currentGroup = g;
        dom.groupFilterBar.querySelectorAll('.filter-btn').forEach(b => b.classList.remove('active'));
        btn.classList.add('active');
        if (cachedData) {
          renderNodes(cachedData.nodes || []);
        }
      });
      dom.groupFilterBar.appendChild(btn);
    });
  }

  // 6. 渲染节点卡片 (原地精确更新 DOM，彻底杜绝全量重排，零 GPU 负担)
  function renderNodes(nodes) {
    const filtered = nodes.filter(n => currentGroup === '所有' || n.group === currentGroup);
    const existingCards = new Map();
    dom.nodeGrid.querySelectorAll('.node-card').forEach(card => {
      existingCards.set(card.dataset.id, card);
    });

    // 追踪本次需要保留的节点 ID
    const currentIds = new Set(filtered.map(n => n.id));

    // 移除不再存在的卡片
    existingCards.forEach((card, id) => {
      if (!currentIds.has(id)) {
        card.remove();
        existingCards.delete(id);
      }
    });

    filtered.forEach((node, index) => {
      let card = existingCards.get(node.id);
      const isOnline = !!node.status.is_online;
      const daysLeft = node.billing.days_left;
      
      let dayClass = 'green';
      let dayText = `余 ${daysLeft} 天`;
      if (daysLeft <= 0) {
        dayClass = 'red';
        dayText = '已过期';
      } else if (daysLeft <= 3) {
        dayClass = 'red';
      } else if (daysLeft <= 15) {
        dayClass = 'yellow';
      }

      const cpuPct = isOnline ? node.status.cpu_pct : 0;
      const ramPct = isOnline ? node.status.ram_pct : 0;
      const diskPct = isOnline ? node.status.disk_pct : 0;
      const swapPct = (!node.status.swap_off && isOnline) ? node.status.swap_pct : 0;
      const healthClass = (value, warnAt, dangerAt) => value >= dangerAt ? 'health-danger' : value >= warnAt ? 'health-warn' : 'health-good';
      const boundedPct = value => Math.max(0, Math.min(100, Number(value) || 0));

      if (!card) {
        // 首次挂载创建 DOM 节点
        card = document.createElement('div');
        card.className = `node-card ${isOnline ? '' : 'offline'}`;
        card.dataset.id = node.id;
        card.innerHTML = `
          <div class="card-header">
            <div class="card-title-group">
              <span class="node-flag">${getFlagSVG(node.flag, node.name, node.region)}</span>
              <span class="node-name" title="${escapeHtml(node.name)}">${escapeHtml(node.name)}</span>
              <span class="node-os-badge">${formatOSBadge(node.os)}</span>
            </div>
            ${renderGFWBadge(node.gfw_status)}
          </div>

          <div class="card-capsules">
            ${node.billing.price ? `<span class="capsule capsule-price">${escapeHtml(node.billing.price)}</span>` : ''}
            <span class="capsule capsule-days ${dayClass}">${dayText}</span>
          </div>

          <div class="card-hardware">
            <span><span class="hw-icon-cpu">🖥️</span> <span class="val-cores">${node.hardware.cpu_cores || 1} Cores</span></span>
            <span><span class="hw-icon-ram">🗄️</span> <span class="val-ram">${node.hardware.ram_str}</span></span>
            <span><span class="hw-icon-disk">💾</span> <span class="val-disk">${node.hardware.disk_str}</span></span>
          </div>

          <div class="metrics-bars">
            <div class="bar-row">
              <span class="bar-label">CPU</span>
              <div class="bar-track"><div class="bar-fill bar-fill-cpu ${healthClass(cpuPct, 60, 80)}" style="width: ${boundedPct(cpuPct)}%;"></div></div>
              <span class="bar-val val-cpu-pct">${cpuPct}%</span>
            </div>
            <div class="bar-row">
              <span class="bar-label">内存</span>
              <div class="bar-track"><div class="bar-fill bar-fill-ram ${healthClass(ramPct, 70, 85)}" style="width: ${boundedPct(ramPct)}%;"></div></div>
              <span class="bar-val val-ram-pct">${ramPct}%</span>
            </div>
            <div class="bar-row">
              <span class="bar-label">SWAP</span>
              <div class="bar-track"><div class="bar-fill bar-fill-swap" style="width: ${boundedPct(swapPct)}%;"></div></div>
              <span class="bar-val val-swap-pct ${node.status.swap_off ? 'off' : ''}">${node.status.swap_off ? 'OFF' : swapPct + '%'}</span>
            </div>
            <div class="bar-row">
              <span class="bar-label">硬盘</span>
              <div class="bar-track"><div class="bar-fill bar-fill-disk ${healthClass(diskPct, 70, 85)}" style="width: ${boundedPct(diskPct)}%;"></div></div>
              <span class="bar-val val-disk-pct">${diskPct}%</span>
            </div>
          </div>

          <div class="card-details">
            <div class="detail-row detail-network">
              <span class="label">网络</span>
              <span class="speed-text"><span class="speed-reading-up">↑ <span class="val-speed-up">${node.status.speed_up_str}</span></span><span class="speed-reading-down">↓ <span class="val-speed-down">${node.status.speed_down_str}</span></span></span>
            </div>
            <div class="detail-row detail-connections" title="TCP 为已建立连接数；UDP 为绑定套接字数">
              <span class="label">连接数</span>
              <span class="connection-counts"><span>TCP <b class="val-tcp-connections">${node.status.tcp_connections || 0}</b></span><span>UDP <b class="val-udp-sockets">${node.status.udp_sockets || 0}</b></span></span>
            </div>
            <div class="detail-row">
              <span class="label">累计流量</span>
              <span class="traffic-text"><span class="up">↑</span> <span class="val-traffic-up">${node.status.traffic_up_str}</span> <span class="down">↓</span> <span class="val-traffic-down">${node.status.traffic_down_str}</span></span>
            </div>
            <div class="detail-row">
              <span class="label">系统负载</span>
              <span class="val val-load">${node.status.load_str}</span>
            </div>
            <div class="detail-row detail-cycle-row">
              <span class="label">到期时间</span>
              <span class="val val-cycle">${node.billing.expire_date || '--'}</span>
            </div>
            <div class="detail-row">
              <span class="label">在线时长</span>
              <span class="val val-uptime">${node.status.uptime_str}</span>
            </div>
          </div>
        `;
        dom.nodeGrid.appendChild(card);
      } else {
        // 原地精细化更新已有卡片，零重排，零丢帧
        if (isOnline) {
          card.classList.remove('offline');
        } else {
          card.classList.add('offline');
        }

        // 国旗与系统徽章更新
        const flagSpan = card.querySelector('.node-flag');
        if (flagSpan) flagSpan.innerHTML = getFlagSVG(node.flag, node.name, node.region);
        const osBadge = card.querySelector('.node-os-badge');
        if (osBadge) osBadge.textContent = formatOSBadge(node.os);

        // GFW 测活状态徽章更新
        const header = card.querySelector('.card-header');
        if (header) {
          const badgePlaceholder = header.querySelector('.gfw-badge, .gfw-badge-placeholder');
          const targetHtml = renderGFWBadge(node.gfw_status);
          if (badgePlaceholder) {
            if (badgePlaceholder.outerHTML !== targetHtml) {
              badgePlaceholder.outerHTML = targetHtml;
            }
          } else {
            header.insertAdjacentHTML('beforeend', targetHtml);
          }
        }

        // 胶囊更新
        const capDays = card.querySelector('.capsule-days');
        if (capDays) {
          capDays.className = `capsule capsule-days ${dayClass}`;
          capDays.textContent = dayText;
        }

        // 硬件更新
        const valCores = card.querySelector('.val-cores');
        if (valCores) valCores.textContent = `${node.hardware.cpu_cores || 1} Cores`;
        const valRam = card.querySelector('.val-ram');
        if (valRam) valRam.textContent = node.hardware.ram_str;
        const valDisk = card.querySelector('.val-disk');
        if (valDisk) valDisk.textContent = node.hardware.disk_str;

        // 进度条与数值更新
        const barCpu = card.querySelector('.bar-fill-cpu');
        if (barCpu) { barCpu.style.width = `${boundedPct(cpuPct)}%`; barCpu.className = `bar-fill bar-fill-cpu ${healthClass(cpuPct, 60, 80)}`; }
        const valCpu = card.querySelector('.val-cpu-pct');
        if (valCpu) valCpu.textContent = `${cpuPct}%`;

        const barRam = card.querySelector('.bar-fill-ram');
        if (barRam) { barRam.style.width = `${boundedPct(ramPct)}%`; barRam.className = `bar-fill bar-fill-ram ${healthClass(ramPct, 70, 85)}`; }
        const valRamPct = card.querySelector('.val-ram-pct');
        if (valRamPct) valRamPct.textContent = `${ramPct}%`;

        const barSwap = card.querySelector('.bar-fill-swap');
        if (barSwap) barSwap.style.width = `${boundedPct(swapPct)}%`;
        const valSwap = card.querySelector('.val-swap-pct');
        if (valSwap) {
          valSwap.className = `bar-val val-swap-pct ${node.status.swap_off ? 'off' : ''}`;
          valSwap.textContent = node.status.swap_off ? 'OFF' : `${swapPct}%`;
        }

        const barDisk = card.querySelector('.bar-fill-disk');
        if (barDisk) { barDisk.style.width = `${boundedPct(diskPct)}%`; barDisk.className = `bar-fill bar-fill-disk ${healthClass(diskPct, 70, 85)}`; }
        const valDiskPct = card.querySelector('.val-disk-pct');
        if (valDiskPct) valDiskPct.textContent = `${diskPct}%`;

        // 详情行更新 (网速、流量、负载、周期)
        const speedUp = card.querySelector('.val-speed-up');
        if (speedUp) speedUp.textContent = node.status.speed_up_str;
        const speedDown = card.querySelector('.val-speed-down');
        if (speedDown) speedDown.textContent = node.status.speed_down_str;
		const tcpConnections = card.querySelector('.val-tcp-connections');
		if (tcpConnections) tcpConnections.textContent = node.status.tcp_connections || 0;
		const udpSockets = card.querySelector('.val-udp-sockets');
		if (udpSockets) udpSockets.textContent = node.status.udp_sockets || 0;

        const trafficUp = card.querySelector('.val-traffic-up');
        if (trafficUp) trafficUp.textContent = node.status.traffic_up_str;
        const trafficDown = card.querySelector('.val-traffic-down');
        if (trafficDown) trafficDown.textContent = node.status.traffic_down_str;

        const valLoad = card.querySelector('.val-load');
        if (valLoad) valLoad.textContent = node.status.load_str;

        const valCycle = card.querySelector('.val-cycle');
        if (valCycle) valCycle.textContent = node.billing.expire_date || '--';
        const valUptime = card.querySelector('.val-uptime');
        if (valUptime) valUptime.textContent = node.status.uptime_str;
      }
    });
  }

  // 工具辅助函数
  function formatBytes(b) {
    if (!b || b === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
    const i = Math.floor(Math.log(b) / Math.log(k));
    return parseFloat((b / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
  }

  function formatSpeed(b) {
    return formatBytes(b) + '/s';
  }

  function detectCountry(code, name, region) {
    return window.TianQueFlags.resolveCountryCode(code, name, region) || 'global';
  }

  function getFlagSVG(code, name, region) {
    return window.TianQueFlags.getFlagHTML(code, name, region);
  }
  function formatOSBadge(os) {
    if (!os) return 'Linux';
    os = os.toLowerCase();
    if (os.includes('debian')) return '🍥 Debian';
    if (os.includes('ubuntu')) return 'Ubuntu';
    if (os.includes('alpine')) return 'Alpine';
    if (os.includes('centos')) return 'CentOS';
    if (os.includes('rocky')) return 'Rocky';
    if (os.includes('alma')) return 'Alma';
    if (os.includes('arch')) return 'Arch';
    if (os.includes('windows')) return '🪟 Windows';
    if (os.includes('openwrt')) return 'OpenWrt';
    return os.charAt(0).toUpperCase() + os.slice(1);
  }

  function renderGFWBadge(status) {
    if (!status) return '<span class="gfw-badge-placeholder"></span>';
    if (status === 'ok') {
      return '<span class="gfw-badge ok" title="国内连通性：TCP连通畅通">✓ 畅通</span>';
    }
    if (status === 'blocked') {
      return '<span class="gfw-badge blocked" title="国内连通性：被墙或阻断超时">✕ 被墙</span>';
    }
    return '<span class="gfw-badge-placeholder"></span>';
  }

  function escapeHtml(str) {
    if (!str) return '';
    return str.replace(/[&<>"']/g, m => ({
      '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
    })[m]);
  }

  // 初始化
  initTheme();
  fetchSummary();
  pollTimer = setInterval(fetchSummary, 2000);
})();
