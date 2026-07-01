package web

import (
	"net/http"
	"strings"

	"github.com/Ctrl-Creeper/mcmon-host/internal/store"
	"github.com/pquerna/otp/totp"
)

const (
	adminSessionCookie = "mcmon_session"
	adminSessionTTL    = int64(30 * 24 * 3600)
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	TOTPCode string `json:"totp_code"`
}

type loginResponse struct {
	SessionToken string `json:"session_token"`
	ExpiresAt    int64  `json:"expires_at"`
	Username     string `json:"username"`
	TOTPEnabled  bool   `json:"totp_enabled"`
}

type passwordRequest struct {
	Username        string `json:"username"`
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

type twoFactorRequest struct {
	Secret          string `json:"secret"`
	TOTPCode        string `json:"totp_code"`
	CurrentPassword string `json:"current_password"`
}

func registerAuthRoutes(mux *http.ServeMux, st *store.Store, opts Options) {
	mux.HandleFunc("/api/auth/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body loginRequest
		if !decodeJSON(w, r, &body) {
			return
		}
		admin, ok, err := st.CheckAdminPassword(body.Username, body.Password)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, "invalid credentials", http.StatusUnauthorized)
			return
		}
		if admin.TwoFactorSecret != "" {
			if body.TOTPCode == "" {
				http.Error(w, "totp code required", http.StatusUnauthorized)
				return
			}
			if !totp.Validate(body.TOTPCode, admin.TwoFactorSecret) {
				http.Error(w, "invalid totp code", http.StatusUnauthorized)
				return
			}
		}
		session, err := st.CreateAdminSession(r.UserAgent(), remoteIP(r), adminSessionTTL)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		setAdminSessionCookie(w, r, session.Token, int(adminSessionTTL))
		writeJSON(w, loginResponse{
			SessionToken: session.Token,
			ExpiresAt:    session.ExpiresAt,
			Username:     admin.Username,
			TOTPEnabled:  admin.TwoFactorSecret != "",
		})
	})

	mux.HandleFunc("/api/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if token := adminSessionToken(r); token != "" {
			_ = st.DeleteAdminSession(token)
		}
		setAdminSessionCookie(w, r, "", -1)
		writeJSON(w, map[string]bool{"ok": true})
	})

	mux.HandleFunc("/api/auth/me", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdmin(w, r, st, opts.AdminToken) {
			return
		}
		admin, ok, err := st.Admin()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, "admin not initialized", http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"username": admin.Username, "totp_enabled": admin.TwoFactorSecret != ""})
	})

	mux.HandleFunc("/api/auth/password", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdmin(w, r, st, opts.AdminToken) {
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body passwordRequest
		if !decodeJSON(w, r, &body) {
			return
		}
		admin, ok, err := st.Admin()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, "admin not initialized", http.StatusInternalServerError)
			return
		}
		admin, passwordOK, err := st.CheckAdminPassword(admin.Username, body.CurrentPassword)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !passwordOK {
			http.Error(w, "invalid credentials", http.StatusUnauthorized)
			return
		}
		username := strings.TrimSpace(body.Username)
		if username == "" {
			username = admin.Username
		}
		if strings.TrimSpace(body.NewPassword) == "" {
			http.Error(w, "new password is required", http.StatusBadRequest)
			return
		}
		if err := st.UpdateAdminCredentials(username, body.NewPassword); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if opts.UpdateAdminCredentials != nil {
			if err := opts.UpdateAdminCredentials(username, body.NewPassword); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		_ = st.DeleteOtherAdminSessions(adminSessionToken(r))
		writeJSON(w, map[string]bool{"ok": true})
	})

	mux.HandleFunc("/api/auth/2fa/setup", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdmin(w, r, st, opts.AdminToken) {
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		admin, _, err := st.Admin()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		key, err := totp.Generate(totp.GenerateOpts{Issuer: "MCMon", AccountName: admin.Username})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"secret": key.Secret(), "provisioning_uri": key.URL()})
	})

	mux.HandleFunc("/api/auth/2fa/enable", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdmin(w, r, st, opts.AdminToken) {
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body twoFactorRequest
		if !decodeJSON(w, r, &body) {
			return
		}
		if body.Secret == "" || body.TOTPCode == "" || !totp.Validate(body.TOTPCode, body.Secret) {
			http.Error(w, "invalid totp code", http.StatusBadRequest)
			return
		}
		if err := st.SetAdminTwoFactor(body.Secret); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	})

	mux.HandleFunc("/api/auth/2fa/disable", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdmin(w, r, st, opts.AdminToken) {
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body twoFactorRequest
		if !decodeJSON(w, r, &body) {
			return
		}
		admin, ok, err := st.Admin()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, "admin not initialized", http.StatusInternalServerError)
			return
		}
		passwordOK := false
		if body.CurrentPassword != "" {
			_, passwordOK, err = st.CheckAdminPassword(admin.Username, body.CurrentPassword)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		totpOK := admin.TwoFactorSecret != "" && body.TOTPCode != "" && totp.Validate(body.TOTPCode, admin.TwoFactorSecret)
		if !passwordOK && !totpOK {
			http.Error(w, "current password or totp code required", http.StatusUnauthorized)
			return
		}
		if err := st.SetAdminTwoFactor(""); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	})
}

func setAdminSessionCookie(w http.ResponseWriter, r *http.Request, token string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     adminSessionCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
	})
}

func adminSessionToken(r *http.Request) string {
	if token := bearerToken(r); token != "" {
		return token
	}
	cookie, err := r.Cookie(adminSessionCookie)
	if err == nil {
		return cookie.Value
	}
	return ""
}

func adminSessionValid(r *http.Request, st *store.Store) bool {
	_, ok, err := st.AdminSession(adminSessionToken(r))
	return err == nil && ok
}

func remoteIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		return forwarded
	}
	return r.RemoteAddr
}
