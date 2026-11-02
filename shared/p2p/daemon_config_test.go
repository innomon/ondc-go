package p2p

import (
	"testing"
	"time"
)

func TestLoadBridgeConfig_Defaults(t *testing.T) {
	emptyEnv := func(k string) (string, bool) {
		return "", false
	}

	cfg, err := LoadBridgeConfig([]string{}, emptyEnv)
	if err != nil {
		t.Fatalf("expected no error for defaults, got: %v", err)
	}

	if cfg.DBPath != "./data/ondc.db" {
		t.Errorf("expected default DBPath './data/ondc.db', got '%s'", cfg.DBPath)
	}
	if cfg.CRMPath != "./data/sqlite_p2p_crm.db" {
		t.Errorf("expected default CRMPath './data/sqlite_p2p_crm.db', got '%s'", cfg.CRMPath)
	}
	if cfg.NodeID == "" {
		t.Errorf("expected generated or non-empty default NodeID")
	}
	if cfg.SyncInterval != 2*time.Second {
		t.Errorf("expected default SyncInterval 2s, got %v", cfg.SyncInterval)
	}
	if cfg.MaxBatchSize != 500 {
		t.Errorf("expected default MaxBatchSize 500, got %d", cfg.MaxBatchSize)
	}
	if cfg.HTTPAddr != ":8085" {
		t.Errorf("expected default HTTPAddr ':8085', got '%s'", cfg.HTTPAddr)
	}
}

func TestLoadBridgeConfig_EnvOverride(t *testing.T) {
	envMap := map[string]string{
		"ONDC_SQLITE_DB_PATH":      "/custom/ondc.db",
		"ONDC_SQLITE_P2P_CRM_PATH": "/custom/crm.db",
		"ONDC_NODE_ID":             "env-node-01",
		"ONDC_P2P_SYNC_INTERVAL":   "500ms",
		"ONDC_P2P_MAX_BATCH_SIZE":  "250",
		"ONDC_BRIDGE_HTTP_ADDR":    ":9090",
	}
	lookupEnv := func(k string) (string, bool) {
		v, ok := envMap[k]
		return v, ok
	}

	cfg, err := LoadBridgeConfig([]string{}, lookupEnv)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if cfg.DBPath != "/custom/ondc.db" {
		t.Errorf("expected '/custom/ondc.db', got '%s'", cfg.DBPath)
	}
	if cfg.CRMPath != "/custom/crm.db" {
		t.Errorf("expected '/custom/crm.db', got '%s'", cfg.CRMPath)
	}
	if cfg.NodeID != "env-node-01" {
		t.Errorf("expected 'env-node-01', got '%s'", cfg.NodeID)
	}
	if cfg.SyncInterval != 500*time.Millisecond {
		t.Errorf("expected 500ms, got %v", cfg.SyncInterval)
	}
	if cfg.MaxBatchSize != 250 {
		t.Errorf("expected 250, got %d", cfg.MaxBatchSize)
	}
	if cfg.HTTPAddr != ":9090" {
		t.Errorf("expected ':9090', got '%s'", cfg.HTTPAddr)
	}
}

func TestLoadBridgeConfig_FlagsPrecedenceOverEnv(t *testing.T) {
	envMap := map[string]string{
		"ONDC_SQLITE_DB_PATH":      "/env/ondc.db",
		"ONDC_SQLITE_P2P_CRM_PATH": "/env/crm.db",
		"ONDC_NODE_ID":             "env-node",
		"ONDC_P2P_SYNC_INTERVAL":   "10s",
		"ONDC_P2P_MAX_BATCH_SIZE":  "100",
		"ONDC_BRIDGE_HTTP_ADDR":    ":7000",
	}
	lookupEnv := func(k string) (string, bool) {
		v, ok := envMap[k]
		return v, ok
	}

	args := []string{
		"--db-path", "/flag/ondc.db",
		"--crm-path", "/flag/crm.db",
		"--node-id", "flag-node-99",
		"--sync-interval", "1s",
		"--max-batch-size", "1000",
		"--http-addr", ":8000",
	}

	cfg, err := LoadBridgeConfig(args, lookupEnv)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if cfg.DBPath != "/flag/ondc.db" {
		t.Errorf("expected flag override '/flag/ondc.db', got '%s'", cfg.DBPath)
	}
	if cfg.CRMPath != "/flag/crm.db" {
		t.Errorf("expected flag override '/flag/crm.db', got '%s'", cfg.CRMPath)
	}
	if cfg.NodeID != "flag-node-99" {
		t.Errorf("expected 'flag-node-99', got '%s'", cfg.NodeID)
	}
	if cfg.SyncInterval != 1*time.Second {
		t.Errorf("expected 1s, got %v", cfg.SyncInterval)
	}
	if cfg.MaxBatchSize != 1000 {
		t.Errorf("expected 1000, got %d", cfg.MaxBatchSize)
	}
	if cfg.HTTPAddr != ":8000" {
		t.Errorf("expected ':8000', got '%s'", cfg.HTTPAddr)
	}
}

func TestLoadBridgeConfig_InvalidValues(t *testing.T) {
	lookupEnv := func(k string) (string, bool) { return "", false }

	// Invalid sync interval flag
	_, err := LoadBridgeConfig([]string{"--sync-interval", "invalid-duration"}, lookupEnv)
	if err == nil {
		t.Errorf("expected error for invalid sync-interval duration")
	}

	// Invalid max batch size
	_, err = LoadBridgeConfig([]string{"--max-batch-size", "-5"}, lookupEnv)
	if err == nil {
		t.Errorf("expected error for negative max-batch-size")
	}
}
