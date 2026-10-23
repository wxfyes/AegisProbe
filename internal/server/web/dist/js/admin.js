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

    // 表单元素
    cfgSiteTitle: document.getElementById('cfgSiteTitle'),
    cfgAccessMode: document.getElementById('cfgAccessMode'),
    cfgAdminPassword: document.getElementById('cfgAdminPassword'),
    cfgOfflineSec: document.getElementById('cfgOfflineSec'),
    cfgTGEnabled: document.getElementById('cfgTGEnabled'),
    cfgTGBotToken: document.getElementById('cfgTGBotToken'),
    cfgTGChatID: document.getElementById('cfgTGChatID'),
    cfgTGProxy: document.getElementById('cfgTGProxy'),

    nodeId: document.getElementById('nodeId'),
    nodeName: document.getElementById('nodeName'),
    nodeRegion: document.getElementById('nodeRegion'),
    nodeGroup: document.getElementById('nodeGroup'),
    nodePrice: document.getElementById('nodePrice'),
    nodeExpireDate: document.getElementById('nodeExpireDate'),
    nodeToken: document.getElementById('nodeToken'),
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

  // 5. 将国家/地区代码自动转为正规 Unicode 国旗 Emoji
  function codeToFlag(code) {
    if (!code) return '🌐';
    code = code.trim().toUpperCase();
    if (code === 'TW') return '🇹🇼';
    if (code === 'HK') return '🇭🇰';
    if (code === 'MO') return '🇲🇴';
    if (code === 'CN') return '🇨🇳';
    if (code === 'GLOBAL' || code === 'DEFAULT') return '🌐';
    if (code.length === 2 && code[0] >= 'A' && code[0] <= 'Z' && code[1] >= 'A' && code[1] <= 'Z') {
      const r1 = code.charCodeAt(0) - 65 + 0x1F1E6;
      const r2 = code.charCodeAt(1) - 65 + 0x1F1E6;
      return String.fromCodePoint(r1, r2);
    }
    return code; // 如果传入本身就是 Emoji
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

  function renderSettings(cfg) {
    dom.cfgSiteTitle.value = cfg.site_title || '';
    dom.cfgAccessMode.value = cfg.access_mode || 'public';
    dom.cfgAdminPassword.value = '';
    dom.cfgOfflineSec.value = cfg.offline_sec || 60;
    dom.cfgTGEnabled.checked = !!cfg.tg_enabled;
    dom.cfgTGBotToken.value = cfg.tg_bot_token || '';
    dom.cfgTGChatID.value = cfg.tg_chat_id || '';
    dom.cfgTGProxy.value = cfg.tg_proxy || '';
  }

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
      dom.nodeTableBody.innerHTML = `<tr><td colspan="8" style="text-align: center; color: var(--text-muted); padding: 30px;">暂无匹配的服务器节点</td></tr>`;
      return;
    }

    dom.nodeTableBody.innerHTML = filtered.map(n => {
      const flagEmoji = codeToFlag(n.flag || n.region);
      return `
        <tr>
          <td><code style="color: var(--accent-blue); font-weight: 700;">${escapeHtml(n.id)}</code></td>
          <td>
            <div style="display: flex; align-items: center; gap: 8px;">
              <span style="font-size: 18px; line-height: 1;">${flagEmoji}</span>
              <b style="font-size: 14px; color: var(--text-main);">${escapeHtml(n.name)}</b>
            </div>
          </td>
          <td>
            <div style="display: flex; align-items: center; gap: 6px; font-family: var(--font-mono); font-size: 12px;">
              <span>${escapeHtml(n.region || '自动识别')}</span>
            </div>
          </td>
          <td>
            <span class="node-os-badge">${escapeHtml(n.os || 'Linux')}</span>
          </td>
          <td><span class="capsule capsule-days green">${escapeHtml(n.group || '默认')}</span></td>
          <td><span class="capsule capsule-price">${escapeHtml(n.price || '--')}</span></td>
          <td><span style="font-family: var(--font-mono); font-size: 12px; color: var(--text-sub);">${escapeHtml(n.expire_date || '--')}</span></td>
          <td style="text-align: right;">
            <div style="display: inline-flex; gap: 6px;">
              <button class="btn-sm" onclick="window.showInstallCmd('${escapeHtml(n.id)}')">一键命令</button>
              <button class="btn-sm" onclick="window.editNode('${escapeHtml(n.id)}')">编辑</button>
              <button class="btn-sm danger" onclick="window.deleteNode('${escapeHtml(n.id)}')">删除</button>
            </div>
          </td>
        </tr>
      `;
    }).join('');
  }

  // 搜索框输入过滤
  dom.nodeSearchInput.addEventListener('input', () => {
    renderNodesTable(allNodes);
  });

  // 7. 保存全局系统配置
  dom.settingsForm.addEventListener('submit', async (e) => {
    e.preventDefault();
    const payload = {
      site_title: dom.cfgSiteTitle.value.trim(),
      access_mode: dom.cfgAccessMode.value,
      admin_password: dom.cfgAdminPassword.value.trim(),
      offline_sec: parseInt(dom.cfgOfflineSec.value, 10) || 60,
      tg_enabled: dom.cfgTGEnabled.checked,
      tg_bot_token: dom.cfgTGBotToken.value.trim(),
      tg_chat_id: dom.cfgTGChatID.value.trim(),
      tg_proxy: dom.cfgTGProxy.value.trim()
    };

    try {
      const res = await fetch('/api/admin/config', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      });
      const data = await res.json();
      if (res.ok && data.ok) {
        alert('✅ 系统配置已成功保存！');
        loadAdminConfig();
      } else {
        alert('❌ 保存失败: ' + (data.error || '未知错误'));
      }
    } catch (err) {
      alert('❌ 网络通信异常');
    }
  });

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
    dom.nodeToken.value = '';

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
    dom.nodeToken.value = n.token || '';

    dom.nodeModal.style.display = 'flex';
  };

  dom.nodeForm.addEventListener('submit', async (e) => {
    e.preventDefault();
    const regionCode = dom.nodeRegion.value.trim().toUpperCase();
    const autoFlag = codeToFlag(regionCode);

    const payload = {
      id: dom.nodeId.value.trim(),
      name: dom.nodeName.value.trim(),
      flag: autoFlag,
      region: regionCode,
      os: 'linux', // 优先由 Agent 真实上报采集，无需手动写死
      group: dom.nodeGroup.value.trim(),
      price: dom.nodePrice.value.trim(),
      expire_date: dom.nodeExpireDate.value.replace(/-/g, '/'),
      token: dom.nodeToken.value.trim()
    };

    try {
      const res = await fetch('/api/admin/node/save', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      });
      const data = await res.json();
      if (res.ok && data.ok) {
        dom.nodeModal.style.display = 'none';
        loadAdminConfig();
      } else {
        alert('❌ 保存节点失败: ' + (data.error || ''));
      }
    } catch (err) {
      alert('❌ 网络通信异常');
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
    const text = dom.cmdText.textContent;

    if (navigator.clipboard && navigator.clipboard.writeText) {
      navigator.clipboard.writeText(text).then(() => {
        dom.btnCopyCmd.textContent = '✅ 已复制！';
        setTimeout(() => { dom.btnCopyCmd.textContent = '📋 复制命令'; }, 2000);
      }).catch(() => fallbackCopy(text));
    } else {
      fallbackCopy(text);
    }
  });

  function fallbackCopy(text) {
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
      dom.btnCopyCmd.textContent = '✅ 已复制！';
      setTimeout(() => { dom.btnCopyCmd.textContent = '📋 复制命令'; }, 2000);
    } catch (err) {
      alert('复制失败，请手动长按选中文本复制');
    }
    document.body.removeChild(ta);
  }

  function escapeHtml(str) {
    if (!str) return '';
    return str.replace(/[&<>"']/g, m => ({
      '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
    })[m]);
  }

  // 初始化加载
  loadAdminConfig();
})();