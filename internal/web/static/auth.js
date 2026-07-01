(() => {
  const t = text => window.mcmonI18n?.t(text) || text;

  const auth = {
    ready: false,
    authenticated: false,
    user: null,
    listeners: new Set(),
  };

  function notify(reason = 'change') {
    const detail = { ready: auth.ready, authenticated: auth.authenticated, user: auth.user, reason };
    window.dispatchEvent(new CustomEvent('mcmon-auth-change', { detail }));
    auth.listeners.forEach(listener => listener(detail));
  }

  function setAuthState(next, reason) {
    auth.ready = true;
    auth.authenticated = Boolean(next?.authenticated);
    auth.user = next?.user || null;
    renderAuthBar();
    notify(reason);
  }

  function installAuthBar() {
    const slot = document.querySelector('.auth-token');
    if (!slot) return;
    slot.innerHTML = `
      <span class="status" id="authStatus"></span>
      <button class="btn primary" id="openLoginBtn" type="button">Login</button>
      <button class="btn ghost" id="logoutBtn" type="button" style="display:none;">Logout</button>
    `;
    if (!document.getElementById('authModal')) {
      document.body.insertAdjacentHTML('beforeend', `
        <div class="modal-backdrop auth-modal-backdrop" id="authModal" aria-hidden="true">
          <section class="modal-panel auth-modal-panel" role="dialog" aria-modal="true" aria-labelledby="authTitle">
            <div class="modal-header">
              <div>
                <h2 id="authTitle">Sign in</h2>
                <p class="muted">Use the admin account configured on this host.</p>
              </div>
              <button class="btn ghost" id="closeLoginBtn" type="button">Close</button>
            </div>
            <div class="auth-form">
              <label>Username<input type="text" id="authUsername" placeholder="admin" autocomplete="username" /></label>
              <label>Password<input type="password" id="authPassword" placeholder="Password" autocomplete="current-password" /></label>
              <label>2FA code<input type="text" id="authTotp" placeholder="Optional" inputmode="numeric" autocomplete="one-time-code" /></label>
              <button class="btn primary" id="loginBtn" type="button">Login</button>
            </div>
            <div class="err" id="authErr"></div>
          </section>
        </div>
      `);
    }
    document.getElementById('openLoginBtn').onclick = openLogin;
    document.getElementById('closeLoginBtn').onclick = closeLogin;
    document.getElementById('loginBtn').onclick = login;
    document.getElementById('logoutBtn').onclick = logout;
    document.getElementById('authModal').onclick = event => {
      if (event.target.id === 'authModal') closeLogin();
    };
    document.addEventListener('keydown', event => {
      if (event.key === 'Escape') closeLogin();
      if (event.key === 'Enter' && document.getElementById('authModal').classList.contains('open')) login();
    });
    window.mcmonI18n?.apply();
    renderAuthBar();
  }

  async function login() {
    const username = document.getElementById('authUsername').value.trim();
    const password = document.getElementById('authPassword').value;
    const totp = document.getElementById('authTotp').value.trim();
    const errBox = document.getElementById('authErr');
    errBox.textContent = '';
    setStatus('Signing in...');
    try {
      const body = { username, password };
      if (totp) body.totp_code = totp;
      const res = await fetch('/api/auth/login', {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body)
      });
      if (!res.ok) throw new Error((await res.text()).trim());
      const me = await res.json();
      document.getElementById('authPassword').value = '';
      document.getElementById('authTotp').value = '';
      closeLogin();
      setAuthState({ authenticated: true, user: me }, 'login');
    } catch (err) {
      const message = err.message || t('Login failed');
      errBox.textContent = message;
      setStatus(message);
    }
  }

  async function logout() {
    await fetch('/api/auth/logout', { method: 'POST', credentials: 'include' }).catch(() => {});
    setAuthState({ authenticated: false, user: null }, 'logout');
  }

  async function refreshAuth({ reason = 'refresh' } = {}) {
    try {
      const res = await fetch('/api/auth/me', { credentials: 'include' });
      if (!res.ok) throw new Error('guest');
      const me = await res.json();
      setAuthState({ authenticated: true, user: me }, reason);
      return me;
    } catch {
      setAuthState({ authenticated: false, user: null }, reason);
      return null;
    }
  }

  function renderAuthBar() {
    const openBtn = document.getElementById('openLoginBtn');
    const logoutBtn = document.getElementById('logoutBtn');
    if (!openBtn || !logoutBtn) return;
    if (!auth.ready) {
      openBtn.style.display = 'none';
      logoutBtn.style.display = 'none';
      setStatus('');
      return;
    }
    if (auth.authenticated) {
      openBtn.style.display = 'none';
      logoutBtn.style.display = '';
      setStatus(`${t('Signed in as')} ${auth.user?.username || 'admin'}`);
      return;
    }
    openBtn.style.display = '';
    logoutBtn.style.display = 'none';
    setStatus('');
  }

  function openLogin() {
    const modal = document.getElementById('authModal');
    modal.classList.add('open');
    modal.setAttribute('aria-hidden', 'false');
    document.getElementById('authErr').textContent = '';
    setTimeout(() => document.getElementById('authUsername').focus(), 0);
  }

  function closeLogin() {
    const modal = document.getElementById('authModal');
    modal.classList.remove('open');
    modal.setAttribute('aria-hidden', 'true');
  }

  function setStatus(text) {
    const status = document.getElementById('authStatus');
    if (status) status.textContent = t(text);
  }

  function onAuthReady(callback) {
    const run = () => callback({ ready: auth.ready, authenticated: auth.authenticated, user: auth.user, reason: 'ready' });
    if (auth.ready) {
      run();
    } else {
      auth.listeners.add(function once(detail) {
        if (!detail.ready) return;
        auth.listeners.delete(once);
        run();
      });
    }
  }

  function onAuthChange(callback) {
    auth.listeners.add(callback);
    return () => auth.listeners.delete(callback);
  }

  window.apiFetch = async function apiFetch(url, opts = {}) {
    const headers = { ...(opts.headers || {}) };
    if (opts.body && !headers['Content-Type']) headers['Content-Type'] = 'application/json';
    const res = await fetch(url, { ...opts, headers, credentials: 'include' });
    if (res.status === 401) {
      setAuthState({ authenticated: false, user: null }, 'unauthorized');
      throw new Error(t('Login required'));
    }
    if (!res.ok) throw new Error((await res.text()).trim());
    if (res.status === 204) return null;
    return res.json();
  };

  const start = () => {
    installAuthBar();
    refreshAuth({ reason: 'initial' });
  };
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', start, { once: true });
  } else {
    start();
  }

  window.mcmonAuth = {
    get ready() { return auth.ready; },
    get isAuthenticated() { return auth.authenticated; },
    get user() { return auth.user; },
    refresh: refreshAuth,
    onReady: onAuthReady,
    onChange: onAuthChange,
    openLogin,
  };
})();
