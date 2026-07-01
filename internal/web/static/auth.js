(() => {
  const t = text => window.mcmonI18n?.t(text) || text;

  function installAuthBar() {
    const slot = document.querySelector('.auth-token');
    if (!slot) return;
    slot.innerHTML = `
      <span class="status" id="authStatus"></span>
      <button class="btn primary" id="openLoginBtn" type="button">Login</button>
      <button class="btn ghost" id="logoutBtn" type="button" style="display:none;">Logout</button>
    `;
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
    refreshAuth();
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
      if (!res.ok) throw new Error(await res.text());
      document.getElementById('authPassword').value = '';
      document.getElementById('authTotp').value = '';
      await refreshAuth();
      closeLogin();
      window.dispatchEvent(new CustomEvent('mcmon-auth-change'));
    } catch (err) {
      errBox.textContent = err.message || t('Login failed');
      setStatus(err.message || 'Login failed');
    }
  }

  async function logout() {
    await fetch('/api/auth/logout', { method: 'POST', credentials: 'include' }).catch(() => {});
    setLoggedOut();
    window.dispatchEvent(new CustomEvent('mcmon-auth-change'));
  }

  async function refreshAuth() {
    try {
      const me = await window.apiFetch('/api/auth/me');
      setLoggedIn(me.username || 'admin');
      return me;
    } catch {
      setLoggedOut();
      return null;
    }
  }

  function setLoggedIn(username) {
    document.getElementById('openLoginBtn').style.display = 'none';
    document.getElementById('logoutBtn').style.display = '';
    setStatus(`${t('Signed in as')} ${username}`);
  }

  function setLoggedOut() {
    document.getElementById('openLoginBtn').style.display = '';
    document.getElementById('logoutBtn').style.display = 'none';
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

  window.apiFetch = function apiFetch(url, opts = {}) {
    const headers = { ...(opts.headers || {}) };
    if (opts.body && !headers['Content-Type']) headers['Content-Type'] = 'application/json';
    return fetch(url, { ...opts, headers, credentials: 'include' }).then(async r => {
      if (r.status === 401) {
        setLoggedOut();
        throw new Error(t('Login required'));
      }
      if (!r.ok) throw new Error(await r.text());
      return r.json();
    });
  };

  window.mcmonAuth = { refresh: refreshAuth };

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', installAuthBar);
  } else {
    installAuthBar();
  }
})();
