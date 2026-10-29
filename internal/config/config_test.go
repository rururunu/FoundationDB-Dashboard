package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFromEnvRequiresCluster(t *testing.T) {
	t.Setenv("FDB_CLUSTER_FILE", "")
	t.Setenv("FDB_CONNECTION_STRING", "")
	t.Setenv("PORT", "")
	if _, err := FromEnv(); err == nil {
		t.Fatal("expected an error when no cluster is configured")
	}
}

func TestFromEnvConnectionString(t *testing.T) {
	t.Setenv("FDB_CLUSTER_FILE", "")
	t.Setenv("FDB_CONNECTION_STRING", " docker:docker@foundationdb:4500 ")
	t.Setenv("PORT", "8090")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != "0.0.0.0:8090" {
		t.Fatalf("addr %q", cfg.Addr)
	}
	if cfg.ConnectionString != "docker:docker@foundationdb:4500" {
		t.Fatalf("connection %q", cfg.ConnectionString)
	}
	if cfg.ConnectionDisplay != cfg.ConnectionString {
		t.Fatalf("display %q", cfg.ConnectionDisplay)
	}
}

func TestFromEnvRejectsMultilineConnectionString(t *testing.T) {
	t.Setenv("FDB_CLUSTER_FILE", "")
	t.Setenv("FDB_CONNECTION_STRING", "docker:docker@a:4500\ndocker:docker@b:4500")
	if _, err := FromEnv(); err == nil {
		t.Fatal("expected multiline connection string to be rejected")
	}
}

func TestFromEnvClusterFileWins(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fdb.cluster")
	if err := os.WriteFile(path, []byte("desc:id@10.0.0.8:4500\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FDB_CLUSTER_FILE", path)
	t.Setenv("FDB_CONNECTION_STRING", "docker:docker@foundationdb:4500")
	t.Setenv("PORT", "")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ClusterFile != path {
		t.Fatalf("file %q", cfg.ClusterFile)
	}
	if cfg.ConnectionDisplay != "desc:id@10.0.0.8:4500" {
		t.Fatalf("display %q", cfg.ConnectionDisplay)
	}
	if cfg.Addr != "0.0.0.0:8080" {
		t.Fatalf("addr %q", cfg.Addr)
	}
}

func TestFromEnvRejectsBadPort(t *testing.T) {
	t.Setenv("FDB_CLUSTER_FILE", "")
	t.Setenv("FDB_CONNECTION_STRING", "docker:docker@foundationdb:4500")
	t.Setenv("PORT", "0")
	if _, err := FromEnv(); err == nil {
		t.Fatal("expected port 0 to be rejected")
	}
}

func TestAuthOffByDefault(t *testing.T) {
	t.Setenv("FDB_CLUSTER_FILE", "")
	t.Setenv("FDB_CONNECTION_STRING", "docker:docker@foundationdb:4500")
	t.Setenv("PORT", "")
	t.Setenv("AUTH", "")
	t.Setenv("AUTH_USER", "admin")
	t.Setenv("AUTH_PASSWORD", "secret")
	cfg, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AuthEnabled || cfg.AuthUser != "" || cfg.AuthPassword != "" {
		t.Fatalf("auth should stay off: %+v", cfg)
	}
}

func TestAuthOnRequiresUserAndPassword(t *testing.T) {
	t.Setenv("FDB_CLUSTER_FILE", "")
	t.Setenv("FDB_CONNECTION_STRING", "docker:docker@foundationdb:4500")
	t.Setenv("AUTH", "on")
	t.Setenv("AUTH_USER", "")
	t.Setenv("AUTH_PASSWORD", "")
	if _, err := FromEnv(); err == nil {
		t.Fatal("expected AUTH=on without credentials to fail")
	}
	t.Setenv("AUTH_USER", "admin")
	t.Setenv("AUTH_PASSWORD", "   ")
	if _, err := FromEnv(); err == nil {
		t.Fatal("expected a blank password to fail")
	}
	t.Setenv("AUTH", "maybe")
	t.Setenv("AUTH_PASSWORD", "secret")
	if _, err := FromEnv(); err == nil {
		t.Fatal("expected an unknown AUTH value to fail")
	}
}

func TestAuthOn(t *testing.T) {
	t.Setenv("FDB_CLUSTER_FILE", "")
	t.Setenv("FDB_CONNECTION_STRING", "docker:docker@foundationdb:4500")
	t.Setenv("AUTH", " ON ")
	t.Setenv("AUTH_USER", " admin ")
	t.Setenv("AUTH_PASSWORD", "secret")
	cfg, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.AuthEnabled || cfg.AuthUser != "admin" || cfg.AuthPassword != "secret" {
		t.Fatalf("auth %+v", cfg)
	}
}
