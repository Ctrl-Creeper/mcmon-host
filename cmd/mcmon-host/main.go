package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/Ctrl-Creeper/mcmon-host/internal/hub"
	"github.com/Ctrl-Creeper/mcmon-host/internal/store"
	"github.com/Ctrl-Creeper/mcmon-host/internal/web"
)

type Config struct {
	Listen           string `json:"listen"`
	DBPath           string `json:"db_path"`
	DiscoveryKey     string `json:"discovery_key"`
	AdminToken       string `json:"admin_token"`
	AdminUsername    string `json:"admin_username,omitempty"`
	AdminPassword    string `json:"admin_password,omitempty"`
	WSAllowedOrigins string `json:"ws_allowed_origins,omitempty"`
	PublicURL        string `json:"public_url,omitempty"`
}

func main() {
	cfgPath := flag.String("config", "config.json", "path to config file")
	verbose := flag.Bool("verbose", false, "enable informational logs")
	flag.Parse()

	cfg, adminCreatedInConfig, generatedAdminPassword, err := prepareConfig(*cfgPath)
	if err != nil {
		log.Fatal(err)
	}

	st, err := store.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer st.Close()
	if created, generatedPassword, err := ensureAdminFromConfig(st, cfg); err != nil {
		log.Fatalf("ensure admin account: %v", err)
	} else if *verbose && generatedPassword != "" {
		log.Printf("admin account created. username=%s", adminUsername(cfg))
		log.Printf("admin password was generated and written to config: %s", *cfgPath)
	} else if *verbose && (created || adminCreatedInConfig) {
		log.Printf("admin account synced from config. username=%s", adminUsername(cfg))
		if generatedAdminPassword != "" {
			log.Printf("admin password was generated and written to config: %s", *cfgPath)
		}
	}

	h := hub.New(st)
	h.SetVerbose(*verbose)
	mux := web.NewMux(st, h, web.Options{
		DiscoveryKey:     cfg.DiscoveryKey,
		AdminToken:       cfg.AdminToken,
		WSAllowedOrigins: cfg.WSAllowedOrigins,
		PublicURL:        cfg.PublicURL,
		Verbose:          *verbose,
		UpdateAdminCredentials: func(username, password string) error {
			cfg.AdminUsername = username
			cfg.AdminPassword = password
			return saveConfig(*cfgPath, cfg)
		},
	})

	if *verbose {
		fmt.Printf("mcmon-host listening on %s\n", cfg.Listen)
		fmt.Printf("dashboard: %s\n", dashboardURL(cfg.Listen))
	}

	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		// No ReadTimeout/WriteTimeout: WebSocket connections must stay open.
	}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func prepareConfig(path string) (Config, bool, string, error) {
	cfg := Config{
		Listen: ":9090",
		DBPath: "mcmon-host.db",
	}
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return Config{}, false, "", fmt.Errorf("parse config %s: %w", path, err)
		}
	} else if !os.IsNotExist(err) {
		return Config{}, false, "", err
	}

	dirty := false
	if cfg.DiscoveryKey == "" {
		cfg.DiscoveryKey = randHex(16)
		dirty = true
	}
	if cfg.AdminToken == "" {
		cfg.AdminToken = randHex(16)
		dirty = true
	}
	adminCreated := false
	generatedPassword := ""
	if cfg.AdminUsername == "" {
		cfg.AdminUsername = "admin"
		dirty = true
	}
	if cfg.AdminPassword == "" {
		cfg.AdminPassword = randHex(8)
		generatedPassword = cfg.AdminPassword
		adminCreated = true
		dirty = true
	}
	if dirty {
		if err := saveConfig(path, cfg); err != nil {
			return Config{}, false, "", err
		}
	}
	return cfg, adminCreated, generatedPassword, nil
}

func saveConfig(path string, cfg Config) error {
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		os.MkdirAll(dir, 0755)
	}
	data, _ := json.MarshalIndent(cfg, "", "  ")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("save config: rename: %w", err)
	}
	return nil
}

func ensureAdminFromConfig(st *store.Store, cfg Config) (created bool, generatedPassword string, err error) {
	username := adminUsername(cfg)
	if cfg.AdminPassword != "" {
		if _, ok, err := st.Admin(); err != nil {
			return false, "", err
		} else if ok {
			return false, "", st.UpdateAdminCredentials(username, cfg.AdminPassword)
		}
		_, _, created, err := st.EnsureAdmin(username, cfg.AdminPassword)
		return created, "", err
	}
	_, password, created, err := st.EnsureAdmin(username, randHex(8))
	if !created {
		password = ""
	}
	return created, password, err
}

func adminUsername(cfg Config) string {
	if cfg.AdminUsername == "" {
		return "admin"
	}
	return cfg.AdminUsername
}

func randHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func dashboardURL(listen string) string {
	if listen == "" {
		return "http://localhost:9090"
	}
	if listen[0] == ':' {
		return "http://localhost" + listen
	}
	return "http://" + listen
}
