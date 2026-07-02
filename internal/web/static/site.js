(() => {
  const defaults = { site_title: 'MCMon Host', brand_name: 'MCMon Host', icon_url: '' };
  let current = { ...defaults };

  function settingValue(key) {
    return String(current[key] || defaults[key] || '').trim();
  }

  function titleBase() {
    return settingValue('site_title') || defaults.site_title;
  }

  function applyTitle(customTitle) {
    const body = document.body;
    if (body && customTitle) body.dataset.currentTitle = customTitle;
    const activeTitle = body?.dataset.currentTitle || body?.dataset.pageTitle || '';
    document.title = activeTitle ? `${activeTitle} - ${titleBase()}` : titleBase();
  }

  function setFavicon(url) {
    let link = document.querySelector('link[rel="icon"]');
    if (!link) {
      link = document.createElement('link');
      link.rel = 'icon';
      link.dataset.siteRole = 'favicon';
      document.head.appendChild(link);
    }
    link.href = url;
  }

  function apply(settings) {
    current = { ...defaults, ...(settings || {}) };
    document.querySelectorAll('[data-site-brand]').forEach(el => {
      el.textContent = `⚡ ${settingValue('brand_name') || defaults.brand_name}`;
    });
    applyTitle();
    if (settingValue('icon_url')) {
      const sep = settingValue('icon_url').includes('?') ? '&' : '?';
      setFavicon(`${settingValue('icon_url')}${sep}v=${Date.now()}`);
    }
    window.dispatchEvent(new CustomEvent('mcmon-site-settings', { detail: current }));
  }

  async function refresh() {
    try {
      const res = await fetch('/api/site-settings', { credentials: 'include' });
      if (!res.ok) throw new Error('site settings unavailable');
      const settings = await res.json();
      apply(settings);
      return settings;
    } catch {
      apply(defaults);
      return current;
    }
  }

  window.mcmonSite = {
    get settings() { return current; },
    get title() { return titleBase(); },
    refresh,
    apply,
    applyTitle,
  };

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', refresh, { once: true });
  } else {
    refresh();
  }
})();
