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
  let activeRouteQualityNode = '';
  const routeQualityHistoryCache = new Map();

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
    renderNodes(data.nodes || [], !!data.route_quality_enabled);
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
        closeRouteQualityModal();
        currentGroup = g;
        dom.groupFilterBar.querySelectorAll('.filter-btn').forEach(b => b.classList.remove('active'));
        btn.classList.add('active');
        if (cachedData) {
          renderNodes(cachedData.nodes || [], !!cachedData.route_quality_enabled);
        }
      });
      dom.groupFilterBar.appendChild(btn);
    });
  }

  // 6. 渲染节点卡片 (原地精确更新 DOM，彻底杜绝全量重排，零 GPU 负担)
  function renderNodes(nodes, routeQualityEnabled) {
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
            ${routeQualityRowHtml(node, routeQualityEnabled)}
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
        syncRouteQuality(card, node, routeQualityEnabled);

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

  function routeQualitySummary(rows) {
    const list = Array.isArray(rows) ? rows : [];
    const ready = list.filter(x => x.status && x.status !== 'pending' && x.status !== 'error' && x.status !== 'unavailable').length;
    return ready ? `三网 ${ready}/3` : '三网检测';
  }

  function routeQualityDetails(rows, histories = {}) {
    const list = Array.isArray(rows) ? rows : [];
    const ordered = ['电信', '移动', '联通'];
    return ordered.map(name => {
      const item = list.find(x => x.carrier === name) || { carrier:name, status:'pending' };
      const history = histories[name] || [];
      const stats = routeQualityHistoryStats(history);
      const currentLatency = Number(item.latency_ms);
      const currentLoss = item.loss_pct == null ? NaN : Number(item.loss_pct);
      const hasCurrent = ['ok', 'loss'].includes(item.status);
      const carrierClass = name === '电信' ? 'telecom' : name === '移动' ? 'mobile' : 'unicom';
      const latencyText = hasCurrent ? (currentLatency > 0 ? `${currentLatency.toFixed(1)} ms` : '超时') : '--';
      // CarrierQuality omits zero-valued LossPct in JSON; status=ok plus a missing value means 0%.
      const lossText = hasCurrent && (Number.isFinite(currentLoss) || item.status === 'ok')
        ? `${(Number.isFinite(currentLoss) ? currentLoss : 0).toFixed(1)}%` : '--';
      const statusText = item.status === 'error' || item.status === 'unavailable'
        ? escapeHtml(item.message || '暂无可用探针')
        : item.status === 'pending' ? '等待下一轮检测' : (item.network ? escapeHtml(item.network) : '检测结果正常');
      const avgLatency = stats.latencyCount ? `${stats.avgLatency.toFixed(1)} ms` : '--';
      const latencyRange = stats.latencyCount ? `${stats.minLatency.toFixed(1)}–${stats.maxLatency.toFixed(1)} ms` : '--';
      const avgLoss = stats.count ? `${stats.avgLoss.toFixed(1)}%` : '--';
      const peakLoss = stats.count ? `${stats.maxLoss.toFixed(1)}%` : '--';
      return `<article class="rq-result rq-${carrierClass}-card">
        <div class="rq-result-head"><span class="rq-carrier"><i class="rq-dot rq-${carrierClass}"></i>${name}</span><span class="rq-result-note">${statusText}</span></div>
        <div class="rq-current-grid"><div><span>当前延迟</span><strong class="${currentLatency > 0 && hasCurrent ? latencyQualityClass(currentLatency) : ''}">${latencyText}</strong></div><div><span>当前丢包</span><strong>${lossText}</strong></div></div>
        <div class="rq-period-head"><strong>近 24 小时统计</strong><span>${stats.count} 次有效检测</span></div>
        <div class="rq-stats-grid">
          <div><span>平均延迟</span><strong>${avgLatency}</strong></div>
          <div><span>延迟范围</span><strong>${latencyRange}</strong></div>
          <div class="rq-stat-highlight"><span>平均丢包率</span><strong>${avgLoss}</strong></div>
          <div><span>最高丢包率</span><strong>${peakLoss}</strong></div>
        </div>
        <div class="rq-history" data-carrier="${name}">${routeQualityHistoryChart(history)}</div>
      </article>`;
    }).join('');
  }

  function latencyQualityClass(latency) {
    if (latency <= 100) return 'rq-latency-good';
    if (latency <= 200) return 'rq-latency-warn';
    return 'rq-latency-poor';
  }

  function routeQualityHistoryStats(points) {
    const cutoff = Date.now() - 24 * 60 * 60 * 1000;
    const samples = (Array.isArray(points) ? points : []).map(point => ({
      at: Date.parse(point.checked_at || ''),
      latency: Number(point.latency_ms),
      loss: point.loss_pct == null ? NaN : Number(point.loss_pct),
      status: point.status
    })).filter(point => Number.isFinite(point.at) && point.at >= cutoff && point.at <= Date.now() &&
      ['ok', 'loss'].includes(point.status) && Number.isFinite(point.loss));
    const latency = samples.map(point => point.latency).filter(value => Number.isFinite(value) && value > 0);
    return {
      count: samples.length,
      latencyCount: latency.length,
      avgLatency: latency.length ? latency.reduce((sum, value) => sum + value, 0) / latency.length : 0,
      minLatency: latency.length ? Math.min(...latency) : 0,
      maxLatency: latency.length ? Math.max(...latency) : 0,
      avgLoss: samples.length ? samples.reduce((sum, point) => sum + point.loss, 0) / samples.length : 0,
      maxLoss: samples.length ? Math.max(...samples.map(point => point.loss)) : 0
    };
  }

  function routeQualityHistoryChart(points) {
    const cutoff = Date.now() - 24 * 60 * 60 * 1000;
    const samples = (Array.isArray(points) ? points : []).map(point => ({
      at: Date.parse(point.checked_at || ''),
      latency: Number(point.latency_ms),
      loss: point.loss_pct == null ? NaN : Number(point.loss_pct),
      status: point.status
    })).filter(point => Number.isFinite(point.at) && point.at >= cutoff && point.at <= Date.now() &&
      ['ok', 'loss'].includes(point.status) && Number.isFinite(point.loss));
    samples.sort((a, b) => a.at - b.at);
    if (!samples.length) return '<div class="rq-history-empty"><strong>还没有 24 小时历史</strong><span>完成下一轮检测后，延迟曲线、丢包率和时间统计会显示在这里。</span></div>';

    const left = 48, right = 348;
    const latencyTop = 24, latencyBottom = 99;
    const lossTop = 132, lossBottom = 174;
    const latencies = samples.map(point => point.latency).filter(value => value > 0);
    let min = latencies.length ? Math.min(...latencies) : 0;
    let max = latencies.length ? Math.max(...latencies) : 1;
    const pad = Math.max((max - min) * 0.18, max * 0.08, 2);
    min = Math.max(0, min - pad);
    max += pad;
    if (max <= min) max = min + 1;
    const x = at => left + Math.max(0, Math.min(1, (at - cutoff) / (24 * 60 * 60 * 1000))) * (right - left);
    const yLatency = latency => latencyBottom - ((latency - min) / (max - min)) * (latencyBottom - latencyTop);
    const maxLoss = Math.max(...samples.map(point => point.loss));
    const lossDomain = maxLoss <= 1 ? 1 : maxLoss <= 5 ? 5 : maxLoss <= 10 ? 10 : maxLoss <= 25 ? 25 : Math.min(100, Math.ceil(maxLoss / 25) * 25);
    const yLoss = loss => lossBottom - (Math.max(0, Math.min(lossDomain, loss)) / lossDomain) * (lossBottom - lossTop);
    const latencySamples = samples.filter(point => point.latency > 0);
    const latencySegments = latencySamples.slice(1).map((point, index) => {
      const previous = latencySamples[index];
      const quality = latencyQualityClass((previous.latency + point.latency) / 2);
      return `<path class="rq-latency-segment ${quality}" d="M${x(previous.at).toFixed(1)},${yLatency(previous.latency).toFixed(1)} L${x(point.at).toFixed(1)},${yLatency(point.latency).toFixed(1)}"/>`;
    }).join('');
    const lossPath = samples.map((point, index) => `${index ? 'L' : 'M'}${x(point.at).toFixed(1)},${yLoss(point.loss).toFixed(1)}`).join(' ');
    const lossArea = samples.length ? `${lossPath} L${x(samples[samples.length - 1].at).toFixed(1)},${lossBottom} L${x(samples[0].at).toFixed(1)},${lossBottom} Z` : '';
    const formatTime = at => new Date(at).toLocaleString([], { month:'2-digit', day:'2-digit', hour:'2-digit', minute:'2-digit' });
    const tooltip = point => `${formatTime(point.at)} · 延迟 ${point.latency > 0 ? `${point.latency.toFixed(1)} ms` : '超时'} · 丢包 ${point.loss.toFixed(1)}%`;
    const pointMarkup = (point, metric) => {
      const valueY = metric === 'latency' ? yLatency(point.latency) : yLoss(point.loss);
      const title = tooltip(point);
      const quality = metric === 'latency' ? ` ${latencyQualityClass(point.latency)}` : '';
      return `<g class="rq-chart-point-group"><circle class="rq-chart-point rq-chart-point-${metric}${quality}" cx="${x(point.at).toFixed(1)}" cy="${valueY.toFixed(1)}" r="8" tabindex="0" role="img" aria-label="${escapeHtml(title)}" data-tooltip="${escapeHtml(title)}"><title>${escapeHtml(title)}</title></circle><circle class="rq-chart-marker rq-chart-marker-${metric}${quality}" cx="${x(point.at).toFixed(1)}" cy="${valueY.toFixed(1)}" r="2.5" aria-hidden="true"/></g>`;
    };
    const latencyPoints = latencySamples.map(point => pointMarkup(point, 'latency')).join('');
    const lossPoints = samples.map(point => pointMarkup(point, 'loss')).join('');
    const xTicks = [0, 6, 12, 18, 24].map(hour => {
      const tx = left + (hour / 24) * (right - left);
      const label = hour === 24 ? '现在' : `${hour}h`;
      return `<line class="rq-chart-vgrid" x1="${tx}" y1="${latencyTop}" x2="${tx}" y2="${lossBottom}"/><text class="rq-chart-time" x="${tx}" y="204" text-anchor="${hour === 0 ? 'start' : hour === 24 ? 'end' : 'middle'}">${label}</text>`;
    }).join('');
    const latencyTicks = [max, (max + min) / 2, min].map(value => {
      const ty = yLatency(value);
      return `<line class="rq-chart-grid" x1="${left}" y1="${ty.toFixed(1)}" x2="${right}" y2="${ty.toFixed(1)}"/><text class="rq-chart-ylabel" x="${left - 6}" y="${(ty + 3).toFixed(1)}" text-anchor="end">${value.toFixed(0)}</text>`;
    }).join('');
    const lossTicks = [lossDomain, lossDomain / 2, 0].map(value => {
      const ty = yLoss(value);
      return `<line class="rq-chart-grid" x1="${left}" y1="${ty.toFixed(1)}" x2="${right}" y2="${ty.toFixed(1)}"/><text class="rq-chart-ylabel" x="${left - 6}" y="${(ty + 3).toFixed(1)}" text-anchor="end">${value.toFixed(0)}%</text>`;
    }).join('');
    return `<div class="rq-chart-legend"><span class="rq-latency-quality-legend"><i class="rq-latency-good"></i>≤100 ms <i class="rq-latency-warn"></i>100–200 ms <i class="rq-latency-poor"></i>&gt;200 ms</span><span><i class="rq-legend-loss"></i>丢包率（0–${lossDomain}%）</span><span class="rq-chart-hint">延迟按绿/黄/红分级；悬停或点击采样点查看数值</span></div>
      <svg class="rq-history-svg rq-history-svg-detailed" viewBox="0 0 360 212" role="img" aria-label="过去 24 小时延迟和丢包率趋势">
        ${xTicks}${latencyTicks}${lossTicks}
        <text class="rq-chart-section-label" x="${left}" y="15">往返延迟</text><text class="rq-chart-section-label" x="${left}" y="123">丢包率</text>
        <g class="rq-latency-dots">${latencySegments}${latencyPoints}</g>
        <path class="rq-loss-area" d="${lossArea}"/><path class="rq-loss-line" d="${lossPath}"/><g class="rq-loss-dots">${lossPoints}</g>
      </svg>`;
  }

  function showRouteQualityTooltip(point, clientX, clientY) {
    if (!point) return;
    let tooltip = document.getElementById('rqChartTooltip');
    if (!tooltip) {
      tooltip = document.createElement('div');
      tooltip.id = 'rqChartTooltip';
      tooltip.className = 'rq-chart-tooltip';
      tooltip.setAttribute('role', 'status');
      document.body.appendChild(tooltip);
    }
    tooltip.textContent = point.dataset.tooltip || point.getAttribute('aria-label') || '';
    tooltip.hidden = false;
    const margin = 12;
    const rect = tooltip.getBoundingClientRect();
    const left = Math.max(8, Math.min(window.innerWidth - rect.width - 8, clientX + margin));
    const top = Math.max(8, Math.min(window.innerHeight - rect.height - 8, clientY + margin));
    tooltip.style.left = `${left}px`;
    tooltip.style.top = `${top}px`;
  }

  function hideRouteQualityTooltip() {
    const tooltip = document.getElementById('rqChartTooltip');
    if (tooltip) tooltip.hidden = true;
  }

  async function loadRouteQualityHistory(nodeID) {
    const cached = routeQualityHistoryCache.get(nodeID);
    if (cached && Date.now() - cached.loadedAt < 5 * 60 * 1000) {
      applyRouteQualityHistory(nodeID, cached.data);
      return;
    }
    if (cached && cached.promise) return cached.promise;
    const entry = { loadedAt: 0, data: {}, promise: null };
    entry.promise = fetch(`/api/nodes/route-quality/history?node_id=${encodeURIComponent(nodeID)}`)
      .then(response => { if (!response.ok) throw new Error(`HTTP ${response.status}`); return response.json(); })
      .then(payload => {
        entry.data = payload.history || {};
        entry.loadedAt = Date.now();
        entry.promise = null;
        routeQualityHistoryCache.set(nodeID, entry);
        applyRouteQualityHistory(nodeID, entry.data);
      })
      .catch(() => {
        entry.promise = null;
        applyRouteQualityHistory(nodeID, {});
      });
    routeQualityHistoryCache.set(nodeID, entry);
    return entry.promise;
  }

  function applyRouteQualityHistory(nodeID, histories) {
    dom.nodeGrid.querySelectorAll('.detail-route-quality').forEach(row => {
      if (row.dataset.routeNode !== nodeID) return;
      row.querySelectorAll('.rq-history').forEach(container => {
        container.innerHTML = routeQualityHistoryChart(histories[container.dataset.carrier] || []);
      });
    });
    if (activeRouteQualityNode === nodeID) {
      const node = cachedData?.nodes?.find(item => item.id === nodeID);
      const results = document.querySelector('#routeQualityModal .rq-modal-results');
      if (node && results) {
        const rows = Array.isArray(node.route_quality) ? node.route_quality : [];
        results.innerHTML = routeQualityDetails(rows, histories);
      }
    }
  }

  function openRouteQualityModal(nodeID) {
    const node = cachedData?.nodes?.find(item => item.id === nodeID);
    if (!node) return;
    activeRouteQualityNode = nodeID;
    const history = routeQualityHistoryCache.get(nodeID)?.data || {};
    const rows = Array.isArray(node.route_quality) ? node.route_quality : [];
    const latest = rows.map(item => Date.parse(item.checked_at || '')).filter(Number.isFinite);
    const latestAt = latest.length ? Math.max(...latest) : 0;
    const staleAfter = Math.max(60, Number(cachedData?.route_quality_interval_sec) || 1800) * 2.5 * 1000;
    const updated = latestAt
      ? `${Date.now() - latestAt > staleAfter ? '数据滞后 · ' : ''}更新 ${new Date(latestAt).toLocaleTimeString([], { hour:'2-digit', minute:'2-digit' })}`
      : '尚未检测';
    let modal = document.getElementById('routeQualityModal');
    if (!modal) {
      modal = document.createElement('div');
      modal.id = 'routeQualityModal';
      modal.className = 'route-quality-modal';
      modal.innerHTML = '<div class="rq-modal-backdrop" data-rq-close></div><section class="rq-modal-panel" role="dialog" aria-modal="true" aria-labelledby="rqModalTitle"><header class="rq-modal-header"><div><h2 id="rqModalTitle">国内三网 ICMP</h2><div class="rq-modal-subtitle"></div></div><button type="button" class="rq-modal-close" aria-label="关闭">×</button></header><div class="rq-modal-results"></div><footer class="rq-modal-foot">近 24 小时均值按有效探测轮次计算 · 悬停或聚焦图表采样点查看该轮精确数值</footer></section>';
      document.body.appendChild(modal);
      modal.addEventListener('click', event => {
        if (event.target.closest('[data-rq-close], .rq-modal-close')) closeRouteQualityModal();
        const point = event.target.closest('.rq-chart-point');
        if (point) {
          const rect = point.getBoundingClientRect();
          showRouteQualityTooltip(point, rect.left + rect.width / 2, rect.top + rect.height / 2);
          window.setTimeout(hideRouteQualityTooltip, 3500);
        }
      });
      modal.addEventListener('pointerover', event => {
        const point = event.target.closest('.rq-chart-point');
        if (point) showRouteQualityTooltip(point, event.clientX, event.clientY);
      });
      modal.addEventListener('pointermove', event => {
        const point = event.target.closest('.rq-chart-point');
        if (point) showRouteQualityTooltip(point, event.clientX, event.clientY);
      });
      modal.addEventListener('pointerout', event => {
        if (event.target.closest('.rq-chart-point') && !event.relatedTarget?.closest?.('.rq-chart-point')) hideRouteQualityTooltip();
      });
      modal.addEventListener('focusin', event => {
        const point = event.target.closest('.rq-chart-point');
        if (point) {
          const rect = point.getBoundingClientRect();
          showRouteQualityTooltip(point, rect.left + rect.width / 2, rect.top + rect.height / 2);
        }
      });
      modal.addEventListener('focusout', event => {
        if (event.target.closest('.rq-chart-point')) hideRouteQualityTooltip();
      });
    }
    modal.querySelector('.rq-modal-subtitle').textContent = `${node.name} · ${updated}`;
    modal.querySelector('.rq-modal-results').innerHTML = routeQualityDetails(rows, history);
    modal.classList.add('open');
    modal.querySelector('.rq-modal-close').focus();
    loadRouteQualityHistory(nodeID);
  }

  function closeRouteQualityModal() {
    const modal = document.getElementById('routeQualityModal');
    if (modal) modal.classList.remove('open');
    hideRouteQualityTooltip();
    activeRouteQualityNode = '';
  }

  function routeQualityRowHtml(node, enabled) {
    if (!enabled) return '';
    const rows = node.route_quality || [];
    return `<div class="detail-row detail-route-quality" data-route-node="${escapeHtml(node.id)}">
      <span class="label">国内线路</span>
      <button type="button" class="route-quality-trigger" aria-haspopup="dialog" title="点击查看国内三网 ICMP 延迟与丢包">${routeQualitySummary(rows)} <span aria-hidden="true">⌄</span></button>
    </div>`;
  }

  function syncRouteQuality(card, node, enabled) {
    const existing = card.querySelector('.detail-route-quality');
    if (!enabled) { if (existing) existing.remove(); if (activeRouteQualityNode === node.id) closeRouteQualityModal(); return; }
    const html = routeQualityRowHtml(node, true);
    if (!existing) {
      const network = card.querySelector('.detail-network');
      if (network) network.insertAdjacentHTML('afterend', html);
      return;
    }
    existing.outerHTML = html;
  }

  dom.nodeGrid.addEventListener('click', (event) => {
    const trigger = event.target.closest('.route-quality-trigger');
    if (trigger) {
      const nodeID = trigger.closest('.node-card')?.dataset.id;
      if (nodeID && activeRouteQualityNode === nodeID) closeRouteQualityModal();
      else if (nodeID) openRouteQualityModal(nodeID);
      event.stopPropagation();
      return;
    }
  });

  document.addEventListener('keydown', event => { if (event.key === 'Escape') closeRouteQualityModal(); });

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
