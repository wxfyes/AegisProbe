// ==============================================================================
// 天阙·神盾安全探针 (TianQue-Probe) 控制台逻辑
// ==============================================================================

(function () {
  'use strict';

  let currentConfig = null;

  const dom = {
    loginModal: document.getElementById('loginModal'),
    adminLoginForm: document.getElementById('adminLoginForm'),
    adminPassword: document.getElementById('adminPassword'),
    adminLoginError: document.getElementById('adminLoginError'),
    btnLogout: document.getElementById('btnLogout'),
    nodeTableBody: document.getElementById('nodeTableBody'),
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
    nodeFlag: document.getElementById('nodeFlag'),
    nodeRegion: document.getElementById('nodeRegion'),
    nodeOS: document.getElementById('nodeOS'),
    nodeGroup: document.getElementById('nodeGroup'),
    nodePrice: document.getElementById('nodePrice'),
    nodeExpireDate: document.getElementById('nodeExpireDate'),
    nodeToken: document.getElementById('nodeToken'),
  };

  // 1. 同步主题
  const savedTheme = localStorage.getItem('aegis_theme') || 'dark';
  document.documentElement.setAttribute('data-theme', savedTheme);

  // 2. 登录流程
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

  // 3. 注销
  dom.btnLogout.addEventListener('click', async () => {
    await fetch('/api/auth/logout', { method: 'POST' });
    window.location.reload();
  });

  // 4. 加载配置与节点列表
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
      renderSettings(data);
      renderNodesTable(data.nodes || []);
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
    dom.nodeTableBody.innerHTML = nodes.map(n => `
      <tr>
        <td><code>${escapeHtml(n.id)}</code></td>
        <td><b>${escapeHtml(n.name)}</b></td>
        <td>${n.flag || '🌐'} ${escapeHtml(n.region || '')}</td>
        <td>${escapeHtml(n.os || 'linux')}</td>
        <td><span class="capsule capsule-price">${escapeHtml(n.group || '默认')}</span></td>
        <td>${escapeHtml(n.price || '--')}</td>
        <td>${escapeHtml(n.expire_date || '--')}</td>
        <td>
          <button class="btn-sm" onclick="window.showInstallCmd('${escapeHtml(n.id)}')">一键命令</button>
          <button class="btn-sm" onclick="window.editNode('${escapeHtml(n.id)}')">编辑</button>
          <button class="btn-sm danger" onclick="window.deleteNode('${escapeHtml(n.id)}')">删除</button>
        </td>
      </tr>
    `).join('');
  }

  // 5. 保存全局系统配置
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

  // 6. 测试 TG 告警消息
  dom.btnTestTG.addEventListener('click', async () => {
    try {
      const res = await fetch('/api/admin/test_tg', { method: 'POST' });
      const data = await res.json();
      if (res.ok && data.ok) {
        alert('🔔 测试通知已成功发出，请检查 Telegram 频道或私聊！');
      } else {
        alert('❌ 发送失败: ' + (data.error || '请先确认 TG 开关开启并保存配置'));
      }
    } catch (err) {
      alert('❌ 网络错误');
    }
  });

  // 7. 节点弹窗操作
  dom.btnAddNode.addEventListener('click', () => {
    dom.nodeModalTitle.textContent = '添加受控节点';
    dom.nodeForm.reset();
    dom.nodeId.readOnly = false;
    dom.nodeId.value = 'node_' + Math.floor(Math.random() * 900 + 100);
    dom.nodeModal.style.display = 'flex';
  });

  dom.btnCancelNode.addEventListener('click', () => {
    dom.nodeModal.style.display = 'none';
  });

  window.editNode = function (id) {
    if (!currentConfig || !currentConfig.nodes) return;
    const n = currentConfig.nodes.find(item => item.id === id);
    if (!n) return;

    dom.nodeModalTitle.textContent = '编辑节点: ' + n.name;
    dom.nodeId.value = n.id;
    dom.nodeId.readOnly = true;
    dom.nodeName.value = n.name || '';
    dom.nodeFlag.value = n.flag || '🇭🇰';
    dom.nodeRegion.value = n.region || 'HK';
    dom.nodeOS.value = n.os || 'debian';
    dom.nodeGroup.value = n.group || '中转';
    dom.nodePrice.value = n.price || '$29.99/月';
    dom.nodeExpireDate.value = (n.expire_date || '').replace(/\//g, '-');
    dom.nodeToken.value = n.token || '';

    dom.nodeModal.style.display = 'flex';
  };

  dom.nodeForm.addEventListener('submit', async (e) => {
    e.preventDefault();
    const payload = {
      id: dom.nodeId.value.trim(),
      name: dom.nodeName.value.trim(),
      flag: dom.nodeFlag.value.trim(),
      region: dom.nodeRegion.value.trim(),
      os: dom.nodeOS.value,
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
    if (!confirm(`确定要删除节点 [${id}] 吗？`)) return;

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

  // 8. 一键安装命令生成
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
    navigator.clipboard.writeText(dom.cmdText.textContent).then(() => {
      alert('✅ 一键安装命令已复制到剪贴板！');
    }).catch(() => {
      alert('请手动全选复制框内命令');
    });
  });

  function escapeHtml(str) {
    if (!str) return '';
    return str.replace(/[&<>"']/g, m => ({
      '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
    })[m]);
  }

  // 初始加载
  loadAdminConfig();
})();
