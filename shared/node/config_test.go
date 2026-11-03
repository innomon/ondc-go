package node_test

import (
	"errors"
	"flag"
	"testing"
	"time"

	"ondc/shared/node"
)

func TestLoadNodeConfig_Defaults(t *testing.T) {
	mockEnv := func(string) (string, bool) { return "", false }

	cfg, err := node.LoadNodeConfig([]string{}, mockEnv)
	if err != nil {
		t.Fatalf("unexpected error loading defaults: %v", err)
	}

	if cfg.Mode != node.ModeTunnel {
		t.Errorf("expected Mode=%q, got %q", node.ModeTunnel, cfg.Mode)
	}
	if cfg.HTTPPort != 8080 {
		t.Errorf("expected HTTPPort=8080, got %d", cfg.HTTPPort)
	}
	if cfg.HTTPSPort != 443 {
		t.Errorf("expected HTTPSPort=443, got %d", cfg.HTTPSPort)
	}
	if cfg.Domain != "localhost" {
		t.Errorf("expected Domain='localhost', got %q", cfg.Domain)
	}
	if cfg.CertDir != "./data/certs" {
		t.Errorf("expected CertDir='./data/certs', got %q", cfg.CertDir)
	}
	if cfg.DBPath != "./data/ondc.db" {
		t.Errorf("expected DBPath='./data/ondc.db', got %q", cfg.DBPath)
	}
	if cfg.CRMPath != "./data/sqlite_p2p_crm.db" {
		t.Errorf("expected CRMPath='./data/sqlite_p2p_crm.db', got %q", cfg.CRMPath)
	}
	if cfg.SyncInterval != 2*time.Second {
		t.Errorf("expected SyncInterval=2s, got %v", cfg.SyncInterval)
	}
	if !cfg.EnableBuyer || !cfg.EnableSeller || !cfg.EnableP2P {
		t.Errorf("expected all subsystems enabled by default: %+v", cfg)
	}
	if !node.IsLocalhost(cfg.Domain) {
		t.Errorf("expected 'localhost' to be considered localhost")
	}
	if cfg.LogDir != "./data/logs" || cfg.LogLevel != "INFO" || cfg.LogMaxSizeMB != 100 || cfg.LogMaxBackups != 5 || !cfg.LogConsole || !cfg.LogFile {
		t.Errorf("expected default logging settings, got: %+v", cfg)
	}
}

func TestLoadNodeConfig_EnvOverride(t *testing.T) {
	envVars := map[string]string{
		"ONDC_MODE":               "vm",
		"ONDC_HTTP_PORT":          "80",
		"ONDC_HTTPS_PORT":         "8443",
		"ONDC_DOMAIN":             "node.ondc-partner.org",
		"ONDC_CERT_DIR":           "/var/certs",
		"ONDC_SQLITE_DB_PATH":     "/mnt/data/primary.db",
		"ONDC_SQLITE_CRM_PATH":    "/mnt/data/crm.db",
		"ONDC_NODE_ID":            "sovereign-node-1",
		"ONDC_P2P_SYNC_INTERVAL":  "5s",
		"ONDC_ENABLE_BUYER":       "false",
		"ONDC_ENABLE_SELLER":      "true",
		"ONDC_ENABLE_P2P":         "true",
		"ONDC_LOG_DIR":            "/var/log/ondc",
		"ONDC_LOG_LEVEL":          "DEBUG",
		"ONDC_LOG_MAX_SIZE_MB":    "200",
		"ONDC_LOG_MAX_BACKUPS":    "10",
		"ONDC_LOG_CONSOLE":        "false",
		"ONDC_LOG_FILE":           "true",
	}

	mockEnv := func(key string) (string, bool) {
		val, ok := envVars[key]
		return val, ok
	}

	cfg, err := node.LoadNodeConfig([]string{}, mockEnv)
	if err != nil {
		t.Fatalf("unexpected error loading from env: %v", err)
	}

	if cfg.Mode != node.ModeVM {
		t.Errorf("expected Mode=%q, got %q", node.ModeVM, cfg.Mode)
	}
	if cfg.HTTPPort != 80 {
		t.Errorf("expected HTTPPort=80, got %d", cfg.HTTPPort)
	}
	if cfg.HTTPSPort != 8443 {
		t.Errorf("expected HTTPSPort=8443, got %d", cfg.HTTPSPort)
	}
	if cfg.Domain != "node.ondc-partner.org" {
		t.Errorf("expected Domain='node.ondc-partner.org', got %q", cfg.Domain)
	}
	if cfg.CertDir != "/var/certs" {
		t.Errorf("expected CertDir='/var/certs', got %q", cfg.CertDir)
	}
	if cfg.DBPath != "/mnt/data/primary.db" {
		t.Errorf("expected DBPath='/mnt/data/primary.db', got %q", cfg.DBPath)
	}
	if cfg.CRMPath != "/mnt/data/crm.db" {
		t.Errorf("expected CRMPath='/mnt/data/crm.db', got %q", cfg.CRMPath)
	}
	if cfg.NodeID != "sovereign-node-1" {
		t.Errorf("expected NodeID='sovereign-node-1', got %q", cfg.NodeID)
	}
	if cfg.SyncInterval != 5*time.Second {
		t.Errorf("expected SyncInterval=5s, got %v", cfg.SyncInterval)
	}
	if cfg.EnableBuyer != false {
		t.Errorf("expected EnableBuyer=false, got %v", cfg.EnableBuyer)
	}
	if node.IsLocalhost(cfg.Domain) {
		t.Errorf("expected 'node.ondc-partner.org' to not be localhost")
	}
	if cfg.LogDir != "/var/log/ondc" || cfg.LogLevel != "DEBUG" || cfg.LogMaxSizeMB != 200 || cfg.LogMaxBackups != 10 || cfg.LogConsole || !cfg.LogFile {
		t.Errorf("expected env-overridden logging settings, got: %+v", cfg)
	}
}

