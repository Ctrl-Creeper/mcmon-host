(() => {
  const storageKey = 'mcmon-host-lang';
  const zh = {
    'Dashboard': '仪表盘',
    'Agents': '节点',
    'Settings': '设置',
    'Username': '用户名',
    'Password': '密码',
    '2FA code': '2FA 验证码',
    'Login': '登录',
    'Logout': '退出',
    'Sign in': '登录',
    'Use the admin account configured on this host.': '使用此 host 配置的管理员账户。',
    'Login required': '需要登录',
    'Login failed': '登录失败',
    'Signing in...': '正在登录...',
    'Signed in as': '已登录为',
    'Login to view host data.': '登录后查看 host 数据。',
    'Login to view host agents.': '登录后查看 host 节点。',
    'Login to manage account security.': '登录后管理账户安全。',
    'Account security': '账户安全',
    'Current password': '当前密码',
    'New password': '新密码',
    'Update password': '更新密码',
    '2FA status': '2FA 状态',
    'Enabled': '已启用',
    'Disabled': '未启用',
    'Generate 2FA': '生成 2FA',
    'Enable 2FA': '启用 2FA',
    'Disable 2FA': '禁用 2FA',
    'Secret': '密钥',
    'Provisioning URI': '配置 URI',
    'Password updated.': '密码已更新。',
    'Add this secret to your authenticator, then enter a code to enable 2FA.': '将此密钥添加到验证器，然后输入验证码启用 2FA。',
    '2FA enabled.': '2FA 已启用。',
    '2FA disabled.': '2FA 已禁用。',
    'Save': '保存',
    'Saved': '已保存',
    'Authorized': '已授权',
    'Toggle theme': '切换主题',
    'Online': '在线',
    'Offline': '离线',
    'Servers': '服务器',
    'All Monitored Servers': '全部监控服务器',
    'Connected Agents': '已连接节点',
    'Status': '状态',
    'Name': '名称',
    'ID': 'ID',
    'Version': '版本',
    'Last seen': '最后在线',
    'Targets': '目标',
    'Create node': '创建节点',
    'Create': '创建',
    'Node config': '节点配置',
    'Save targets': '保存目标',
    'Copy Linux/macOS install': '复制 Linux/macOS 安装命令',
    'Copy Windows install': '复制 Windows 安装命令',
    'Copy': '复制',
    'Close': '关闭',
    'Add target': '添加目标',
    'Remove': '移除',
    'Delete': '删除',
    'Delete agent': '删除节点',
    'This removes its targets and stored metrics from the host.': '这会从 host 删除它的目标和已保存指标。',
    'Target': '目标',
    'Target name': '目标名称',
    'Host': '主机',
    'Port': '端口',
    'Timeout (ms)': '超时（毫秒）',
    'Interval (sec)': '周期（秒）',
    'Players': '人数',
    'Latency': '延迟',
    'Probes / burst': '每轮探测次数',
    'Probe gap (ms)': '探测间隔（毫秒）',
    'Protocol version': '协议版本',
    'Linux/macOS install': 'Linux/macOS 安装',
    'Windows install': 'Windows 安装',
    'Target name and host are required.': '目标名称和主机不能为空。',
    'Name is required.': '名称不能为空。',
    'No agents registered yet.': '还没有注册节点。',
    'No servers being monitored yet. Connect an agent to get started.': '还没有监控服务器。连接一个节点后开始。',
    'Targets saved. Re-run the install command to overwrite the agent config.': '目标已保存。重新运行安装命令以覆盖节点配置。',
    'Linux/macOS install command copied.': '已复制 Linux/macOS 安装命令。',
    'Windows install command copied.': '已复制 Windows 安装命令。',
    'Configure': '配置',
    'Median': '中位数',
    'Loss': '丢包',
    'Server Detail': '服务器详情',
    'Back to Dashboard': '返回仪表盘',
  };

  function defaultLanguage() {
    const saved = localStorage.getItem(storageKey);
    if (saved === 'en' || saved === 'zh-CN') return saved;
    return navigator.language && navigator.language.toLowerCase().startsWith('zh') ? 'zh-CN' : 'en';
  }

  function currentLanguage() {
    return localStorage.getItem(storageKey) || defaultLanguage();
  }

  function translate(text) {
    const trimmed = String(text).trim();
    return currentLanguage() === 'zh-CN' ? (zh[trimmed] || text) : text;
  }

  function translateTextNodes(root) {
    const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT, {
      acceptNode(node) {
        if (!node.nodeValue.trim()) return NodeFilter.FILTER_REJECT;
        const parent = node.parentElement;
        if (!parent || ['SCRIPT', 'STYLE', 'TEXTAREA', 'CODE', 'PRE'].includes(parent.tagName)) {
          return NodeFilter.FILTER_REJECT;
        }
        return NodeFilter.FILTER_ACCEPT;
      }
    });
    const nodes = [];
    while (walker.nextNode()) nodes.push(walker.currentNode);
    nodes.forEach(node => {
      const original = node.__i18nOriginal || node.nodeValue;
      node.__i18nOriginal = original;
      const leading = original.match(/^\s*/)[0];
      const trailing = original.match(/\s*$/)[0];
      node.nodeValue = leading + translate(original.trim()) + trailing;
    });
  }

  function applyI18n() {
    document.documentElement.lang = currentLanguage() === 'zh-CN' ? 'zh-CN' : 'en';
    translateTextNodes(document.body);
    document.querySelectorAll('[placeholder], [title]').forEach(el => {
      ['placeholder', 'title'].forEach(attr => {
        if (!el.hasAttribute(attr)) return;
        const key = `__i18n_${attr}`;
        const original = el[key] || el.getAttribute(attr);
        el[key] = original;
        el.setAttribute(attr, translate(original));
      });
    });
    const btn = document.getElementById('langBtn');
    if (btn) btn.textContent = currentLanguage() === 'zh-CN' ? 'EN' : '中';
  }

  function installLanguageButton() {
    if (document.getElementById('langBtn')) return;
    const themeBtn = document.getElementById('themeBtn');
    if (!themeBtn || !themeBtn.parentElement) return;
    const btn = document.createElement('button');
    btn.className = 'theme-toggle';
    btn.id = 'langBtn';
    btn.type = 'button';
    btn.title = currentLanguage() === 'zh-CN' ? 'Switch to English' : '切换到中文';
    btn.onclick = () => {
      localStorage.setItem(storageKey, currentLanguage() === 'zh-CN' ? 'en' : 'zh-CN');
      applyI18n();
      window.dispatchEvent(new CustomEvent('mcmon-language-change'));
    };
    themeBtn.parentElement.insertBefore(btn, themeBtn);
  }

  window.mcmonI18n = { lang: currentLanguage, t: translate, apply: applyI18n };

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', () => {
      installLanguageButton();
      applyI18n();
    });
  } else {
    installLanguageButton();
    applyI18n();
  }
})();
