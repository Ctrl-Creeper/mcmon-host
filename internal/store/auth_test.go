package store

import "testing"

func TestEnsureAdminCreatesSingleAdminAndChecksPassword(t *testing.T) {
	st, err := Open(t.TempDir() + "/host.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	admin, password, created, err := st.EnsureAdmin("admin", "initial-secret")
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("EnsureAdmin created = false, want true")
	}
	if admin.Username != "admin" || password != "initial-secret" {
		t.Fatalf("admin=%#v password=%q", admin, password)
	}

	admin, password, created, err = st.EnsureAdmin("ignored", "ignored-secret")
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Fatal("second EnsureAdmin created = true, want false")
	}
	if admin.Username != "admin" || password != "" {
		t.Fatalf("existing admin=%#v password=%q", admin, password)
	}

	got, ok, err := st.CheckAdminPassword("admin", "initial-secret")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || got.Username != "admin" {
		t.Fatalf("CheckAdminPassword ok=%v admin=%#v", ok, got)
	}

	if _, ok, err := st.CheckAdminPassword("admin", "wrong"); err != nil || ok {
		t.Fatalf("wrong password ok=%v err=%v", ok, err)
	}
}

func TestAdminSessionLifecycle(t *testing.T) {
	st, err := Open(t.TempDir() + "/host.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, _, _, err := st.EnsureAdmin("admin", "initial-secret"); err != nil {
		t.Fatal(err)
	}

	session, err := st.CreateAdminSession("agent", "127.0.0.1", 3600)
	if err != nil {
		t.Fatal(err)
	}
	if session.Token == "" || session.ExpiresAt == 0 {
		t.Fatalf("session = %#v", session)
	}

	got, ok, err := st.AdminSession(session.Token)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || got.Token != session.Token {
		t.Fatalf("AdminSession ok=%v got=%#v", ok, got)
	}

	if err := st.DeleteAdminSession(session.Token); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := st.AdminSession(session.Token); err != nil || ok {
		t.Fatalf("deleted session ok=%v err=%v", ok, err)
	}
}

func TestAdminTwoFactorSecretCanBeSetAndCleared(t *testing.T) {
	st, err := Open(t.TempDir() + "/host.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, _, _, err := st.EnsureAdmin("admin", "initial-secret"); err != nil {
		t.Fatal(err)
	}

	if err := st.SetAdminTwoFactor("secret-value"); err != nil {
		t.Fatal(err)
	}
	admin, ok, err := st.Admin()
	if err != nil {
		t.Fatal(err)
	}
	if !ok || admin.TwoFactorSecret != "secret-value" {
		t.Fatalf("admin after set ok=%v admin=%#v", ok, admin)
	}

	if err := st.SetAdminTwoFactor(""); err != nil {
		t.Fatal(err)
	}
	admin, ok, err = st.Admin()
	if err != nil {
		t.Fatal(err)
	}
	if !ok || admin.TwoFactorSecret != "" {
		t.Fatalf("admin after clear ok=%v admin=%#v", ok, admin)
	}
}
