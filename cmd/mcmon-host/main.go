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
	WSAllowedOrigins string `json:"ws_allowed_origins,omitempty"`
	PublicURL        string `json:"public_url,omitempty"`
}

func main() {
	cfgPath := flag.String("config", "config.json", "path to config file")
	flag.Parse()

	cfg := Config{
		Listen: ":9090",
		DBPath: "mcmon-host.db",
	}
	if data, err := os.ReadFile(*cfgPath); err == nil {
		if err := json.Unmarshal(data, &cfg); err != nil {
			log.Fatalf("parse config %s: %v", *cfgPath, err)
		}
	}

	if cfg.DiscoveryKey == "" {
		cfg.DiscoveryKey = randHex(16)
		log.Printf("generated discovery key: %s", cfg.DiscoveryKey)
		saveConfig(*cfgPath, cfg)
	}
	if cfg.AdminToken == "" {
		cfg.AdminToken = randHex(16)
		log.Printf("generated admin token: %s", cfg.AdminToken)
		saveConfig(*cfgPath, cfg)
	}

	st, err := store.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer st.Close()

	h := hub.New(st)
	mux := web.NewMux(st, h, web.Options{
		DiscoveryKey:     cfg.DiscoveryKey,
		AdminToken:       cfg.AdminToken,
		WSAllowedOrigins: cfg.WSAllowedOrigins,
		PublicURL:        cfg.PublicURL,
	})

	fmt.Printf("mcmon-host listening on %s\n", cfg.Listen)
	fmt.Printf("discovery key: %s\n", cfg.DiscoveryKey)
	fmt.Printf("admin token: %s\n", cfg.AdminToken)
	fmt.Printf("dashboard: http://localhost%s\n", cfg.Listen)

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

func saveConfig(path string, cfg Config) {
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		os.MkdirAll(dir, 0755)
	}
	data, _ := json.MarshalIndent(cfg, "", "  ")
	os.WriteFile(path, data, 0600)
}

func randHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}
