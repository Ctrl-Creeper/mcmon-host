(() => {
  const t = text => window.mcmonI18n?.t(text) || text;

  function installAuthBar() {
    const slot = document.querySelector('.auth-token');
    if (!slot) return;
    slot.innerHTML = `
      <input type="text" id="authUsername" placeholder="Username" autocomplete="username" />
      <input type="password" id="authPassword" placeholder="Password" autocomplete="current-password" />
      <input type="text" id="authTotp" placeholder="2FA code" inputmode="numeric" autocomplete="one-time-code" />
      <button class="btn" id="loginBtn" type="button">Login</button>
      <button class="btn ghost" id="logoutBtn" type="button" style="display:none;">Logout</button>
      <span class="status" id="authStatus"></span>
    `;
    document.getElementById('loginBtn').onclick = login;
    document.getElementById('logoutBtn').onclick = logout;
    window.mcmonI18n?.apply();
    refreshAuth();
  }

  async function login() {
    const username = document.getElementById('authUsername').value.trim();
    const password = document.getElementById('authPassword').value;
    const totp = document.getElementById('authTotp').value.trim();
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
      window.dispatchEvent(new CustomEvent('mcmon-auth-change'));
    } catch (err) {
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
    document.getElementById('authUsername').style.display = 'none';
    document.getElementById('authPassword').style.display = 'none';
    document.getElementById('authTotp').style.display = 'none';
    document.getElementById('loginBtn').style.display = 'none';
    document.getElementById('logoutBtn').style.display = '';
    setStatus(`${t('Signed in as')} ${username}`);
  }

  function setLoggedOut() {
    document.getElementById('authUsername').style.display = '';
    document.getElementById('authPassword').style.display = '';
    document.getElementById('authTotp').style.display = '';
    document.getElementById('loginBtn').style.display = '';
    document.getElementById('logoutBtn').style.display = 'none';
    setStatus('');
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
