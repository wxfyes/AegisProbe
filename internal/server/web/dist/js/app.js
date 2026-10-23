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
        const c = detectCountry(n.flag || n.region, n.name);
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

      if (!card) {
        // 首次挂载创建 DOM 节点
        card = document.createElement('div');
        card.className = `node-card ${isOnline ? '' : 'offline'}`;
        card.dataset.id = node.id;
        card.innerHTML = `
          <div class="card-header">
            <div class="card-title-group">
              <span class="node-flag">${getFlagSVG(node.flag || node.region, node.name)}</span>
              <span class="node-name" title="${escapeHtml(node.name)}">${escapeHtml(node.name)}</span>
              <span class="node-os-badge">${formatOSBadge(node.os)}</span>
            </div>
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

          <!-- 实时网速微仪表盘卡片 (复刻图一高大上质感) -->
          <div class="speed-pill-group">
            <div class="speed-pill up">
              <div class="pill-header"><span>实时 ↑</span></div>
              <div class="pill-value"><span class="val-speed-up">${node.status.speed_up_str}</span></div>
            </div>
            <div class="speed-pill down">
              <div class="pill-header"><span>实时 ↓</span></div>
              <div class="pill-value"><span class="val-speed-down">${node.status.speed_down_str}</span></div>
            </div>
          </div>

          <div class="metrics-bars">
            <div class="bar-row">
              <span class="bar-label">CPU</span>
              <div class="bar-track"><div class="bar-fill bar-fill-cpu" style="width: ${cpuPct}%;"></div></div>
              <span class="bar-val val-cpu-pct">${cpuPct}%</span>
            </div>
            <div class="bar-row">
              <span class="bar-label">内存</span>
              <div class="bar-track"><div class="bar-fill bar-fill-ram" style="width: ${ramPct}%;"></div></div>
              <span class="bar-val val-ram-pct">${ramPct}%</span>
            </div>
            <div class="bar-row">
              <span class="bar-label">SWAP</span>
              <div class="bar-track"><div class="bar-fill bar-fill-swap" style="width: ${swapPct}%;"></div></div>
              <span class="bar-val val-swap-pct ${node.status.swap_off ? 'off' : ''}">${node.status.swap_off ? 'OFF' : swapPct + '%'}</span>
            </div>
            <div class="bar-row">
              <span class="bar-label">硬盘</span>
              <div class="bar-track"><div class="bar-fill bar-fill-disk" style="width: ${diskPct}%;"></div></div>
              <span class="bar-val val-disk-pct">${diskPct}%</span>
            </div>
          </div>

          <div class="card-details">
            <div class="detail-row">
              <span class="label">累计流量</span>
              <span class="traffic-text"><span class="up">↑</span> <span class="val-traffic-up">${node.status.traffic_up_str}</span> <span class="down">↓</span> <span class="val-traffic-down">${node.status.traffic_down_str}</span></span>
            </div>
            <div class="detail-row">
              <span class="label">系统负载</span>
              <span class="val val-load">${node.status.load_str}</span>
            </div>
            <div class="detail-row">
              <span class="label">运行周期</span>
              <span class="val val-cycle">到期: ${node.billing.expire_date || '--'} &nbsp; ${node.status.uptime_str}</span>
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
        if (flagSpan) flagSpan.innerHTML = getFlagSVG(node.flag || node.region, node.name);
        const osBadge = card.querySelector('.node-os-badge');
        if (osBadge) osBadge.textContent = formatOSBadge(node.os);

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
        if (barCpu) barCpu.style.width = `${cpuPct}%`;
        const valCpu = card.querySelector('.val-cpu-pct');
        if (valCpu) valCpu.textContent = `${cpuPct}%`;

        const barRam = card.querySelector('.bar-fill-ram');
        if (barRam) barRam.style.width = `${ramPct}%`;
        const valRamPct = card.querySelector('.val-ram-pct');
        if (valRamPct) valRamPct.textContent = `${ramPct}%`;

        const barSwap = card.querySelector('.bar-fill-swap');
        if (barSwap) barSwap.style.width = `${swapPct}%`;
        const valSwap = card.querySelector('.val-swap-pct');
        if (valSwap) {
          valSwap.className = `bar-val val-swap-pct ${node.status.swap_off ? 'off' : ''}`;
          valSwap.textContent = node.status.swap_off ? 'OFF' : `${swapPct}%`;
        }

        const barDisk = card.querySelector('.bar-fill-disk');
        if (barDisk) barDisk.style.width = `${diskPct}%`;
        const valDiskPct = card.querySelector('.val-disk-pct');
        if (valDiskPct) valDiskPct.textContent = `${diskPct}%`;

        // 详情行更新 (网速、流量、负载、周期)
        const speedUp = card.querySelector('.val-speed-up');
        if (speedUp) speedUp.textContent = node.status.speed_up_str;
        const speedDown = card.querySelector('.val-speed-down');
        if (speedDown) speedDown.textContent = node.status.speed_down_str;

        const trafficUp = card.querySelector('.val-traffic-up');
        if (trafficUp) trafficUp.textContent = node.status.traffic_up_str;
        const trafficDown = card.querySelector('.val-traffic-down');
        if (trafficDown) trafficDown.textContent = node.status.traffic_down_str;

        const valLoad = card.querySelector('.val-load');
        if (valLoad) valLoad.textContent = node.status.load_str;

        const valCycle = card.querySelector('.val-cycle');
        if (valCycle) valCycle.textContent = `到期: ${node.billing.expire_date || '--'}   ${node.status.uptime_str}`;
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

  // 智能地区代码提取函数
  function detectCountry(code, name) {
    name = (name || '').toLowerCase();
    code = (code || '').trim().toLowerCase();
    if (code === 'hk' || name.includes('香港') || name.includes('hk')) return 'hk';
    if (code === 'tw' || name.includes('台湾') || name.includes('tw') || name.includes('hinet')) return 'tw';
    if (code === 'jp' || name.includes('日本') || name.includes('东京') || name.includes('大阪') || name.includes('jp')) return 'jp';
    if (code === 'sg' || name.includes('新加坡') || name.includes('狮城') || name.includes('sg')) return 'sg';
    if (code === 'us' || name.includes('美国') || name.includes('美') || name.includes('us') || name.includes('洛杉矶')) return 'us';
    if (code === 'cn' || name.includes('中国') || name.includes('国内') || name.includes('北京') || name.includes('上海') || name.includes('广州') || name.includes('深圳')) return 'cn';
    if (code === 'de' || name.includes('德国') || name.includes('法兰克福') || name.includes('de')) return 'de';
    if (code === 'gb' || code === 'uk' || name.includes('英国') || name.includes('伦敦') || name.includes('uk')) return 'gb';
    if (code === 'kr' || name.includes('韩国') || name.includes('首尔') || name.includes('kr')) return 'kr';
    if (code === 'fr' || name.includes('法国') || name.includes('巴黎')) return 'fr';
    return (code && code !== 'global' && code !== 'un') ? code : 'global';
  }

  // 高清矢量 SVG 国旗渲染引擎 (彻底消灭 Windows 平台不支持 Emoji 导致的大地球和字符)
  function getFlagSVG(code, name) {
    const country = detectCountry(code, name);

    switch (country) {
      case 'hk': // 中国香港
        return `<span class="flag-container" title="中国香港"><svg viewBox="0 0 600 400"><rect width="600" height="400" fill="#c8102e"/><circle cx="300" cy="200" r="95" fill="none" stroke="#fff" stroke-width="12"/><g fill="#fff" transform="translate(300,200)"><path d="M0,-85 C25,-40 45,-20 75,-10 C45,-10 25,10 0,65 C-25,10 -45,-10 -75,-10 C-45,-20 -25,-40 0,-85 Z"/><circle cx="0" cy="0" r="18" fill="#c8102e"/><circle cx="0" cy="0" r="10" fill="#fff"/></g></svg></span>`;
      case 'tw': // 中国台湾
        return `<span class="flag-container" title="中国台湾"><svg viewBox="0 0 600 400"><rect width="600" height="400" fill="#fe0000"/><rect width="300" height="200" fill="#000095"/><circle cx="150" cy="100" r="50" fill="#fff"/><circle cx="150" cy="100" r="35" fill="#000095"/><circle cx="150" cy="100" r="22" fill="#fff"/></svg></span>`;
      case 'jp': // 日本
        return `<span class="flag-container" title="日本"><svg viewBox="0 0 600 400"><rect width="600" height="400" fill="#ffffff"/><circle cx="300" cy="200" r="120" fill="#bc002d"/></svg></span>`;
      case 'sg': // 新加坡
        return `<span class="flag-container" title="新加坡"><svg viewBox="0 0 600 400"><rect width="600" height="200" fill="#ed2939"/><rect y="200" width="600" height="200" fill="#ffffff"/><circle cx="150" cy="100" r="65" fill="#ffffff"/><circle cx="175" cy="100" r="58" fill="#ed2939"/></svg></span>`;
      case 'us': // 美国
        return `<span class="flag-container" title="美国"><svg viewBox="0 0 600 400"><rect width="600" height="400" fill="#b22234"/><path d="M0,31H600M0,92H600M0,154H600M0,215H600M0,277H600M0,338H600" stroke="#fff" stroke-width="31"/><rect width="240" height="215" fill="#3c3b6e"/></svg></span>`;
      case 'de': // 德国
        return `<span class="flag-container" title="德国"><svg viewBox="0 0 600 400"><rect width="600" height="133" fill="#000000"/><rect y="133" width="600" height="134" fill="#dd0000"/><rect y="267" width="600" height="133" fill="#ffce00"/></svg></span>`;
      case 'cn': // 中国
        return `<span class="flag-container" title="中国"><svg viewBox="0 0 600 400"><rect width="600" height="400" fill="#de2910"/><polygon fill="#ffde00" points="100,50 120,110 68,73 132,73 80,110"/></svg></span>`;
      case 'gb': // 英国
        return `<span class="flag-container" title="英国"><svg viewBox="0 0 600 400"><rect width="600" height="400" fill="#012169"/><path d="M0,0 L600,400 M600,0 L0,400" stroke="#fff" stroke-width="60"/><path d="M0,0 L600,400 M600,0 L0,400" stroke="#c8102e" stroke-width="20"/><path d="M300,0 V400 M0,200 H600" stroke="#fff" stroke-width="100"/><path d="M300,0 V400 M0,200 H600" stroke="#c8102e" stroke-width="60"/></svg></span>`;
      case 'kr': // 韩国
        return `<span class="flag-container" title="韩国"><svg viewBox="0 0 600 400"><rect width="600" height="400" fill="#ffffff"/><circle cx="300" cy="200" r="100" fill="#c60c30"/><path d="M300,300 A100,100 0 0,1 300,100 A50,50 0 0,1 300,200 A50,50 0 0,0 300,300 Z" fill="#003478"/></svg></span>`;
      default: // 全球精美科技图标
        return `<span class="flag-container" title="全球服务器"><svg viewBox="0 0 600 400"><rect width="600" height="400" fill="#1e293b"/><circle cx="300" cy="200" r="140" fill="none" stroke="#38bdf8" stroke-width="18"/><ellipse cx="300" cy="200" rx="70" ry="140" fill="none" stroke="#38bdf8" stroke-width="14"/><line x1="160" y1="200" x2="440" y2="200" stroke="#38bdf8" stroke-width="14"/></svg></span>`;
    }
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
