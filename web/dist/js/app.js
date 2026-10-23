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

  // 2. 跳转管理后台
  dom.btnAdmin.addEventListener('click', () => {
    window.location.href = '/admin.html';
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
      dom.valRegionCount.textContent = s.region_count;
      dom.valTrafficUp.textContent = `↑ ${formatBytes(s.total_net_out_bytes)}`;
      dom.valTrafficDown.textContent = `↓ ${formatBytes(s.total_net_in_bytes)}`;
      dom.valSpeedUp.textContent = `↑ ${formatSpeed(s.instant_speed_out)}`;
      dom.valSpeedDown.textContent = `↓ ${formatSpeed(s.instant_speed_in)}`;
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

  // 6. 渲染节点卡片 (纯 CSS 硬件合成加速 transform)
  function renderNodes(nodes) {
    const filtered = nodes.filter(n => currentGroup === '所有' || n.group === currentGroup);
    
    // 复用或全量重绘 DOM
    dom.nodeGrid.innerHTML = filtered.map(node => {
      const isOnline = node.status.is_online;
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

      const cpuScale = Math.min(node.status.cpu_pct / 100, 1);
      const ramScale = Math.min(node.status.ram_pct / 100, 1);
      const diskScale = Math.min(node.status.disk_pct / 100, 1);

      let swapScale = 0;
      let swapValHtml = '<span class="bar-val off">OFF</span>';
      if (!node.status.swap_off) {
        swapScale = Math.min(node.status.swap_pct / 100, 1);
        swapValHtml = `<span class="bar-val">${node.status.swap_pct}%</span>`;
      }

      return `
        <div class="node-card ${isOnline ? '' : 'offline'}" data-id="${node.id}">
          <div class="card-header">
            <div class="card-title-group">
              <span class="node-flag">${node.flag || '🌐'}</span>
              <span class="node-name" title="${escapeHtml(node.name)}">${escapeHtml(node.name)}</span>
              <span class="node-os-badge">${escapeHtml(node.os || 'linux')}</span>
            </div>
          </div>

          <div class="card-capsules">
            ${node.billing.price ? `<span class="capsule capsule-price">${escapeHtml(node.billing.price)}</span>` : ''}
            <span class="capsule capsule-days ${dayClass}">${dayText}</span>
          </div>

          <div class="card-hardware">
            <span>⚙️ ${node.hardware.cpu_cores || 1} Cores</span>
            <span>🗄️ ${node.hardware.ram_str}</span>
            <span>💾 ${node.hardware.disk_str}</span>
          </div>

          <div class="metrics-bars">
            <div class="bar-row">
              <span class="bar-label">CPU</span>
              <div class="bar-track">
                <div class="bar-fill cpu" style="transform: scaleX(${isOnline ? cpuScale : 0});"></div>
              </div>
              <span class="bar-val">${isOnline ? node.status.cpu_pct : 0}%</span>
            </div>
            <div class="bar-row">
              <span class="bar-label">内存</span>
              <div class="bar-track">
                <div class="bar-fill ram" style="transform: scaleX(${isOnline ? ramScale : 0});"></div>
              </div>
              <span class="bar-val">${isOnline ? node.status.ram_pct : 0}%</span>
            </div>
            <div class="bar-row">
              <span class="bar-label">SWAP</span>
              <div class="bar-track">
                <div class="bar-fill swap" style="transform: scaleX(${isOnline ? swapScale : 0});"></div>
              </div>
              ${swapValHtml}
            </div>
            <div class="bar-row">
              <span class="bar-label">硬盘</span>
              <div class="bar-track">
                <div class="bar-fill disk" style="transform: scaleX(${isOnline ? diskScale : 0});"></div>
              </div>
              <span class="bar-val">${isOnline ? node.status.disk_pct : 0}%</span>
            </div>
          </div>

          <div class="card-details">
            <div class="detail-row">
              <span>网络</span>
              <span><span class="up">↑ ${node.status.speed_up_str}</span> <span class="down">↓ ${node.status.speed_down_str}</span></span>
            </div>
            <div class="detail-row">
              <span>流量</span>
              <span><span class="up">↑ ${node.status.traffic_up_str}</span> <span class="down">↓ ${node.status.traffic_down_str}</span></span>
            </div>
            <div class="detail-row">
              <span>负载</span>
              <span class="val">${node.status.load_str}</span>
            </div>
            <div class="detail-row">
              <span>周期</span>
              <span>到期: ${node.billing.expire_date || '--'} &nbsp; ${node.status.uptime_str}</span>
            </div>
          </div>
        </div>
      `;
    }).join('');
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
