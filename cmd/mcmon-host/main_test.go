package main

import (
	"testing"

	"github.com/Ctrl-Creeper/mcmon-host/internal/store"
)

func TestEnsureAdminFromConfigUsesConfiguredCredentials(t *testing.T) {
	st, err := store.Open(t.TempDir() + "/host.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	created, password, err := ensureAdminFromConfig(st, Config{AdminUsername: "root", AdminPassword: "first-password"})
	if err != nil {
		t.Fatal(err)
	}
	if !created || password != "" {
		t.Fatalf("created=%v password=%q, want created without generated password", created, password)
	}
	if _, ok, err := st.CheckAdminPassword("root", "first-password"); err != nil || !ok {
		t.Fatalf("configured password check ok=%v err=%v", ok, err)
	}

	created, password, err = ensureAdminFromConfig(st, Config{AdminUsername: "admin2", AdminPassword: "second-password"})
	if err != nil {
		t.Fatal(err)
	}
	if created || password != "" {
		t.Fatalf("second sync created=%v password=%q, want update without generated password", created, password)
	}
	if _, ok, err := st.CheckAdminPassword("admin2", "second-password"); err != nil || !ok {
		t.Fatalf("updated password check ok=%v err=%v", ok, err)
	}
}

func TestEnsureAdminFromConfigGeneratesPasswordWhenMissing(t *testing.T) {
	st, err := store.Open(t.TempDir() + "/host.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	created, password, err := ensureAdminFromConfig(st, Config{})
	if err != nil {
		t.Fatal(err)
	}
	if !created || password == "" {
		t.Fatalf("created=%v password=%q, want generated password", created, password)
	}
	if _, ok, err := st.CheckAdminPassword("admin", password); err != nil || !ok {
		t.Fatalf("generated password check ok=%v err=%v", ok, err)
	}
}