func TestLoadNodeConfig_FlagsPrecedenceOverEnv(t *testing.T) {
	envVars := map[string]string{
		"ONDC_MODE":       "tunnel",
		"ONDC_HTTP_PORT":  "8080",
		"ONDC_DOMAIN":     "tunnel.domain.com",
		"ONDC_LOG_DIR":    "/var/log/from_env",
		"ONDC_LOG_LEVEL":  "INFO",
	}

	mockEnv := func(key string) (string, bool) {
		val, ok := envVars[key]
		return val, ok
	}

	cliArgs := []string{
		"-mode", "vm",
		"-http-port", "80",
		"-domain", "sovereign.ondc.org",
		"-cert-dir", "/etc/letsencrypt",
		"-enable-seller=false",
		"-log-dir", "/mnt/flash/logs",
		"-log-level", "DEBUG",
		"-log-max-size", "50",
		"-log-max-backups", "3",
	}

	cfg, err := node.LoadNodeConfig(cliArgs, mockEnv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Mode != node.ModeVM {
		t.Errorf("flag should override env: expected Mode=vm, got %q", cfg.Mode)
	}
	if cfg.HTTPPort != 80 {
		t.Errorf("flag should override env: expected HTTPPort=80, got %d", cfg.HTTPPort)
	}
	if cfg.Domain != "sovereign.ondc.org" {
		t.Errorf("flag should override env: expected Domain='sovereign.ondc.org', got %q", cfg.Domain)
	}
	if cfg.CertDir != "/etc/letsencrypt" {
		t.Errorf("flag should override env: expected CertDir='/etc/letsencrypt', got %q", cfg.CertDir)
	}
	if cfg.EnableSeller != false {
		t.Errorf("flag should override env: expected EnableSeller=false, got %v", cfg.EnableSeller)
	}
	if cfg.LogDir != "/mnt/flash/logs" || cfg.LogLevel != "DEBUG" || cfg.LogMaxSizeMB != 50 || cfg.LogMaxBackups != 3 {
		t.Errorf("flag should override env for logging: %+v", cfg)
	}
}

func TestLoadNodeConfig_InvalidFlags(t *testing.T) {
	mockEnv := func(string) (string, bool) { return "", false }
	_, err := node.LoadNodeConfig([]string{"-unknown-flag"}, mockEnv)
	if err == nil {
		t.Fatalf("expected error for unknown flag, got nil")
	}

	_, err = node.LoadNodeConfig([]string{"-sync-interval", "not-a-duration"}, mockEnv)
	if err == nil {
		t.Fatalf("expected error for invalid duration flag, got nil")
	}

	_, err = node.LoadNodeConfig([]string{"-mode", "unsupported-mode"}, mockEnv)
	if err == nil || !errors.Is(err, node.ErrInvalidMode) {
		t.Fatalf("expected ErrInvalidMode, got %v", err)
	}
}

func TestLoadNodeConfig_Help(t *testing.T) {
	mockEnv := func(string) (string, bool) { return "", false }
	_, err := node.LoadNodeConfig([]string{"--help"}, mockEnv)
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("expected flag.ErrHelp, got %v", err)
	}
}
