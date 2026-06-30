(() => {
  const storageKey = 'mcmon-host-lang';
  const zh = {
    'Dashboard': '仪表盘',
    'Agents': '节点',
    'Admin token': '管理员 token',
    'Save': '保存',
    'Saved': '已保存',
    'Authorized': '已授权',
    'Token required': '需要 token',
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
    'No agents registered yet.': '还没有注册节点。',
    'Enter the admin token from the host config to view data.': '输入 host 配置中的管理员 token 以查看数据。',
    'Enter the admin token from the host config to view agents.': '输入 host 配置中的管理员 token 以查看节点。',
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
